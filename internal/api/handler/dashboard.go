package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/api/response"
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
	Settings    store.SettingsStore
	VacationEnt store.VacationEntitlementStore
	Schedules   store.ScheduleStore
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
