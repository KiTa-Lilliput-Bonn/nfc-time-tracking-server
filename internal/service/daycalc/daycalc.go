package daycalc

import (
	"sort"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/timecalc"
)

// DailyTarget returns expected working hours for a calendar day (non-work weekdays incl. weekend → 0;
// holidays/closures → full daily; sick/vacation/other absences → full or half daily).
func DailyTarget(day time.Time, daily float64, fixedNonWork []int, hol *model.Holiday, abs *model.Absence, clo *model.ClosureDay) float64 {
	if !model.IsEmployeeWorkday(day, fixedNonWork) {
		return 0
	}
	if hol != nil || clo != nil {
		return daily
	}
	if abs != nil {
		switch abs.AbsenceType {
		case model.AbsenceSick, model.AbsenceVacation, model.AbsenceOther, model.AbsenceCompensationDay:
			if abs.HalfDay {
				return daily / 2
			}
			return daily
		}
	}
	return daily
}

// AbsenceCreditHours returns hours credited as "worked" due to an absence (vacation/sick/other).
// This allows days where someone is absent AND still works to count both.
func AbsenceCreditHours(day time.Time, daily float64, fixedNonWork []int, hol *model.Holiday, abs *model.Absence, clo *model.ClosureDay) float64 {
	if !model.IsEmployeeWorkday(day, fixedNonWork) {
		return 0
	}
	if hol != nil || clo != nil {
		return 0
	}
	if abs == nil {
		return 0
	}
	switch abs.AbsenceType {
	case model.AbsenceSick, model.AbsenceVacation, model.AbsenceOther:
		if abs.HalfDay {
			return daily / 2
		}
		return daily
	}
	return 0
}

type workInterval struct {
	start time.Time
	end   time.Time
	dur   time.Duration
}

// NetHours computes day net hours: ceil each closed non-break block to whole minutes, sum day gross,
// credit gaps between consecutive blocks as stamped breaks, apply progressive break deduction once
// for the day total. Legacy is_break rows are ignored. Incomplete periods are skipped.
func NetHours(wps []model.WorkPeriod, breakRules []model.BreakRule, shift *ShiftBounds) float64 {
	loc := time.Local
	var intervals []workInterval
	for _, wp := range wps {
		if wp.PunchOut == nil || wp.IsBreak {
			continue
		}
		start, end, ok := effectiveWorkInterval(wp, shift, loc)
		if !ok {
			continue
		}
		dur := timecalc.RoundUpToMinute(end.Sub(start))
		if dur <= 0 {
			continue
		}
		intervals = append(intervals, workInterval{start: start, end: end, dur: dur})
	}
	if len(intervals) == 0 {
		return 0
	}
	sort.Slice(intervals, func(i, j int) bool {
		return intervals[i].start.Before(intervals[j].start)
	})

	var gross time.Duration
	for _, iv := range intervals {
		gross += iv.dur
	}

	var stamped time.Duration
	for i := 0; i+1 < len(intervals); i++ {
		gap := intervals[i+1].start.Sub(intervals[i].end)
		if gap > 0 {
			stamped += timecalc.RoundUpToMinute(gap)
		}
	}

	ded := timecalc.CalcBreakDeduction(gross, stamped, breakRules)
	if ded > gross {
		ded = gross
	}
	net := gross - ded
	if net < 0 {
		net = 0
	}
	return net.Hours()
}
