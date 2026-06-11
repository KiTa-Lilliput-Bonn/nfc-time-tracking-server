package handler

import (
	"context"
	"database/sql"
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
	authsvc "nfc-time-tracking-server/internal/service/auth"
	"nfc-time-tracking-server/internal/service/compensationday"
	"nfc-time-tracking-server/internal/service/entrylock"
	"nfc-time-tracking-server/internal/service/fixednonwork"
	"nfc-time-tracking-server/internal/service/saldocalc"
	"nfc-time-tracking-server/internal/service/timesummary"
	"nfc-time-tracking-server/internal/service/vacationbalance"
	"nfc-time-tracking-server/internal/service/vacationentitlement"
	"nfc-time-tracking-server/internal/store"
)

type EmployeeHandler struct {
	Users       store.UserStore
	Groups      store.GroupStore
	Auth        *authsvc.Service
	WorkPeriods store.WorkPeriodStore
	Corrections store.CorrectionStore
	Absences               store.AbsenceStore
	CompensationDayClaims  store.CompensationDayClaimStore
	Holidays               store.HolidayStore
	ClosureDays            store.ClosureDayStore
	WeeklyHours            store.WeeklyHoursStore
	FixedNonWorkWeekdays   store.FixedNonWorkWeekdaysStore
	ScheduleBound          store.ScheduleBoundStore
	VacationEnt            store.VacationEntitlementStore
	NFCTags     store.NFCTagStore
	Schedules   store.ScheduleStore
	TeamMeetings store.TeamMeetingStore
	Audit       *audit.Logger
}

func (h *EmployeeHandler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.Users.List(r.Context(), false)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{"employees": employeesJSONForList(r.Context(), h.FixedNonWorkWeekdays, users)})
}

type createEmployeeBody struct {
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	Role        model.Role `json:"role"`
}

func (h *EmployeeHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createEmployeeBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Username == "" || body.DisplayName == "" {
		response.Error(w, http.StatusBadRequest, "username and display_name required")
		return
	}
	role := body.Role
	if role == "" {
		role = model.RoleUser
	}
	if err := h.enforceEmployeeRole(r, role); err != nil {
		response.Error(w, http.StatusForbidden, err.Error())
		return
	}
	pw := authsvc.GenerateRandomPassword(14)
	hash, err := h.Auth.HashPassword(pw)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "hash error")
		return
	}
	u := &model.User{
		Username: body.Username, DisplayName: body.DisplayName, Role: role,
		PasswordHash: hash, Active: true, MustChangePassword: true,
		DefaultTeamMeetingParticipant: true,
	}
	if err := h.Users.Create(r.Context(), u); err != nil {
		response.Error(w, http.StatusBadRequest, "create failed (duplicate username?)")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionCreate, EntityType: audit.EntityEmployee, EntityID: auditID(u.ID),
		TargetUserID: auditTarget(u.ID),
		Summary:      audit.JSONSummary(map[string]any{"username": u.Username, "role": u.Role}),
	})
	response.JSON(w, http.StatusCreated, map[string]interface{}{
		"user": u, "temporary_password": pw,
	})
}

type patchEmployeeBody struct {
	DisplayName *string     `json:"display_name"`
	Role        *model.Role `json:"role"`
	Active      *bool       `json:"active"`
	DefaultTeamMeetingParticipant *bool `json:"default_team_meeting_participant"`
	// Start-Salden (Import), optional
	OpeningHoursBalance *float64 `json:"opening_hours_balance"`
	OpeningVacationDays *float64 `json:"opening_vacation_days"`
	GroupID             OptionalPatchInt `json:"group_id"`
}

func (h *EmployeeHandler) Patch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body patchEmployeeBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	u, err := h.Users.GetByID(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	if !model.ActorMayManageUser(middleware.Role(r), u) {
		response.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	if body.DisplayName != nil {
		u.DisplayName = *body.DisplayName
	}
	if body.Active != nil {
		u.Active = *body.Active
	}
	if body.DefaultTeamMeetingParticipant != nil {
		u.DefaultTeamMeetingParticipant = *body.DefaultTeamMeetingParticipant
	}
	if body.Role != nil {
		if middleware.Role(r) != string(model.RoleSuperadmin) {
			response.Error(w, http.StatusForbidden, "only superadmin may change role")
			return
		}
		if *body.Role == model.RoleSuperadmin {
			response.Error(w, http.StatusBadRequest, "use /users to manage superadmin")
			return
		}
		u.Role = *body.Role
	}
	if body.OpeningHoursBalance != nil {
		u.OpeningHoursBalance = *body.OpeningHoursBalance
	}
	if body.OpeningVacationDays != nil {
		u.OpeningVacationDays = *body.OpeningVacationDays
	}
	if body.GroupID.Sent {
		if h.Groups == nil {
			response.Error(w, http.StatusInternalServerError, "groups unavailable")
			return
		}
		if body.GroupID.Value != nil {
			if _, err := h.Groups.GetByID(r.Context(), *body.GroupID.Value); err != nil {
				response.Error(w, http.StatusBadRequest, "invalid group_id")
				return
			}
			gid := *body.GroupID.Value
			u.GroupID = &gid
		} else {
			u.GroupID = nil
		}
	}
	if err := h.Users.Update(r.Context(), u); err != nil {
		response.Error(w, http.StatusInternalServerError, "update failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityEmployee, EntityID: auditID(u.ID),
		TargetUserID: auditTarget(u.ID),
		Summary:      audit.JSONSummary(map[string]any{"username": u.Username, "role": u.Role, "active": u.Active}),
	})
	response.JSON(w, http.StatusOK, employeeJSONForUser(r.Context(), h.FixedNonWorkWeekdays, *u, time.Now().In(time.Local).Format("2006-01-02")))
}

func (h *EmployeeHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseEmployeeIDForWrite(w, r)
	if !ok {
		return
	}
	pw := authsvc.GenerateRandomPassword(14)
	hash, err := h.Auth.HashPassword(pw)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "hash error")
		return
	}
	if err := h.Users.SetPassword(r.Context(), id, hash, true); err != nil {
		response.Error(w, http.StatusInternalServerError, "update failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityEmployee, EntityID: auditID(id),
		TargetUserID: auditTarget(id), Summary: `{"reset_password":true}`,
	})
	u, err := h.Users.GetByID(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"user": u, "temporary_password": pw,
	})
}

func (h *EmployeeHandler) Times(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeID(w, r)
	if !ok {
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
	worked, err := timesummary.SumWorkedHoursFromStore(r.Context(), uid, periods, h.Schedules, h.TeamMeetings, h.ScheduleBound)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	holCredits := buildHolidayCredits(r.Context(), uid, from, to, h.FixedNonWorkWeekdays, h.WeeklyHours, h.Holidays)
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"from": from, "to": to, "work_periods": periods,
		"worked_hours": worked,
		"holidays":     holCredits,
	})
}

func (h *EmployeeHandler) Schedule(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeID(w, r)
	if !ok {
		return
	}
	from, to, err := queryDateRange(r)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.Schedules == nil {
		response.JSON(w, http.StatusOK, map[string]interface{}{
			"from": from, "to": to, "schedules": []model.Schedule{},
		})
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

func (h *EmployeeHandler) Balance(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeID(w, r)
	if !ok {
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
	mb, err := saldocalc.MonthWithOpening(
		r.Context(),
		uid,
		y,
		m,
		u.OpeningHoursBalance,
		h.FixedNonWorkWeekdays,
		h.WorkPeriods,
		h.WeeklyHours,
		h.Holidays,
		h.Schedules,
		h.ScheduleBound,
	)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, mb)
}

func (h *EmployeeHandler) Vacation(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeID(w, r)
	if !ok {
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

// workPeriodBody: is_break im JSON wird ignoriert — manuelle Zeiten sind immer Arbeitsintervalle.
type workPeriodBody struct {
	WorkDate string     `json:"work_date"`
	PunchIn  time.Time  `json:"punch_in"`
	PunchOut *time.Time `json:"punch_out"`
	IsBreak  bool       `json:"is_break"`
}

func (h *EmployeeHandler) CreateWorkPeriod(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeIDForWrite(w, r)
	if !ok {
		return
	}
	var body workPeriodBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.WorkDate == "" {
		response.Error(w, http.StatusBadRequest, "work_date required")
		return
	}
	wd, err := time.ParseInLocation("2006-01-02", body.WorkDate, time.Local)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid work_date")
		return
	}
	now := time.Now().In(time.Local)
	today0 := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	if wd.After(today0) {
		response.Error(w, http.StatusBadRequest, "work_date must not be in the future")
		return
	}
	wp := &model.WorkPeriod{UserID: uid, WorkDate: body.WorkDate, PunchIn: body.PunchIn, PunchOut: body.PunchOut, IsBreak: false}
	if err := h.WorkPeriods.CreateManual(r.Context(), wp); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := compensationday.SyncClaimAfterWorkDayChange(r.Context(), h.FixedNonWorkWeekdays, h.WorkPeriods, h.Corrections, h.CompensationDayClaims, uid, body.WorkDate); err != nil {
		response.Error(w, http.StatusInternalServerError, "Ausgleichstag-Anspruch konnte nicht aktualisiert werden")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionCreate, EntityType: audit.EntityWorkPeriod, EntityID: auditID(wp.ID),
		TargetUserID: auditTarget(uid),
		Summary:      audit.JSONSummary(map[string]any{"work_date": body.WorkDate, "source": "manual"}),
	})
	response.JSON(w, http.StatusCreated, wp)
}

func (h *EmployeeHandler) DeleteWorkPeriod(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeIDForWrite(w, r)
	if !ok {
		return
	}
	wpid, err := strconv.Atoi(chi.URLParam(r, "wpId"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid wp id")
		return
	}
	// ensure period belongs to user
	from := "1970-01-01"
	to := "2099-12-31"
	periods, err := h.WorkPeriods.ListByUserDateRange(r.Context(), uid, from, to)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	var workDate string
	found := false
	for _, p := range periods {
		if p.ID == wpid {
			found = true
			workDate = p.WorkDate
			break
		}
	}
	if !found {
		response.Error(w, http.StatusNotFound, "work period not found")
		return
	}
	if err := h.WorkPeriods.DeleteManual(r.Context(), wpid); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := compensationday.SyncClaimAfterWorkDayChange(r.Context(), h.FixedNonWorkWeekdays, h.WorkPeriods, h.Corrections, h.CompensationDayClaims, uid, workDate); err != nil {
		response.Error(w, http.StatusInternalServerError, "Ausgleichstag-Anspruch konnte nicht aktualisiert werden")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionDelete, EntityType: audit.EntityWorkPeriod, EntityID: auditID(wpid),
		TargetUserID: auditTarget(uid),
		Summary:      audit.JSONSummary(map[string]any{"work_date": workDate}),
	})
	w.WriteHeader(http.StatusNoContent)
}

type correctionBody struct {
	WorkPeriodID int       `json:"work_period_id"`
	CorrectedIn  time.Time `json:"corrected_in"`
	CorrectedOut time.Time `json:"corrected_out"`
	Reason       string    `json:"reason"`
}

func (h *EmployeeHandler) CreateCorrection(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeIDForWrite(w, r)
	if !ok {
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
	by := middleware.UserID(r)
	c := &model.TimeCorrection{
		WorkPeriodID: body.WorkPeriodID, CorrectedIn: body.CorrectedIn, CorrectedOut: body.CorrectedOut,
		Reason: body.Reason, CorrectedBy: by,
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

	// Prevent corrections from creating overlapping work periods on the same day.
	// We compare the *effective* intervals (latest correction if present) of all periods on that work_date.
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
		// Ignore invalid existing intervals; overlap check is best-effort here.
		if end != nil && !end.After(start) {
			continue
		}
		// Overlap condition: existing_start < new_end && new_start < existing_end (nil end = open-ended).
		if start.Before(newEnd) && (end == nil || newStart.Before(end.UTC())) {
			response.Error(w, http.StatusBadRequest, "die korrigierte Zeit überschneidet sich mit einem anderen Eintrag an diesem Tag")
			return
		}
	}

	if err := h.Corrections.Create(r.Context(), c); err != nil {
		response.Error(w, http.StatusInternalServerError, "create failed")
		return
	}
	if err := compensationday.SyncClaimAfterWorkDayChange(r.Context(), h.FixedNonWorkWeekdays, h.WorkPeriods, h.Corrections, h.CompensationDayClaims, uid, day); err != nil {
		response.Error(w, http.StatusInternalServerError, "Ausgleichstag-Anspruch konnte nicht aktualisiert werden")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionCreate, EntityType: audit.EntityTimeCorrection, EntityID: auditID(c.ID),
		TargetUserID: auditTarget(uid),
		Summary:      audit.JSONSummary(map[string]any{"work_period_id": body.WorkPeriodID, "work_date": day}),
	})
	response.JSON(w, http.StatusCreated, c)
}

func (h *EmployeeHandler) ListCorrections(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeID(w, r)
	if !ok {
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
	response.JSON(w, http.StatusOK, map[string]interface{}{"corrections": list})
}

type absenceBody struct {
	AbsenceDate string            `json:"absence_date"`
	AbsenceType model.AbsenceType `json:"absence_type"`
	HalfDay     bool              `json:"half_day"`
}

func absenceTypeLabelDE(t model.AbsenceType) string {
	switch t {
	case model.AbsenceSick:
		return "Krankmeldung"
	case model.AbsenceVacation:
		return "Urlaub"
	case model.AbsenceOther:
		return "Sonstiges"
	case model.AbsenceCompensationDay:
		return "Ausgleichstag"
	default:
		return "Abwesenheit"
	}
}

func formatAbsenceDateDE(dateStr string) string {
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return dateStr
	}
	return t.Format("02.01.2006")
}

// duplicateAbsenceUserMessage erklärt den Unique-Konflikt pro Kalendertag (ohne SQLite-Text).
func duplicateAbsenceUserMessage(dateStr string, existingType *model.AbsenceType) string {
	pretty := formatAbsenceDateDE(dateStr)
	if existingType == nil {
		return fmt.Sprintf("Für den %s ist bereits eine Abwesenheit eingetragen.", pretty)
	}
	return fmt.Sprintf(
		"Für den %s ist bereits eine Abwesenheit eingetragen (%s).",
		pretty,
		absenceTypeLabelDE(*existingType),
	)
}

func isAbsenceDateUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique constraint failed") && strings.Contains(s, "absences")
}

var germanWeekdayName = map[time.Weekday]string{
	time.Sunday:    "Sonntag",
	time.Monday:    "Montag",
	time.Tuesday:   "Dienstag",
	time.Wednesday: "Mittwoch",
	time.Thursday:  "Donnerstag",
	time.Friday:    "Freitag",
	time.Saturday:  "Samstag",
}

func validateVacationAbsenceDate(ctx context.Context, holidays store.HolidayStore, closures store.ClosureDayStore, fixedNonWork []int, dateStr string) error {
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return errors.New("Ungültiges Datum: Bitte ein gültiges Kalenderdatum im Format JJJJ-MM-TT angeben.")
	}
	if !model.IsEmployeeWorkday(t, fixedNonWork) {
		wd := t.Weekday()
		if wd == time.Saturday || wd == time.Sunday {
			return fmt.Errorf(
				"Am %s, %s, kann kein Urlaub gebucht werden — das ist ein Wochenende.",
				germanWeekdayName[wd], t.Format("02.01.2006"),
			)
		}
		return fmt.Errorf(
			"Am %s, %s, kann kein Urlaub gebucht werden — das ist ein fester freier Wochentag (Dienstplan).",
			germanWeekdayName[wd], t.Format("02.01.2006"),
		)
	}
	wd := t.Weekday()
	h, err := holidays.GetForDate(ctx, dateStr)
	if err != nil {
		return err
	}
	if h != nil {
		return fmt.Errorf(
			"Am %s, %s, kann kein Urlaub gebucht werden — „%s“ ist ein gesetzlicher Feiertag.",
			germanWeekdayName[wd], t.Format("02.01.2006"), h.Name,
		)
	}
	if closures != nil {
		clo, err := closures.GetForDate(ctx, dateStr)
		if err != nil {
			return err
		}
		if clo != nil && clo.ID != 0 {
			return fmt.Errorf(
				"Am %s, %s, kann kein Urlaub gebucht werden — „%s“ ist ein Schließtag.",
				germanWeekdayName[wd], t.Format("02.01.2006"), clo.Name,
			)
		}
	}
	return nil
}

func validateCompensationDayAbsenceDate(ctx context.Context, holidays store.HolidayStore, fixedNonWork []int, dateStr string, halfDay bool) error {
	if halfDay {
		return errors.New("Halbe Ausgleichstage sind nicht möglich.")
	}
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return errors.New("Ungültiges Datum: Bitte ein gültiges Kalenderdatum im Format JJJJ-MM-TT angeben.")
	}
	if !model.IsEmployeeWorkday(t, fixedNonWork) {
		wd := t.Weekday()
		if wd == time.Saturday || wd == time.Sunday {
			return fmt.Errorf(
				"Am %s, %s, kann kein Ausgleichstag gebucht werden — das ist ein Wochenende.",
				germanWeekdayName[wd], t.Format("02.01.2006"),
			)
		}
		return fmt.Errorf(
			"Am %s, %s, kann kein Ausgleichstag gebucht werden — das ist ein fester freier Wochentag (Dienstplan).",
			germanWeekdayName[wd], t.Format("02.01.2006"),
		)
	}
	wd := t.Weekday()
	h, err := holidays.GetForDate(ctx, dateStr)
	if err != nil {
		return err
	}
	if h != nil {
		return fmt.Errorf(
			"Am %s, %s, kann kein Ausgleichstag gebucht werden — „%s“ ist ein gesetzlicher Feiertag.",
			germanWeekdayName[wd], t.Format("02.01.2006"), h.Name,
		)
	}
	return nil
}

func (h *EmployeeHandler) CreateAbsence(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeIDForWrite(w, r)
	if !ok {
		return
	}
	var body absenceBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.AbsenceDate == "" || body.AbsenceType == "" {
		response.Error(w, http.StatusBadRequest, "absence_date and absence_type required")
		return
	}
	targetUser, err := h.Users.GetByID(r.Context(), uid)
	if err != nil || targetUser == nil {
		response.Error(w, http.StatusInternalServerError, "user query failed")
		return
	}
	fixed := fixednonwork.WeekdaysForUserDate(r.Context(), h.FixedNonWorkWeekdays, uid, body.AbsenceDate)
	if body.AbsenceType == model.AbsenceVacation {
		if err := validateVacationAbsenceDate(r.Context(), h.Holidays, h.ClosureDays, fixed, body.AbsenceDate); err != nil {
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	var pendingClaim *model.CompensationDayClaim
	if body.AbsenceType == model.AbsenceCompensationDay {
		if err := validateCompensationDayAbsenceDate(r.Context(), h.Holidays, fixed, body.AbsenceDate, body.HalfDay); err != nil {
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		if h.CompensationDayClaims == nil {
			response.Error(w, http.StatusInternalServerError, "Ausgleichstag-Anspruch nicht konfiguriert")
			return
		}
		claim, err := h.CompensationDayClaims.GetOldestOpen(r.Context(), uid)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Ausgleichstag-Anspruch konnte nicht geprüft werden")
			return
		}
		if claim == nil {
			response.Error(w, http.StatusBadRequest, "Für diesen Mitarbeiter ist kein offener Ausgleichstag-Anspruch vorhanden.")
			return
		}
		pendingClaim = claim
	}
	existing, err := h.Absences.GetForUserDate(r.Context(), uid, body.AbsenceDate)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	if existing != nil {
		t := existing.AbsenceType
		response.Error(w, http.StatusBadRequest, duplicateAbsenceUserMessage(body.AbsenceDate, &t))
		return
	}
	a := &model.Absence{
		UserID: uid, AbsenceDate: body.AbsenceDate, AbsenceType: body.AbsenceType,
		HalfDay: body.HalfDay, CreatedBy: middleware.UserID(r),
	}
	if err := h.Absences.Create(r.Context(), a); err != nil {
		if isAbsenceDateUniqueViolation(err) {
			ex, qerr := h.Absences.GetForUserDate(r.Context(), uid, body.AbsenceDate)
			if qerr == nil && ex != nil {
				tt := ex.AbsenceType
				response.Error(w, http.StatusBadRequest, duplicateAbsenceUserMessage(body.AbsenceDate, &tt))
				return
			}
			response.Error(w, http.StatusBadRequest, duplicateAbsenceUserMessage(body.AbsenceDate, nil))
			return
		}
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if pendingClaim != nil {
		if err := h.CompensationDayClaims.MarkUsed(r.Context(), pendingClaim.ID, a.ID); err != nil {
			_ = h.Absences.Delete(r.Context(), a.ID)
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionCreate, EntityType: audit.EntityAbsence, EntityID: auditID(a.ID),
		TargetUserID: auditTarget(uid),
		Summary:      audit.JSONSummary(map[string]any{"absence_date": body.AbsenceDate, "absence_type": body.AbsenceType}),
	})
	response.JSON(w, http.StatusCreated, a)
}

func (h *EmployeeHandler) ListAbsences(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeID(w, r)
	if !ok {
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
	response.JSON(w, http.StatusOK, map[string]interface{}{"absences": list})
}

func (h *EmployeeHandler) DeleteAbsence(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeIDForWrite(w, r)
	if !ok {
		return
	}
	aid, err := strconv.Atoi(chi.URLParam(r, "absenceId"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid absence id")
		return
	}
	a, err := h.Absences.GetByID(r.Context(), aid)
	if err != nil || a == nil {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	if a.UserID != uid {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	isCompensationDay := a.AbsenceType == model.AbsenceCompensationDay
	if isCompensationDay && h.CompensationDayClaims != nil {
		if err := h.CompensationDayClaims.ReopenByAbsenceID(r.Context(), aid); err != nil {
			response.Error(w, http.StatusInternalServerError, "Ausgleichstag-Anspruch konnte nicht zurückgesetzt werden")
			return
		}
	}
	if err := h.Absences.Delete(r.Context(), aid); err != nil {
		response.Error(w, http.StatusInternalServerError, "delete failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionDelete, EntityType: audit.EntityAbsence, EntityID: auditID(aid),
		TargetUserID: auditTarget(uid),
		Summary:      audit.JSONSummary(map[string]any{"absence_date": a.AbsenceDate, "absence_type": a.AbsenceType}),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *EmployeeHandler) ListCompensationDayClaims(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeID(w, r)
	if !ok {
		return
	}
	if h.CompensationDayClaims == nil {
		response.Error(w, http.StatusInternalServerError, "Ausgleichstag-Anspruch nicht konfiguriert")
		return
	}
	var status *model.CompensationDayClaimStatus
	if raw := r.URL.Query().Get("status"); raw != "" {
		s := model.CompensationDayClaimStatus(raw)
		if s != model.CompensationDayClaimOpen && s != model.CompensationDayClaimUsed && s != model.CompensationDayClaimWaived {
			response.Error(w, http.StatusBadRequest, "invalid status")
			return
		}
		status = &s
	}
	list, err := h.CompensationDayClaims.ListByUser(r.Context(), uid, status)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	if list == nil {
		list = []model.CompensationDayClaim{}
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{"compensation_day_claims": list})
}

func (h *EmployeeHandler) WaiveCompensationDayClaim(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeIDForWrite(w, r)
	if !ok {
		return
	}
	if h.CompensationDayClaims == nil {
		response.Error(w, http.StatusInternalServerError, "Ausgleichstag-Anspruch nicht konfiguriert")
		return
	}
	claimID, err := strconv.Atoi(chi.URLParam(r, "claimId"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid claim id")
		return
	}
	if err := h.CompensationDayClaims.Waive(r.Context(), uid, claimID); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityCompensationDayClaim, EntityID: auditID(claimID),
		TargetUserID: auditTarget(uid), Summary: `{"waived":true}`,
	})
	w.WriteHeader(http.StatusNoContent)
}

type weeklyHoursBody struct {
	HoursPerWeek float64 `json:"hours_per_week"`
	ValidFrom    string  `json:"valid_from"`
}

type weeklyHoursResponse struct {
	model.WeeklyHours
	Mutable bool `json:"mutable"`
}

type vacationEntitlementResponse struct {
	model.VacationEntitlement
	Mutable bool `json:"mutable"`
}

func weeklyHoursResponses(list []model.WeeklyHours) []weeklyHoursResponse {
	now := time.Now().UTC()
	out := make([]weeklyHoursResponse, len(list))
	for i, row := range list {
		out[i] = weeklyHoursResponse{
			WeeklyHours: row,
			Mutable:     entrylock.IsMutable(row.CreatedAt, now),
		}
	}
	return out
}

func vacationEntitlementResponses(list []model.VacationEntitlement) []vacationEntitlementResponse {
	now := time.Now().UTC()
	out := make([]vacationEntitlementResponse, len(list))
	for i, row := range list {
		out[i] = vacationEntitlementResponse{
			VacationEntitlement: row,
			Mutable:             entrylock.IsMutable(row.CreatedAt, now),
		}
	}
	return out
}

func (h *EmployeeHandler) enforceMutableEntry(w http.ResponseWriter, r *http.Request, createdAt time.Time) bool {
	role := middleware.Role(r)
	if role == string(model.RoleSuperadmin) || role == string(model.RoleLeitung) {
		return true
	}
	if entrylock.IsMutable(createdAt, time.Now().UTC()) {
		return true
	}
	response.Error(w, http.StatusForbidden, "entry cannot be changed after 24 hours")
	return false
}

func (h *EmployeeHandler) PutWeeklyHours(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeIDForWrite(w, r)
	if !ok {
		return
	}
	var body weeklyHoursBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.ValidFrom == "" || body.HoursPerWeek <= 0 {
		response.Error(w, http.StatusBadRequest, "valid_from and hours_per_week required")
		return
	}
	wh := &model.WeeklyHours{UserID: uid, HoursPerWeek: body.HoursPerWeek, ValidFrom: body.ValidFrom}
	if err := h.WeeklyHours.Set(r.Context(), wh); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityWeeklyHours, EntityID: auditID(wh.ID),
		TargetUserID: auditTarget(uid),
		Summary:      audit.JSONSummary(map[string]any{"hours_per_week": body.HoursPerWeek, "valid_from": body.ValidFrom}),
	})
	resp := weeklyHoursResponse{
		WeeklyHours: *wh,
		Mutable:     entrylock.IsMutable(wh.CreatedAt, time.Now().UTC()),
	}
	response.JSON(w, http.StatusOK, resp)
}

func (h *EmployeeHandler) GetWeeklyHours(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeID(w, r)
	if !ok {
		return
	}
	list, err := h.WeeklyHours.ListByUser(r.Context(), uid)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{"weekly_hours": weeklyHoursResponses(list)})
}

func (h *EmployeeHandler) DeleteWeeklyHours(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeIDForWrite(w, r)
	if !ok {
		return
	}
	whID, err := strconv.Atoi(chi.URLParam(r, "whId"))
	if err != nil || whID <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid weekly hours id")
		return
	}
	row, err := h.WeeklyHours.GetByID(r.Context(), uid, whID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	if row == nil {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	if !h.enforceMutableEntry(w, r, row.CreatedAt) {
		return
	}
	if err := h.WeeklyHours.Delete(r.Context(), uid, whID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.Error(w, http.StatusNotFound, "not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "delete failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionDelete, EntityType: audit.EntityWeeklyHours, EntityID: auditID(whID),
		TargetUserID: auditTarget(uid),
	})
	w.WriteHeader(http.StatusNoContent)
}

type vacationEntBody struct {
	DaysPerYear float64 `json:"days_per_year"`
	ValidFrom   string  `json:"valid_from"`
}

func (h *EmployeeHandler) PutVacationEntitlement(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeIDForWrite(w, r)
	if !ok {
		return
	}
	var body vacationEntBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.ValidFrom == "" || body.DaysPerYear < 0 {
		response.Error(w, http.StatusBadRequest, "valid_from and days_per_year required")
		return
	}
	vfNorm, err := vacationentitlement.ParseValidFromCalendarDay(body.ValidFrom)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	ve := &model.VacationEntitlement{UserID: uid, DaysPerYear: body.DaysPerYear, ValidFrom: vfNorm}
	if err := h.VacationEnt.Set(r.Context(), ve); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityVacationEntitlement, EntityID: auditID(ve.ID),
		TargetUserID: auditTarget(uid),
		Summary:      audit.JSONSummary(map[string]any{"days_per_year": body.DaysPerYear, "valid_from": body.ValidFrom}),
	})
	resp := vacationEntitlementResponse{
		VacationEntitlement: *ve,
		Mutable:             entrylock.IsMutable(ve.CreatedAt, time.Now().UTC()),
	}
	response.JSON(w, http.StatusOK, resp)
}

func (h *EmployeeHandler) GetVacationEntitlement(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeID(w, r)
	if !ok {
		return
	}
	list, err := h.VacationEnt.ListByUser(r.Context(), uid)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{"vacation_entitlements": vacationEntitlementResponses(list)})
}

func (h *EmployeeHandler) DeleteVacationEntitlement(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeIDForWrite(w, r)
	if !ok {
		return
	}
	veID, err := strconv.Atoi(chi.URLParam(r, "veId"))
	if err != nil || veID <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid vacation entitlement id")
		return
	}
	row, err := h.VacationEnt.GetByID(r.Context(), uid, veID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	if row == nil {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	if !h.enforceMutableEntry(w, r, row.CreatedAt) {
		return
	}
	if err := h.VacationEnt.Delete(r.Context(), uid, veID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.Error(w, http.StatusNotFound, "not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "delete failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionDelete, EntityType: audit.EntityVacationEntitlement, EntityID: auditID(veID),
		TargetUserID: auditTarget(uid),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *EmployeeHandler) PutFixedNonWorkWeekdays(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeIDForWrite(w, r)
	if !ok {
		return
	}
	var body fixedNonWorkWeekdaysBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.ValidFrom == "" {
		response.Error(w, http.StatusBadRequest, "valid_from required")
		return
	}
	if err := model.ValidateFixedNonWorkWeekdays(body.Weekdays); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	row := &model.FixedNonWorkWeekdays{UserID: uid, Weekdays: append([]int(nil), body.Weekdays...), ValidFrom: body.ValidFrom}
	if err := h.FixedNonWorkWeekdays.Set(r.Context(), row); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityFixedNonWorkWeekdays, EntityID: auditID(row.ID),
		TargetUserID: auditTarget(uid),
		Summary:      audit.JSONSummary(map[string]any{"weekdays": body.Weekdays, "valid_from": body.ValidFrom}),
	})
	resp := fixedNonWorkWeekdaysResponse{
		FixedNonWorkWeekdays: *row,
		Mutable:              entrylock.IsMutable(row.CreatedAt, time.Now().UTC()),
	}
	response.JSON(w, http.StatusOK, resp)
}

func (h *EmployeeHandler) GetFixedNonWorkWeekdays(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeID(w, r)
	if !ok {
		return
	}
	list, err := h.FixedNonWorkWeekdays.ListByUser(r.Context(), uid)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{"fixed_non_work_weekdays": fixedNonWorkWeekdaysResponses(list)})
}

func (h *EmployeeHandler) DeleteFixedNonWorkWeekdays(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeIDForWrite(w, r)
	if !ok {
		return
	}
	rowID, err := strconv.Atoi(chi.URLParam(r, "fnwId"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	row, err := h.FixedNonWorkWeekdays.GetByID(r.Context(), uid, rowID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	if row == nil {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	if !h.enforceMutableEntry(w, r, row.CreatedAt) {
		return
	}
	if err := h.FixedNonWorkWeekdays.Delete(r.Context(), uid, rowID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.Error(w, http.StatusNotFound, "not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "delete failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionDelete, EntityType: audit.EntityFixedNonWorkWeekdays, EntityID: auditID(rowID),
		TargetUserID: auditTarget(uid),
	})
	w.WriteHeader(http.StatusNoContent)
}

type nfcTagBody struct {
	TagUID       string `json:"tag_uid"`
	AssignedFrom string `json:"assigned_from"`
}

func (h *EmployeeHandler) PostNFCTag(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeIDForWrite(w, r)
	if !ok {
		return
	}
	var body nfcTagBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.TagUID == "" || body.AssignedFrom == "" {
		response.Error(w, http.StatusBadRequest, "tag_uid and assigned_from required")
		return
	}
	u, err := h.Users.GetByID(r.Context(), uid)
	if err != nil || !model.RoleMayHaveNFCTag(u.Role) {
		response.Error(w, http.StatusBadRequest, "nfc tags only for role user or leitung")
		return
	}
	tag := &model.NFCTag{TagUID: body.TagUID, UserID: uid, AssignedFrom: body.AssignedFrom}
	if err := h.NFCTags.Assign(r.Context(), tag); err != nil {
		var tagConflict *store.NFCTagAssignedError
		if errors.As(err, &tagConflict) {
			response.Error(w, http.StatusConflict, tagConflict.Error())
			return
		}
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionCreate, EntityType: audit.EntityNFCTag, EntityID: auditID(tag.ID),
		TargetUserID: auditTarget(uid),
		Summary:      audit.JSONSummary(map[string]any{"tag_uid": body.TagUID, "assigned_from": body.AssignedFrom}),
	})
	response.JSON(w, http.StatusCreated, tag)
}

func (h *EmployeeHandler) ListNFCTags(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeID(w, r)
	if !ok {
		return
	}
	list, err := h.NFCTags.ListByUser(r.Context(), uid)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{"nfc_tags": list})
}

func (h *EmployeeHandler) parseEmployeeID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	if _, err := h.Users.GetByID(r.Context(), id); err != nil {
		response.Error(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

// parseEmployeeIDForWrite loads the employee and returns 403 if the actor may not manage that account
// (e.g. Leitung must not modify superadmin).
func (h *EmployeeHandler) parseEmployeeIDForWrite(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	u, err := h.Users.GetByID(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusNotFound, "not found")
		return 0, false
	}
	if !model.ActorMayManageUser(middleware.Role(r), u) {
		response.Error(w, http.StatusForbidden, "forbidden")
		return 0, false
	}
	return id, true
}

func (h *EmployeeHandler) enforceEmployeeRole(r *http.Request, role model.Role) error {
	actor := middleware.Role(r)
	if role == model.RoleSuperadmin {
		return fmt.Errorf("invalid role")
	}
	if actor == string(model.RoleSuperadmin) {
		if role == model.RoleUser || role == model.RoleLeitung {
			return nil
		}
		return fmt.Errorf("invalid role")
	}
	if actor == string(model.RoleLeitung) {
		if role == model.RoleUser {
			return nil
		}
		return fmt.Errorf("leitung may only create role user")
	}
	return fmt.Errorf("forbidden")
}
