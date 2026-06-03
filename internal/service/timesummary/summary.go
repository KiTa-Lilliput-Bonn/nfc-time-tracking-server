package timesummary

import (
	"context"
	"math"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/daycalc"
	"nfc-time-tracking-server/internal/store"
)

// SumWorkedHoursWithMap sums closed non-break work periods in hours (2 decimal places).
// shiftByDate maps YYYY-MM-DD to shift bounds; nil map or missing key means no shift clipping for that day.
func SumWorkedHoursWithMap(periods []model.WorkPeriod, shiftByDate map[string]*daycalc.ShiftBounds) float64 {
	loc := time.Local
	var sum float64
	for _, wp := range periods {
		if wp.IsBreak || wp.PunchOut == nil {
			continue
		}
		var sh *daycalc.ShiftBounds
		if shiftByDate != nil {
			ds := daycalc.WorkCalendarDate(wp, loc)
			sh = shiftByDate[ds]
		}
		sum += daycalc.EffectiveWorkedHours(wp, sh)
	}
	return math.Round(sum*100) / 100
}

// BuildShiftBoundsMap loads schedules for every calendar day present in periods (unique dates).
// Shift clipping applies only when schedule_bound is true for that day (default true).
func BuildShiftBoundsMap(ctx context.Context, userID int, periods []model.WorkPeriod, schedules store.ScheduleStore, scheduleBound store.ScheduleBoundStore) (map[string]*daycalc.ShiftBounds, error) {
	if schedules == nil {
		return nil, nil
	}
	var boundRows []model.ScheduleBoundSetting
	if scheduleBound != nil {
		var err error
		boundRows, err = scheduleBound.ListByUser(ctx, userID)
		if err != nil {
			return nil, err
		}
	}
	loc := time.Local
	seen := make(map[string]struct{})
	for _, wp := range periods {
		ds := daycalc.WorkCalendarDate(wp, loc)
		if len(ds) == 10 {
			seen[ds] = struct{}{}
		}
	}
	out := make(map[string]*daycalc.ShiftBounds, len(seen))
	for ds := range seen {
		sch, err := schedules.GetForUserDate(ctx, userID, ds)
		if err != nil {
			return nil, err
		}
		bound := model.ScheduleBoundForDate(boundRows, ds)
		out[ds] = daycalc.ShiftBoundsIfBound(sch, bound)
	}
	return out, nil
}

// BuildMeetingBoundsByDate lists Team-Meeting-Zeitfenster (als ShiftBounds) je Kalendertag für den Nutzer.
func BuildMeetingBoundsByDate(ctx context.Context, userID int, periods []model.WorkPeriod, tms store.TeamMeetingStore) (map[string][]*daycalc.ShiftBounds, error) {
	out := map[string][]*daycalc.ShiftBounds{}
	if tms == nil || len(periods) == 0 {
		return out, nil
	}
	loc := time.Local
	minD, maxD := "", ""
	for _, wp := range periods {
		if wp.IsBreak {
			continue
		}
		ds := daycalc.WorkCalendarDate(wp, loc)
		if len(ds) != 10 {
			continue
		}
		if minD == "" || ds < minD {
			minD = ds
		}
		if maxD == "" || ds > maxD {
			maxD = ds
		}
	}
	if minD == "" {
		return out, nil
	}
	list, err := tms.ListForUserInDateRange(ctx, userID, minD, maxD)
	if err != nil {
		return nil, err
	}
	for i := range list {
		tm := &list[i]
		ds := strings.TrimSpace(tm.MeetingDate)
		if len(ds) >= 10 {
			ds = ds[:10]
		}
		if len(ds) != 10 {
			continue
		}
		if strings.TrimSpace(tm.TimeStart) == "" || strings.TrimSpace(tm.TimeEnd) == "" {
			continue
		}
		out[ds] = append(out[ds], &daycalc.ShiftBounds{Start: tm.TimeStart, End: tm.TimeEnd})
	}
	return out, nil
}

// SumWorkedHoursFromStore loads shift bounds per day and sums worked hours (same clipping as NetHours).
// teamMeetings optional: bei gesetzter Store werden Montags-Teamsitzungen für die Aufteilung berücksichtigt.
func SumWorkedHoursFromStore(ctx context.Context, userID int, periods []model.WorkPeriod, schedules store.ScheduleStore, teamMeetings store.TeamMeetingStore, scheduleBound store.ScheduleBoundStore) (float64, error) {
	if schedules == nil {
		return SumWorkedHoursWithMap(periods, nil), nil
	}
	m, err := BuildShiftBoundsMap(ctx, userID, periods, schedules, scheduleBound)
	if err != nil {
		return 0, err
	}
	meetByDate, err := BuildMeetingBoundsByDate(ctx, userID, periods, teamMeetings)
	if err != nil {
		return 0, err
	}
	loc := time.Local
	var sum float64
	for _, wp := range periods {
		if wp.IsBreak || wp.PunchOut == nil {
			continue
		}
		ds := daycalc.WorkCalendarDate(wp, loc)
		sh := m[ds]
		ms := meetByDate[ds]
		if len(ms) > 0 {
			sum += daycalc.EffectiveWorkedHoursWithMeetings(wp, sh, ms)
		} else {
			sum += daycalc.EffectiveWorkedHours(wp, sh)
		}
	}
	return math.Round(sum*100) / 100, nil
}

// WeekdaysInMonth counts Monday–Friday dates in the given calendar month (local timezone).
func WeekdaysInMonth(year, month int) int {
	loc := time.Local
	t := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	n := 0
	for t.Month() == time.Month(month) {
		wd := t.Weekday()
		if wd != time.Saturday && wd != time.Sunday {
			n++
		}
		t = t.AddDate(0, 0, 1)
	}
	return n
}

// TargetHoursMonth is the sum of daily targets for each calendar day in the month (local timezone):
// each employee workday (Mon–Fri excluding fixed non-work weekdays) contributes DailyHours(hoursPerWeek, fixed).
func TargetHoursMonth(hoursPerWeek float64, year, month int, fnwRows []model.FixedNonWorkWeekdays) float64 {
	if hoursPerWeek <= 0 {
		return 0
	}
	loc := time.Local
	t := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	var sum float64
	for t.Month() == time.Month(month) {
		ds := t.Format("2006-01-02")
		fixed := model.FixedNonWorkWeekdaysForDate(fnwRows, ds)
		if model.IsEmployeeWorkday(t, fixed) {
			d := model.DailyHours(hoursPerWeek, fixed)
			if d > 0 {
				sum += d
			}
		}
		t = t.AddDate(0, 0, 1)
	}
	return math.Round(sum*100) / 100
}

type weeklyHoursForDateStore interface {
	GetForDate(ctx context.Context, userID int, date string) (*model.WeeklyHours, error)
}

// TargetHoursMonthByWeekHistory resolves weekly hours per calendar date and sums daily targets
// for employee workdays in the month (including fixed non-work weekday history).
func TargetHoursMonthByWeekHistory(
	ctx context.Context,
	userID int,
	year, month int,
	fnwRows []model.FixedNonWorkWeekdays,
	whs weeklyHoursForDateStore,
) (float64, error) {
	loc := time.Local
	t := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	var sum float64
	for t.Month() == time.Month(month) {
		ds := t.Format("2006-01-02")
		wh, err := whs.GetForDate(ctx, userID, ds)
		if err != nil {
			return 0, err
		}
		if wh != nil && wh.HoursPerWeek > 0 {
			fixed := model.FixedNonWorkWeekdaysForDate(fnwRows, ds)
			if model.IsEmployeeWorkday(t, fixed) {
				d := model.DailyHours(wh.HoursPerWeek, fixed)
				if d > 0 {
					sum += d
				}
			}
		}
		t = t.AddDate(0, 0, 1)
	}
	return math.Round(sum*100) / 100, nil
}

// MonthDateRange returns first and last date (YYYY-MM-DD) of the month in UTC labels.
func MonthDateRange(year, month int) (from, to string) {
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1)
	return first.Format("2006-01-02"), last.Format("2006-01-02")
}

// SumVacationDays counts full and half vacation absences in range (from/to inclusive, YYYY-MM-DD).
func SumVacationDays(absences []model.Absence) float64 {
	var d float64
	for _, a := range absences {
		if a.AbsenceType != model.AbsenceVacation {
			continue
		}
		if a.HalfDay {
			d += 0.5
		} else {
			d += 1.0
		}
	}
	return d
}
