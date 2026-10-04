package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/compensationday"
	"nfc-time-tracking-server/internal/service/fixednonwork"
	"nfc-time-tracking-server/internal/store"
)

var errDayOverlap = errors.New("die korrigierte Zeit überschneidet sich mit einem anderen Eintrag an diesem Tag")

// checkDayOverlap reports errDayOverlap when [newIn,newOut) overlaps the effective interval (latest correction
// if present, disabled entries ignored) of another work period of userID on day. excludeID skips the period
// being corrected (0 for a new entry).
func checkDayOverlap(ctx context.Context, wps store.WorkPeriodStore, corrs store.CorrectionStore, userID int, day string, excludeID int, newIn, newOut time.Time) error {
	dayPeriods, err := wps.ListByUserDateRange(ctx, userID, day, day)
	if err != nil {
		return err
	}
	newStart := newIn.UTC()
	newEnd := newOut.UTC()
	for _, p := range dayPeriods {
		if p.ID == excludeID {
			continue
		}
		start := p.PunchIn.UTC()
		end := p.PunchOut
		if corr, err := corrs.GetLatestForPeriod(ctx, p.ID); err == nil && corr != nil {
			if corr.Disabled {
				continue
			}
			start = corr.CorrectedIn.UTC()
			cend := corr.CorrectedOut.UTC()
			end = &cend
		}
		// Ignore invalid existing intervals; overlap check is best-effort here.
		if end != nil && !end.After(start) {
			continue
		}
		// Overlap condition: existing_start < new_end && new_start < existing_end (nil end = open-ended).
		if start.Before(newEnd) && (end == nil || newStart.Before(end.UTC())) {
			return errDayOverlap
		}
	}
	return nil
}

// ChangeRequestHandler serves employee requests (time corrections, missing entries, vacation)
// and their approval by Leitung. A request changes nothing until it is approved.
type ChangeRequestHandler struct {
	Requests              store.ChangeRequestStore
	Users                 store.UserStore
	WorkPeriods           store.WorkPeriodStore
	Corrections           store.CorrectionStore
	Absences              store.AbsenceStore
	CompensationDayClaims store.CompensationDayClaimStore
	FixedNonWorkWeekdays  store.FixedNonWorkWeekdaysStore
	Holidays              store.HolidayStore
	ClosureDays           store.ClosureDayStore
	Audit                 *audit.Logger
}

// maxVacationRequestDays limits the calendar span of one vacation request.
const maxVacationRequestDays = 62

// requestError is a user-facing validation or conflict error.
type requestError struct {
	status int
	msg    string
}

func (e *requestError) Error() string { return e.msg }

func badRequest(format string, a ...any) error {
	return &requestError{status: http.StatusBadRequest, msg: fmt.Sprintf(format, a...)}
}

func conflict(format string, a ...any) error {
	return &requestError{status: http.StatusConflict, msg: fmt.Sprintf(format, a...)}
}

func writeRequestError(w http.ResponseWriter, err error) {
	var re *requestError
	if errors.As(err, &re) {
		response.Error(w, re.status, re.msg)
		return
	}
	response.Error(w, http.StatusInternalServerError, "Antrag konnte nicht verarbeitet werden")
}

type changeRequestResponse struct {
	model.ChangeRequest
	UserDisplayName string   `json:"user_display_name"`
	DecidedByName   string   `json:"decided_by_name,omitempty"`
	VacationDays    float64  `json:"vacation_days,omitempty"`
	VacationDates   []string `json:"vacation_dates,omitempty"`
}

type changeRequestBody struct {
	Kind         model.ChangeRequestKind `json:"kind"`
	WorkPeriodID int                     `json:"work_period_id"`
	WorkDate     string                  `json:"work_date"`
	PunchIn      *time.Time              `json:"punch_in"`
	PunchOut     *time.Time              `json:"punch_out"`
	DateFrom     string                  `json:"date_from"`
	DateTo       string                  `json:"date_to"`
	HalfDay      bool                    `json:"half_day"`
	Reason       string                  `json:"reason"`
}

type decisionBody struct {
	Comment string `json:"comment"`
}

func todayLocal() time.Time {
	now := time.Now().In(time.Local)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
}

// effectivePeriod returns the current interval of a work period (latest correction wins).
func (h *ChangeRequestHandler) effectivePeriod(ctx context.Context, wp *model.WorkPeriod) (in time.Time, out *time.Time, disabled bool, err error) {
	in, out = wp.PunchIn, wp.PunchOut
	latest, err := h.Corrections.GetLatestForPeriod(ctx, wp.ID)
	if err != nil {
		return in, out, false, err
	}
	if latest != nil {
		if latest.Disabled {
			return in, out, true, nil
		}
		co := latest.CorrectedOut
		return latest.CorrectedIn, &co, false, nil
	}
	return in, out, false, nil
}

func validateTimesOnDay(day string, in, out *time.Time) error {
	if in == nil || out == nil {
		return badRequest("Bitte Kommen und Gehen angeben.")
	}
	if !out.After(*in) {
		return badRequest("Gehen muss nach Kommen liegen.")
	}
	if in.In(time.Local).Format("2006-01-02") != day {
		return badRequest("Kommen muss am %s liegen.", formatAbsenceDateDE(day))
	}
	return nil
}

// vacationDates returns the dates in [from,to] that count as vacation days for userID
// (weekends, fixed non-work weekdays, holidays and closure days are skipped).
func (h *ChangeRequestHandler) vacationDates(ctx context.Context, userID int, from, to string) ([]string, error) {
	a, err := time.ParseInLocation("2006-01-02", from, time.Local)
	if err != nil {
		return nil, badRequest("Ungültiges Startdatum.")
	}
	b, err := time.ParseInLocation("2006-01-02", to, time.Local)
	if err != nil {
		return nil, badRequest("Ungültiges Enddatum.")
	}
	if b.Before(a) {
		return nil, badRequest("Das Enddatum liegt vor dem Startdatum.")
	}
	if b.Sub(a) > time.Duration(maxVacationRequestDays)*24*time.Hour {
		return nil, badRequest("Ein Urlaubsantrag darf höchstens %d Kalendertage umfassen.", maxVacationRequestDays)
	}
	var out []string
	var lastErr error
	for d := a; !d.After(b); d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		fixed := fixednonwork.WeekdaysForUserDate(ctx, h.FixedNonWorkWeekdays, userID, ds)
		if err := validateVacationAbsenceDate(ctx, h.Holidays, h.ClosureDays, fixed, ds); err != nil {
			lastErr = err
			continue
		}
		out = append(out, ds)
	}
	if len(out) == 0 {
		if from == to && lastErr != nil {
			return nil, badRequest("%s", lastErr.Error())
		}
		return nil, badRequest("Im gewählten Zeitraum liegt kein Arbeitstag, für den Urlaub nötig wäre.")
	}
	return out, nil
}

func (h *ChangeRequestHandler) checkNoAbsences(ctx context.Context, userID int, dates []string) error {
	for _, ds := range dates {
		ex, err := h.Absences.GetForUserDate(ctx, userID, ds)
		if err != nil {
			return err
		}
		if ex != nil {
			t := ex.AbsenceType
			return conflict("%s", duplicateAbsenceUserMessage(ds, &t))
		}
	}
	return nil
}

func (h *ChangeRequestHandler) pending(ctx context.Context, userID int, kind model.ChangeRequestKind) ([]model.ChangeRequest, error) {
	uid := userID
	return h.Requests.List(ctx, store.ChangeRequestFilter{UserID: &uid, Kind: kind, Statuses: []model.ChangeRequestStatus{model.RequestPending}})
}

// validateNew checks a new request and fills derived fields (original times, normalized dates).
func (h *ChangeRequestHandler) validateNew(ctx context.Context, userID int, body changeRequestBody) (*model.ChangeRequest, error) {
	reason := strings.TrimSpace(body.Reason)
	c := &model.ChangeRequest{UserID: userID, Kind: body.Kind, Reason: reason}
	switch body.Kind {
	case model.RequestTimeCorrection:
		if reason == "" {
			return nil, badRequest("Bitte einen Grund angeben.")
		}
		wp, err := h.WorkPeriods.GetByID(ctx, body.WorkPeriodID)
		if err != nil {
			return nil, err
		}
		if wp == nil || wp.UserID != userID || wp.IsBreak {
			return nil, badRequest("Ungültiger Zeiteintrag.")
		}
		if err := validateTimesOnDay(wp.WorkDate, body.PunchIn, body.PunchOut); err != nil {
			return nil, err
		}
		in, out, disabled, err := h.effectivePeriod(ctx, wp)
		if err != nil {
			return nil, err
		}
		if disabled {
			return nil, badRequest("Dieser Eintrag wurde von der Leitung deaktiviert. Bitte direkt bei der Leitung melden.")
		}
		open, err := h.pending(ctx, userID, model.RequestTimeCorrection)
		if err != nil {
			return nil, err
		}
		for _, p := range open {
			if p.WorkPeriodID != nil && *p.WorkPeriodID == wp.ID {
				return nil, conflict("Für diesen Eintrag gibt es schon einen offenen Antrag.")
			}
		}
		if err := checkDayOverlap(ctx, h.WorkPeriods, h.Corrections, userID, wp.WorkDate, wp.ID, *body.PunchIn, *body.PunchOut); err != nil {
			if errors.Is(err, errDayOverlap) {
				return nil, badRequest("%s", err.Error())
			}
			return nil, err
		}
		id := wp.ID
		c.WorkPeriodID = &id
		c.WorkDate = wp.WorkDate
		c.OriginalIn = &in
		c.OriginalOut = out
		c.PunchIn, c.PunchOut = body.PunchIn, body.PunchOut
	case model.RequestTimeEntry:
		if reason == "" {
			return nil, badRequest("Bitte einen Grund angeben.")
		}
		wd, err := time.ParseInLocation("2006-01-02", body.WorkDate, time.Local)
		if err != nil {
			return nil, badRequest("Ungültiges Datum.")
		}
		if wd.After(todayLocal()) {
			return nil, badRequest("Arbeitszeiten in der Zukunft können nicht nachgetragen werden.")
		}
		if err := validateTimesOnDay(body.WorkDate, body.PunchIn, body.PunchOut); err != nil {
			return nil, err
		}
		if err := checkDayOverlap(ctx, h.WorkPeriods, h.Corrections, userID, body.WorkDate, 0, *body.PunchIn, *body.PunchOut); err != nil {
			if errors.Is(err, errDayOverlap) {
				return nil, badRequest("Der Zeitraum überschneidet sich mit einem vorhandenen Eintrag an diesem Tag.")
			}
			return nil, err
		}
		c.WorkDate = body.WorkDate
		c.PunchIn, c.PunchOut = body.PunchIn, body.PunchOut
	case model.RequestVacation:
		if body.DateTo == "" {
			body.DateTo = body.DateFrom
		}
		if body.HalfDay && body.DateFrom != body.DateTo {
			return nil, badRequest("Ein halber Urlaubstag ist nur für einen einzelnen Tag möglich.")
		}
		dates, err := h.vacationDates(ctx, userID, body.DateFrom, body.DateTo)
		if err != nil {
			return nil, err
		}
		if err := h.checkNoAbsences(ctx, userID, dates); err != nil {
			return nil, err
		}
		open, err := h.pending(ctx, userID, model.RequestVacation)
		if err != nil {
			return nil, err
		}
		for _, p := range open {
			if p.DateFrom <= body.DateTo && body.DateFrom <= p.DateTo {
				return nil, conflict("Für diesen Zeitraum gibt es schon einen offenen Urlaubsantrag.")
			}
		}
		c.DateFrom, c.DateTo, c.HalfDay = body.DateFrom, body.DateTo, body.HalfDay
	default:
		return nil, badRequest("Unbekannte Antragsart.")
	}
	return c, nil
}

func (h *ChangeRequestHandler) toResponses(ctx context.Context, list []model.ChangeRequest) []changeRequestResponse {
	names := map[int]string{}
	name := func(id int) string {
		if n, ok := names[id]; ok {
			return n
		}
		n := ""
		if u, err := h.Users.GetByID(ctx, id); err == nil && u != nil {
			n = u.DisplayName
		}
		names[id] = n
		return n
	}
	out := make([]changeRequestResponse, 0, len(list))
	for _, c := range list {
		r := changeRequestResponse{ChangeRequest: c, UserDisplayName: name(c.UserID)}
		if c.DecidedBy != nil {
			r.DecidedByName = name(*c.DecidedBy)
		}
		if c.Kind == model.RequestVacation {
			if dates, err := h.vacationDates(ctx, c.UserID, c.DateFrom, c.DateTo); err == nil {
				r.VacationDates = dates
				r.VacationDays = float64(len(dates))
				if c.HalfDay {
					r.VacationDays = 0.5
				}
			}
		}
		out = append(out, r)
	}
	return out
}

// --- Mitarbeitende ---

// ListMine lists the caller's own requests (newest first).
func (h *ChangeRequestHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if uid == 0 {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	list, err := h.Requests.List(r.Context(), store.ChangeRequestFilter{UserID: &uid, Limit: 200})
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"requests": h.toResponses(r.Context(), list)})
}

// CreateMine files a new request for the caller. Nothing is changed until Leitung approves.
func (h *ChangeRequestHandler) CreateMine(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if uid == 0 {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body changeRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	c, err := h.validateNew(r.Context(), uid, body)
	if err != nil {
		writeRequestError(w, err)
		return
	}
	if err := h.Requests.Create(r.Context(), c); err != nil {
		response.Error(w, http.StatusInternalServerError, "create failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionCreate, EntityType: audit.EntityChangeRequest, EntityID: auditID(c.ID),
		TargetUserID: auditTarget(uid),
		Summary:      audit.JSONSummary(map[string]any{"kind": c.Kind, "work_date": c.WorkDate, "date_from": c.DateFrom, "date_to": c.DateTo}),
	})
	resp := h.toResponses(r.Context(), []model.ChangeRequest{*c})
	response.JSON(w, http.StatusCreated, resp[0])
}

// WithdrawMine withdraws a pending request of the caller.
func (h *ChangeRequestHandler) WithdrawMine(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if uid == 0 || err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	ok, err := h.Requests.Withdraw(r.Context(), id, uid)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "update failed")
		return
	}
	if !ok {
		response.Error(w, http.StatusConflict, "Nur offene eigene Anträge können zurückgezogen werden.")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityChangeRequest, EntityID: auditID(id),
		TargetUserID: auditTarget(uid),
		Summary:      audit.JSONSummary(map[string]any{"status": model.RequestWithdrawn}),
	})
	w.WriteHeader(http.StatusNoContent)
}

// --- Leitung ---

// List lists requests for Leitung. ?status=pending (default), decided or all.
func (h *ChangeRequestHandler) List(w http.ResponseWriter, r *http.Request) {
	f := store.ChangeRequestFilter{}
	switch r.URL.Query().Get("status") {
	case "", "pending":
		f.Statuses = []model.ChangeRequestStatus{model.RequestPending}
	case "decided":
		f.Statuses = []model.ChangeRequestStatus{model.RequestApproved, model.RequestRejected}
		f.Limit = 200
	case "all":
		f.Limit = 200
	default:
		response.Error(w, http.StatusBadRequest, "invalid status")
		return
	}
	list, err := h.Requests.List(r.Context(), f)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"requests": h.toResponses(r.Context(), list)})
}

func (h *ChangeRequestHandler) PendingCount(w http.ResponseWriter, r *http.Request) {
	n, err := h.Requests.CountPending(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]int{"pending": n})
}

// loadForDecision loads a pending request the caller may decide on.
func (h *ChangeRequestHandler) loadForDecision(r *http.Request) (*model.ChangeRequest, error) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		return nil, badRequest("invalid id")
	}
	c, err := h.Requests.GetByID(r.Context(), id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, &requestError{status: http.StatusNotFound, msg: "Antrag nicht gefunden."}
	}
	if c.Status != model.RequestPending {
		return nil, conflict("Der Antrag wurde bereits bearbeitet.")
	}
	if c.UserID == middleware.UserID(r) {
		return nil, &requestError{status: http.StatusForbidden, msg: "Eigene Anträge müssen von einer anderen Leitung entschieden werden."}
	}
	owner, err := h.Users.GetByID(r.Context(), c.UserID)
	if err != nil || owner == nil {
		return nil, &requestError{status: http.StatusNotFound, msg: "Mitarbeiter nicht gefunden."}
	}
	if !model.ActorMayManageUser(middleware.Role(r), owner) {
		return nil, &requestError{status: http.StatusForbidden, msg: "forbidden"}
	}
	return c, nil
}

// Reject declines a request; a comment for the employee is required.
func (h *ChangeRequestHandler) Reject(w http.ResponseWriter, r *http.Request) {
	var body decisionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	comment := strings.TrimSpace(body.Comment)
	if comment == "" {
		response.Error(w, http.StatusBadRequest, "Bitte einen Kommentar zur Ablehnung angeben.")
		return
	}
	c, err := h.loadForDecision(r)
	if err != nil {
		writeRequestError(w, err)
		return
	}
	ok, err := h.Requests.SetDecision(r.Context(), c.ID, model.RequestRejected, middleware.UserID(r), comment)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "update failed")
		return
	}
	if !ok {
		response.Error(w, http.StatusConflict, "Der Antrag wurde bereits bearbeitet.")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityChangeRequest, EntityID: auditID(c.ID),
		TargetUserID: auditTarget(c.UserID),
		Summary:      audit.JSONSummary(map[string]any{"kind": c.Kind, "status": model.RequestRejected}),
	})
	h.respondOne(w, r, c.ID)
}

// Approve accepts a request and applies the change (correction, manual entry or vacation days).
func (h *ChangeRequestHandler) Approve(w http.ResponseWriter, r *http.Request) {
	var body decisionBody
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			response.Error(w, http.StatusBadRequest, "invalid json")
			return
		}
	}
	c, err := h.loadForDecision(r)
	if err != nil {
		writeRequestError(w, err)
		return
	}
	by := middleware.UserID(r)
	// Claim the request first so two concurrent approvals cannot both apply it.
	ok, err := h.Requests.SetDecision(r.Context(), c.ID, model.RequestApproved, by, strings.TrimSpace(body.Comment))
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "update failed")
		return
	}
	if !ok {
		response.Error(w, http.StatusConflict, "Der Antrag wurde bereits bearbeitet.")
		return
	}
	if err := h.apply(r.Context(), c, by); err != nil {
		_ = h.Requests.Reopen(r.Context(), c.ID)
		writeRequestError(w, err)
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityChangeRequest, EntityID: auditID(c.ID),
		TargetUserID: auditTarget(c.UserID),
		Summary:      audit.JSONSummary(map[string]any{"kind": c.Kind, "status": model.RequestApproved}),
	})
	h.respondOne(w, r, c.ID)
}

func (h *ChangeRequestHandler) respondOne(w http.ResponseWriter, r *http.Request, id int) {
	c, err := h.Requests.GetByID(r.Context(), id)
	if err != nil || c == nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, h.toResponses(r.Context(), []model.ChangeRequest{*c})[0])
}

// apply performs the approved change. State is re-checked, since it may have changed since the request.
func (h *ChangeRequestHandler) apply(ctx context.Context, c *model.ChangeRequest, by int) error {
	switch c.Kind {
	case model.RequestTimeCorrection:
		wp, err := h.WorkPeriods.GetByID(ctx, *c.WorkPeriodID)
		if err != nil {
			return err
		}
		if wp == nil {
			return conflict("Der Zeiteintrag existiert nicht mehr. Bitte den Antrag ablehnen.")
		}
		in, out, disabled, err := h.effectivePeriod(ctx, wp)
		if err != nil {
			return err
		}
		unchanged := !disabled && c.OriginalIn != nil && in.Equal(*c.OriginalIn) &&
			((out == nil && c.OriginalOut == nil) || (out != nil && c.OriginalOut != nil && out.Equal(*c.OriginalOut)))
		if !unchanged {
			return conflict("Der Zeiteintrag wurde seit dem Antrag geändert. Bitte den Antrag ablehnen und neu stellen lassen.")
		}
		if err := checkDayOverlap(ctx, h.WorkPeriods, h.Corrections, c.UserID, wp.WorkDate, wp.ID, *c.PunchIn, *c.PunchOut); err != nil {
			if errors.Is(err, errDayOverlap) {
				return conflict("%s", err.Error())
			}
			return err
		}
		tc := &model.TimeCorrection{
			WorkPeriodID: wp.ID, CorrectedIn: *c.PunchIn, CorrectedOut: *c.PunchOut,
			Reason: c.Reason, CorrectedBy: by,
		}
		if err := h.Corrections.Create(ctx, tc); err != nil {
			return err
		}
		if err := compensationday.SyncClaimAfterWorkDayChange(ctx, h.FixedNonWorkWeekdays, h.WorkPeriods, h.Corrections, h.CompensationDayClaims, c.UserID, wp.WorkDate); err != nil {
			return err
		}
		logAudit(h.Audit, ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: audit.EntityTimeCorrection, EntityID: auditID(tc.ID),
			TargetUserID: auditTarget(c.UserID),
			Summary:      audit.JSONSummary(map[string]any{"work_period_id": wp.ID, "work_date": wp.WorkDate, "change_request_id": c.ID}),
		})
	case model.RequestTimeEntry:
		wp := &model.WorkPeriod{UserID: c.UserID, WorkDate: c.WorkDate, PunchIn: *c.PunchIn, PunchOut: c.PunchOut}
		if err := checkDayOverlap(ctx, h.WorkPeriods, h.Corrections, c.UserID, c.WorkDate, 0, *c.PunchIn, *c.PunchOut); err != nil {
			if errors.Is(err, errDayOverlap) {
				return conflict("Der Zeitraum überschneidet sich inzwischen mit einem vorhandenen Eintrag.")
			}
			return err
		}
		if err := h.WorkPeriods.CreateManual(ctx, wp); err != nil {
			return conflict("%s", err.Error())
		}
		if err := compensationday.SyncClaimAfterWorkDayChange(ctx, h.FixedNonWorkWeekdays, h.WorkPeriods, h.Corrections, h.CompensationDayClaims, c.UserID, c.WorkDate); err != nil {
			return err
		}
		logAudit(h.Audit, ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: audit.EntityWorkPeriod, EntityID: auditID(wp.ID),
			TargetUserID: auditTarget(c.UserID),
			Summary:      audit.JSONSummary(map[string]any{"work_date": c.WorkDate, "source": "manual", "change_request_id": c.ID}),
		})
	case model.RequestVacation:
		dates, err := h.vacationDates(ctx, c.UserID, c.DateFrom, c.DateTo)
		if err != nil {
			return err
		}
		if err := h.checkNoAbsences(ctx, c.UserID, dates); err != nil {
			return err
		}
		created := make([]int, 0, len(dates))
		for _, ds := range dates {
			a := &model.Absence{UserID: c.UserID, AbsenceDate: ds, AbsenceType: model.AbsenceVacation, HalfDay: c.HalfDay, CreatedBy: by}
			if err := h.Absences.Create(ctx, a); err != nil {
				for _, id := range created {
					_ = h.Absences.Delete(ctx, id)
				}
				return conflict("Urlaub am %s konnte nicht eingetragen werden.", formatAbsenceDateDE(ds))
			}
			created = append(created, a.ID)
		}
		for i, id := range created {
			logAudit(h.Audit, ctx, audit.Entry{
				Action: audit.ActionCreate, EntityType: audit.EntityAbsence, EntityID: auditID(id),
				TargetUserID: auditTarget(c.UserID),
				Summary:      audit.JSONSummary(map[string]any{"absence_date": dates[i], "absence_type": model.AbsenceVacation, "change_request_id": c.ID}),
			})
		}
	default:
		return badRequest("Unbekannte Antragsart.")
	}
	return nil
}
