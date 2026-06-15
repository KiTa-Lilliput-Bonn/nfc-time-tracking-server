package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	apimw "nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/service/schedulegaps"
	"nfc-time-tracking-server/internal/service/shiftalerts"
	"nfc-time-tracking-server/internal/service/teamoverview"
	"nfc-time-tracking-server/internal/store"
)

// DashboardHandler serves Leitung dashboard aggregates (team overview).
type DashboardHandler struct {
	Users       store.UserStore
	WorkPeriods store.WorkPeriodStore
	Corrections            store.CorrectionStore
	Absences               store.AbsenceStore
	CompensationDayClaims  store.CompensationDayClaimStore
	Holidays               store.HolidayStore
	Closures    store.ClosureDayStore
	WeeklyHours store.WeeklyHoursStore
	FixedNonWorkWeekdays store.FixedNonWorkWeekdaysStore
	ScheduleBound        store.ScheduleBoundStore
	Settings             store.SettingsStore
	VacationEnt store.VacationEntitlementStore
	Schedules   store.ScheduleStore
	ShiftAlertDismissals store.ShiftAlertDismissalStore
	Audit       *audit.Logger
}

func (h *DashboardHandler) teamDeps() teamoverview.Deps {
	return teamoverview.Deps{
		Users:                 h.Users,
		WorkPeriods:           h.WorkPeriods,
		Corrections:           h.Corrections,
		Absences:              h.Absences,
		CompensationDayClaims: h.CompensationDayClaims,
		Holidays:              h.Holidays,
		Closures:              h.Closures,
		WeeklyHours:           h.WeeklyHours,
		FixedNonWorkWeekdays:  h.FixedNonWorkWeekdays,
		ScheduleBound:         h.ScheduleBound,
		Settings:              h.Settings,
		VacationEnt:           h.VacationEnt,
		Schedules:             h.Schedules,
	}
}

// TeamOverview returns aggregated hours and vacation rows for active employees (GET /dashboard/team-overview).
// Stundensaldo je Mitarbeitenden ab frühestem Stundensoll (weekly_hours.valid_from) bzw. 1.1. des Jahres von „gestern“
// ohne Stundensoll-Datensätze, bis einschließlich letztem vollen Tag (gestern). Query as_of wird ignoriert (Abwärtskompatibilität).
func (h *DashboardHandler) TeamOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	loc := time.Local
	q := r.URL.Query()

	vyParam, err := parseVacationYearParam(q)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid vacation_year")
		return
	}
	now := time.Now()
	resolvedVY := vyParam
	if resolvedVY == 0 {
		resolvedVY = now.In(loc).Year()
	}

	rows, periodStartISO, err := teamoverview.Build(ctx, h.teamDeps(), vyParam, now)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"as_of":         periodStartISO,
		"vacation_year": resolvedVY,
		"rows":          rows,
	})
}

// ScheduleGaps returns days with planned shifts through yesterday without work or blocking absence.
func (h *DashboardHandler) ScheduleGaps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	res, err := schedulegaps.Build(ctx, schedulegaps.Deps{
		Users:       h.Users,
		Schedules:   h.Schedules,
		WorkPeriods: h.WorkPeriods,
		Absences:    h.Absences,
		WeeklyHours: h.WeeklyHours,
	}, time.Now())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, res)
}

func (h *DashboardHandler) shiftAlertDeps() shiftalerts.Deps {
	return shiftalerts.Deps{
		Users:       h.Users,
		WorkPeriods: h.WorkPeriods,
		Corrections: h.Corrections,
		WeeklyHours: h.WeeklyHours,
		Dismissals:  h.ShiftAlertDismissals,
	}
}

// ShiftAlerts returns days with unusually long or late recorded work times through yesterday.
func (h *DashboardHandler) ShiftAlerts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg, err := shiftalerts.LoadConfig(ctx, h.Settings)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "settings failed")
		return
	}
	res, err := shiftalerts.Build(ctx, h.shiftAlertDeps(), cfg, time.Now())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, res)
}

type shiftAlertDismissBody struct {
	UserID   int    `json:"user_id"`
	WorkDate string `json:"work_date"`
}

// DismissShiftAlert marks a user/day shift alert as reviewed (POST /dashboard/shift-alerts/dismiss).
func (h *DashboardHandler) DismissShiftAlert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body shiftAlertDismissBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.WorkDate = strings.TrimSpace(body.WorkDate)
	if body.UserID <= 0 || body.WorkDate == "" {
		response.Error(w, http.StatusBadRequest, "user_id and work_date required")
		return
	}
	actorID, ok := ctx.Value(apimw.CtxUserID).(int)
	if !ok || actorID <= 0 {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.ShiftAlertDismissals == nil {
		response.Error(w, http.StatusInternalServerError, "not configured")
		return
	}
	if err := h.ShiftAlertDismissals.Create(ctx, body.UserID, body.WorkDate, actorID); err != nil {
		response.Error(w, http.StatusInternalServerError, "save failed")
		return
	}
	logAudit(h.Audit, ctx, audit.Entry{
		Action: audit.ActionCreate, EntityType: audit.EntityShiftAlertDismissal,
		EntityID: auditID(body.UserID) + ":" + body.WorkDate,
		TargetUserID: auditTarget(body.UserID),
		Summary: audit.JSONSummary(map[string]any{
			"user_id": body.UserID, "work_date": body.WorkDate,
		}),
	})
	response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func parseVacationYearParam(q interface{ Get(string) string }) (int, error) {
	s := strings.TrimSpace(q.Get("vacation_year"))
	if s == "" {
		return 0, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if v == 0 {
		return 0, nil
	}
	if v < 1 || v > 9999 {
		return 0, fmt.Errorf("vacation_year out of range")
	}
	return v, nil
}
