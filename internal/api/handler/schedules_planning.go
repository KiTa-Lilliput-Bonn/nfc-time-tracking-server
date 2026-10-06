package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/model"
	sqlitesched "nfc-time-tracking-server/internal/store/sqlite"
)

// planningUser enthält, was die Handy-Planung je Person für die Woche braucht (Soll-Berechnung im Browser).
type planningUser struct {
	UserID               int     `json:"user_id"`
	HoursPerWeek         float64 `json:"hours_per_week"`
	FixedNonWorkWeekdays []int   `json:"fixed_non_work_weekdays"`
}

// PlanningWeek liefert Zusatzdaten für die Dienstplanung einer ISO-Woche: Wochenstunden und fix freie
// Wochentage (Stand Montag), alle Abwesenheiten (inkl. krank/sonstige), Schließtage und Pausenregeln.
// Der Desktop-Editor nutzt weiterhin nur GET /schedules.
func (h *ScheduleHandler) PlanningWeek(w http.ResponseWriter, r *http.Request) {
	wk, err1 := strconv.Atoi(r.URL.Query().Get("week"))
	yr, err2 := strconv.Atoi(r.URL.Query().Get("year"))
	if err1 != nil || err2 != nil || wk < 1 || wk > 53 {
		response.Error(w, http.StatusBadRequest, "week and year required")
		return
	}
	from, to, err := sqlitesched.ISOWeekMondayFriday(yr, wk)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()

	users, err := h.Users.List(ctx, true)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	outUsers := make([]planningUser, 0, len(users))
	for _, u := range users {
		pu := planningUser{UserID: u.ID, FixedNonWorkWeekdays: []int{}}
		if h.WeeklyHours != nil {
			wh, err := h.WeeklyHours.GetForDate(ctx, u.ID, from)
			if err != nil {
				response.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			if wh != nil {
				pu.HoursPerWeek = wh.HoursPerWeek
			}
		}
		if h.FixedNonWorkWeekdays != nil {
			rows, err := h.FixedNonWorkWeekdays.ListByUser(ctx, u.ID)
			if err != nil {
				response.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			if fixed := model.FixedNonWorkWeekdaysForDate(rows, from); fixed != nil {
				pu.FixedNonWorkWeekdays = fixed
			}
		}
		outUsers = append(outUsers, pu)
	}

	absences, err := h.Absences.ListByDateRangeTypes(ctx, from, to, []model.AbsenceType{
		model.AbsenceVacation,
		model.AbsenceCompensationDay,
		model.AbsenceSick,
		model.AbsenceOther,
	})
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if absences == nil {
		absences = []model.Absence{}
	}

	closures := []model.ClosureDay{}
	if h.Closures != nil {
		all, err := h.Closures.List(ctx)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, c := range all {
			d := model.NormCalendarDate(c.ClosureDate)
			if d >= from && d <= to {
				c.ClosureDate = d
				closures = append(closures, c)
			}
		}
	}

	breakRules := []model.BreakRule{}
	if h.Settings != nil {
		if v, err := h.Settings.Get(ctx, "break_rules"); err == nil && v != "" {
			var parsed []model.BreakRule
			if json.Unmarshal([]byte(v), &parsed) == nil && parsed != nil {
				breakRules = parsed
			}
		}
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"from": from, "to": to,
		"users": outUsers, "absences": absences,
		"closure_days": closures, "break_rules": breakRules,
	})
}
