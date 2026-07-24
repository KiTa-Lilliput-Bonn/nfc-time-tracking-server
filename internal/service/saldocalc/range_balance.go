package saldocalc

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/daycalc"
	"nfc-time-tracking-server/internal/service/timesummary"
	"nfc-time-tracking-server/internal/store"
)

// Deps bundles stores for day-level net balance aggregation (export/dashboard algorithm).
type Deps struct {
	WorkPeriods          store.WorkPeriodStore
	Corrections          store.CorrectionStore
	Absences             store.AbsenceStore
	Holidays             store.HolidayStore
	Closures             store.ClosureDayStore
	WeeklyHours          store.WeeklyHoursStore
	FixedNonWorkWeekdays store.FixedNonWorkWeekdaysStore
	ScheduleBound        store.ScheduleBoundStore
	Schedules            store.ScheduleStore
	Settings             store.SettingsStore
}

// RangeTotals is the sum of daily net+credit, target, and balance over an inclusive date range.
type RangeTotals struct {
	WorkedHours  float64
	TargetHours  float64
	BalanceHours float64
}

// SumRange aggregates (NetHours + AbsenceCredit − DailyTarget) per calendar day from..to (YYYY-MM-DD, inclusive).
func SumRange(ctx context.Context, d Deps, userID int, from, to string) (RangeTotals, error) {
	loc := time.Local
	start, err := time.ParseInLocation("2006-01-02", from, loc)
	if err != nil {
		return RangeTotals{}, fmt.Errorf("range from: %w", err)
	}
	end, err := time.ParseInLocation("2006-01-02", to, loc)
	if err != nil {
		return RangeTotals{}, fmt.Errorf("range to: %w", err)
	}
	if end.Before(start) {
		return RangeTotals{WorkedHours: 0, TargetHours: 0, BalanceHours: 0}, nil
	}
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, loc)

	breakRules := loadBreakRules(ctx, d.Settings)

	var fnwRows []model.FixedNonWorkWeekdays
	if d.FixedNonWorkWeekdays != nil {
		fnwRows, err = d.FixedNonWorkWeekdays.ListByUser(ctx, userID)
		if err != nil {
			return RangeTotals{}, err
		}
	}
	var scheduleBoundRows []model.ScheduleBoundSetting
	if d.ScheduleBound != nil {
		scheduleBoundRows, err = d.ScheduleBound.ListByUser(ctx, userID)
		if err != nil {
			return RangeTotals{}, err
		}
	}
	var whRows []model.WeeklyHours
	if d.WeeklyHours != nil {
		whRows, err = d.WeeklyHours.ListByUser(ctx, userID)
		if err != nil {
			return RangeTotals{}, err
		}
	}

	var wps []model.WorkPeriod
	if d.WorkPeriods != nil {
		wps, err = d.WorkPeriods.ListByUserDateRange(ctx, userID, from, to)
		if err != nil {
			return RangeTotals{}, err
		}
	}
	if d.Corrections != nil {
		corrs, err := d.Corrections.ListByUser(ctx, userID, from, to)
		if err != nil {
			return RangeTotals{}, err
		}
		wps = timesummary.ApplyLatestCorrections(wps, corrs)
	}
	byDate := groupWorkPeriodsByDate(wps)

	var absList []model.Absence
	if d.Absences != nil {
		absList, err = d.Absences.ListByUserDateRange(ctx, userID, from, to)
		if err != nil {
			return RangeTotals{}, err
		}
	}
	absByDate := indexFirstAbsenceByDate(absList)

	holidayByDate, err := loadHolidayMap(ctx, d.Holidays, from, to)
	if err != nil {
		return RangeTotals{}, err
	}
	closureByDate, err := loadClosureMap(ctx, d.Closures, from, to)
	if err != nil {
		return RangeTotals{}, err
	}

	var schByDate map[string]*model.Schedule
	if d.Schedules != nil {
		schRows, err := d.Schedules.ListByUserDateRange(ctx, userID, from, to)
		if err != nil {
			return RangeTotals{}, err
		}
		schByDate = indexSchedulesByDate(schRows)
	}

	var worked, target float64
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		ds := day.Format("2006-01-02")
		dayWps := byDate[ds]
		var shiftBounds *daycalc.ShiftBounds
		if schByDate != nil {
			if sch := schByDate[ds]; sch != nil {
				bound := model.ScheduleBoundForDate(scheduleBoundRows, ds)
				shiftBounds = daycalc.ShiftBoundsIfBound(sch, bound)
			}
		}
		net := daycalc.NetHours(dayWps, breakRules, shiftBounds)

		fixed := model.FixedNonWorkWeekdaysForDate(fnwRows, ds)
		var daily float64
		if wh := weeklyHoursForDate(whRows, ds); wh != nil {
			daily = model.DailyHours(wh.HoursPerWeek, fixed)
		}
		hol := holidayByDate[ds]
		abs := absByDate[ds]
		clo := closureByDate[ds]
		dayTarget := daycalc.DailyTarget(day, daily, fixed, hol, abs, clo)
		credit := daycalc.AbsenceCreditHours(day, daily, fixed, hol, abs, clo)

		worked += net + credit
		target += dayTarget
	}

	return RangeTotals{
		WorkedHours:  round2(worked),
		TargetHours:  round2(target),
		BalanceHours: round2(worked - target),
	}, nil
}

func loadBreakRules(ctx context.Context, s store.SettingsStore) (breakRules []model.BreakRule) {
	if s == nil {
		return breakRules
	}
	if v, err := s.Get(ctx, "break_rules"); err == nil {
		_ = json.Unmarshal([]byte(v), &breakRules)
	}
	return breakRules
}

func groupWorkPeriodsByDate(wps []model.WorkPeriod) map[string][]model.WorkPeriod {
	m := make(map[string][]model.WorkPeriod)
	for _, wp := range wps {
		ds := normDate(wp.WorkDate)
		m[ds] = append(m[ds], wp)
	}
	return m
}

func normDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return s
}

func loadHolidayMap(ctx context.Context, hs store.HolidayStore, fromStr, toStr string) (map[string]*model.Holiday, error) {
	m := make(map[string]*model.Holiday)
	if hs == nil {
		return m, nil
	}
	fromDay, err := time.ParseInLocation("2006-01-02", fromStr, time.Local)
	if err != nil {
		return nil, fmt.Errorf("holiday map from: %w", err)
	}
	toDay, err := time.ParseInLocation("2006-01-02", toStr, time.Local)
	if err != nil {
		return nil, fmt.Errorf("holiday map to: %w", err)
	}
	for y := fromDay.Year(); y <= toDay.Year(); y++ {
		list, err := hs.ListByYear(ctx, y)
		if err != nil {
			return nil, err
		}
		for i := range list {
			ds := normDate(list[i].HolidayDate)
			if ds < fromStr || ds > toStr {
				continue
			}
			h := &list[i]
			m[ds] = h
		}
	}
	return m, nil
}

func loadClosureMap(ctx context.Context, cs store.ClosureDayStore, fromStr, toStr string) (map[string]*model.ClosureDay, error) {
	m := make(map[string]*model.ClosureDay)
	if cs == nil {
		return m, nil
	}
	all, err := cs.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range all {
		ds := normDate(all[i].ClosureDate)
		if ds < fromStr || ds > toStr {
			continue
		}
		c := &all[i]
		m[ds] = c
	}
	return m, nil
}

func weeklyHoursForDate(rows []model.WeeklyHours, date string) *model.WeeklyHours {
	date = normDate(date)
	var best *model.WeeklyHours
	for i := range rows {
		wh := &rows[i]
		vf := normDate(wh.ValidFrom)
		if vf > date {
			continue
		}
		if best == nil || vf > normDate(best.ValidFrom) {
			best = wh
		}
	}
	return best
}

func indexFirstAbsenceByDate(abs []model.Absence) map[string]*model.Absence {
	m := make(map[string]*model.Absence)
	for i := range abs {
		ds := normDate(abs[i].AbsenceDate)
		if _, ok := m[ds]; ok {
			continue
		}
		a := &abs[i]
		m[ds] = a
	}
	return m
}

func indexSchedulesByDate(rows []model.Schedule) map[string]*model.Schedule {
	m := make(map[string]*model.Schedule)
	for i := range rows {
		s := &rows[i]
		m[normDate(s.ScheduleDate)] = s
	}
	return m
}

func round2(x float64) float64 {
	return math.Round(x*100) / 100
}
