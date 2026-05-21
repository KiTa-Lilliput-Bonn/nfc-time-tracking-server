package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/compensationday"
	"nfc-time-tracking-server/internal/service/saldocalc"
	"nfc-time-tracking-server/internal/service/timesummary"
	"nfc-time-tracking-server/internal/service/vacationbalance"
	"nfc-time-tracking-server/internal/store"
)

type MeHandler struct {
	Users       store.UserStore
	WorkPeriods store.WorkPeriodStore
	WeeklyHours store.WeeklyHoursStore
	VacationEnt store.VacationEntitlementStore
	Absences              store.AbsenceStore
	CompensationDayClaims store.CompensationDayClaimStore
	Schedules             store.ScheduleStore
	TeamMeetings          store.TeamMeetingStore
	Corrections           store.CorrectionStore
	Holidays              store.HolidayStore
	Audit                 *audit.Logger
}

func (h *MeHandler) Times(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if uid == 0 {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	from, to, err := queryDateRange(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	periods, err := h.WorkPeriods.ListByUserDateRange(r.Context(), uid, from, to)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	if periods == nil {
		periods = []model.WorkPeriod{}
	}
	worked, err := timesummary.SumWorkedHoursFromStore(r.Context(), uid, periods, h.Schedules, h.TeamMeetings)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	fixed := []int(nil)
	if h.Users != nil {
		if u, err := h.Users.GetByID(r.Context(), uid); err == nil && u != nil {
			fixed = u.FixedNonWorkWeekdays
		}
	}
	holCredits := buildHolidayCredits(r.Context(), uid, from, to, fixed, h.WeeklyHours, h.Holidays)
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"from":         from,
		"to":           to,
		"work_periods": periods,
		"worked_hours": worked,
		"holidays":     holCredits,
	})
}

type holidayCredit struct {
	HolidayDate string  `json:"holiday_date"`
	Name        string  `json:"name"`
	CreditHours float64 `json:"credit_hours"`
}

func buildHolidayCredits(ctx context.Context, userID int, from, to string, fixedNonWork []int, weekly store.WeeklyHoursStore, holidays store.HolidayStore) []holidayCredit {
	if weekly == nil || holidays == nil {
		return []holidayCredit{}
	}
	loc := time.Local
	a, err := time.ParseInLocation("2006-01-02", from, loc)
	if err != nil {
		return []holidayCredit{}
	}
	b, err := time.ParseInLocation("2006-01-02", to, loc)
	if err != nil {
		return []holidayCredit{}
	}
	a = time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, loc)
	b = time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, loc)
	out := make([]holidayCredit, 0, 8)
	for d := a; !d.After(b); d = d.AddDate(0, 0, 1) {
		if !model.IsEmployeeWorkday(d, fixedNonWork) {
			continue
		}
		ds := d.Format("2006-01-02")
		hol, err := holidays.GetForDate(ctx, ds)
		if err != nil || hol == nil || hol.ID == 0 {
			continue
		}
		wh, err := weekly.GetForDate(ctx, userID, ds)
		if err != nil || wh == nil || wh.HoursPerWeek <= 0 {
			out = append(out, holidayCredit{HolidayDate: ds, Name: hol.Name, CreditHours: 0})
			continue
		}
		out = append(out, holidayCredit{HolidayDate: ds, Name: hol.Name, CreditHours: model.DailyHours(wh.HoursPerWeek, fixedNonWork)})
	}
	return out
}

func (h *MeHandler) Balance(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if uid == 0 {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	y, m, err := queryMonthYear(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := h.Users.GetByID(r.Context(), uid)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "user query failed")
		return
	}
	mb, err := saldocalc.MonthWithOpening(r.Context(), uid, y, m, u.OpeningHoursBalance, u.FixedNonWorkWeekdays, h.WorkPeriods, h.WeeklyHours, h.Schedules)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, mb)
}

func (h *MeHandler) Vacation(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if uid == 0 {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	loc := time.Local
	nowWall := time.Now().In(loc)
	y, mo, d := nowWall.Date()
	today := time.Date(y, mo, d, 0, 0, 0, 0, loc)

	vb, err := vacationbalance.ComputeForUser(r.Context(), uid, h.Users, h.VacationEnt, h.Absences, today)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, vb)
}

// Profile liefert felder für die eigene Urlaubs-/Kalenderdarstellung (z. B. feste freie Wochentage).
func (h *MeHandler) Profile(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if uid == 0 {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	u, err := h.Users.GetByID(r.Context(), uid)
	if err != nil || u == nil {
		response.Error(w, http.StatusInternalServerError, "user query failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"fixed_non_work_weekdays": u.FixedNonWorkWeekdays,
	})
}

func (h *MeHandler) Schedule(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if uid == 0 {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	from, to, err := queryDateRange(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	list, err := schedulesForUserRange(r.Context(), h.Schedules, uid, from, to)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	var teamMeetings []model.TeamMeeting
	if h.TeamMeetings != nil {
		tm, err := h.TeamMeetings.ListForUserInDateRange(r.Context(), uid, from, to)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		teamMeetings = tm
	}
	if teamMeetings == nil {
		teamMeetings = []model.TeamMeeting{}
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"from": from, "to": to, "schedules": list, "team_meetings": teamMeetings,
	})
}

// ListAbsences lists the authenticated user's absences in a date range (for UI: vacation/sick markers).
func (h *MeHandler) ListAbsences(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if uid == 0 {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	from, to, err := queryDateRange(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	list, err := h.Absences.ListByUserDateRange(r.Context(), uid, from, to)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"from": from, "to": to, "absences": list,
	})
}

// ListCorrections lists the authenticated user's time corrections in a date range (for UI tables).
func (h *MeHandler) ListCorrections(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if uid == 0 {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	from, to, err := queryDateRange(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	list, err := h.Corrections.ListByUser(r.Context(), uid, from, to)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	if list == nil {
		list = []model.TimeCorrection{}
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"from": from, "to": to, "corrections": list,
	})
}

// CreateCorrection creates a time correction for a work period owned by the authenticated user.
// Logic mirrors employees.CreateCorrection, but the target user is always the caller.
func (h *MeHandler) CreateCorrection(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserID(r)
	if uid == 0 {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body correctionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Reason == "" {
		response.Error(w, http.StatusBadRequest, "reason required")
		return
	}
	if !body.CorrectedOut.After(body.CorrectedIn) {
		response.Error(w, http.StatusBadRequest, "corrected_out must be after corrected_in")
		return
	}
	c := &model.TimeCorrection{
		WorkPeriodID: body.WorkPeriodID, CorrectedIn: body.CorrectedIn, CorrectedOut: body.CorrectedOut,
		Reason: body.Reason, CorrectedBy: uid,
	}
	target, err := h.WorkPeriods.GetByID(r.Context(), body.WorkPeriodID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	if target == nil || target.UserID != uid {
		response.Error(w, http.StatusBadRequest, "invalid work_period_id")
		return
	}
	day := target.WorkDate
	dayPeriods, err := h.WorkPeriods.ListByUserDateRange(r.Context(), uid, day, day)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	newStart := body.CorrectedIn.UTC()
	newEnd := body.CorrectedOut.UTC()
	for _, p := range dayPeriods {
		if p.ID == body.WorkPeriodID {
			continue
		}
		start := p.PunchIn.UTC()
		end := p.PunchOut
		if corr, err := h.Corrections.GetLatestForPeriod(r.Context(), p.ID); err == nil && corr != nil {
			start = corr.CorrectedIn.UTC()
			cend := corr.CorrectedOut.UTC()
			end = &cend
		}
		if end != nil && !end.After(start) {
			continue
		}
		if start.Before(newEnd) && (end == nil || newStart.Before(end.UTC())) {
			response.Error(w, http.StatusBadRequest, "die korrigierte Zeit überschneidet sich mit einem anderen Eintrag an diesem Tag")
			return
		}
	}

	if err := h.Corrections.Create(r.Context(), c); err != nil {
		response.Error(w, http.StatusInternalServerError, "create failed")
		return
	}
	if err := compensationday.SyncClaimAfterWorkDayChange(r.Context(), h.Users, h.WorkPeriods, h.Corrections, h.CompensationDayClaims, uid, day); err != nil {
		response.Error(w, http.StatusInternalServerError, "Ausgleichstag-Anspruch konnte nicht aktualisiert werden")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionCreate, EntityType: audit.EntityTimeCorrection, EntityID: auditID(c.ID),
		TargetUserID: auditTarget(uid),
		Summary:      audit.JSONSummary(map[string]any{"work_period_id": body.WorkPeriodID, "work_date": day, "self_service": true}),
	})
	response.JSON(w, http.StatusCreated, c)
}
