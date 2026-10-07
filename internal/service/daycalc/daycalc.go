package daycalc

import (
	"sort"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/timecalc"
)

// DailyTarget returns expected working hours for a calendar day (non-work weekdays incl. weekend → 0;
// holidays/closures → full daily, credited by AbsenceCreditHours; sick/vacation/other absences → full or half daily).
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

// AbsenceCreditHours returns hours credited as "worked" for a holiday, closure day or absence (vacation/sick/other).
// Holidays and closure days credit the full daily target, so they balance the target like vacation does
// (also when a closure day carries the automatically booked vacation). This allows days where someone is
// absent AND still works to count both.
func AbsenceCreditHours(day time.Time, daily float64, fixedNonWork []int, hol *model.Holiday, abs *model.Absence, clo *model.ClosureDay) float64 {
	if !model.IsEmployeeWorkday(day, fixedNonWork) {
		return 0
	}
	if hol != nil || clo != nil {
		return daily
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
	return NetBreakdownForDay(wps, breakRules, shift).Net.Hours()
}

// NetBreakdown zerlegt die Tagesberechnung von NetHours in nachvollziehbare Teile.
type NetBreakdown struct {
	// Gross ist die Summe der (minutengenau aufgerundeten) Arbeitsblöcke nach Dienstbeginn-Regel.
	Gross time.Duration
	// StampedBreak ist die Summe der Lücken zwischen aufeinanderfolgenden Blöcken (gestempelte Pause).
	StampedBreak time.Duration
	// Deduction ist der automatische Pausenabzug (progressiv, nach Anrechnung der gestempelten Pause).
	Deduction time.Duration
	// Net = Gross − Deduction (nie negativ).
	Net time.Duration
	// OpenPeriod ist true, wenn mindestens ein Block noch keinen Ausstempel-Zeitpunkt hat (nicht mitgezählt).
	OpenPeriod bool
}

// NetBreakdownForDay liefert dieselbe Rechnung wie NetHours, aber mit allen Zwischenwerten.
func NetBreakdownForDay(wps []model.WorkPeriod, breakRules []model.BreakRule, shift *ShiftBounds) NetBreakdown {
	loc := time.Local
	var out NetBreakdown
	var intervals []workInterval
	for _, wp := range wps {
		if wp.IsBreak {
			continue
		}
		if wp.PunchOut == nil {
			out.OpenPeriod = true
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
		return out
	}
	sort.Slice(intervals, func(i, j int) bool {
		return intervals[i].start.Before(intervals[j].start)
	})

	for _, iv := range intervals {
		out.Gross += iv.dur
	}

	for i := 0; i+1 < len(intervals); i++ {
		gap := intervals[i+1].start.Sub(intervals[i].end)
		if gap > 0 {
			out.StampedBreak += timecalc.RoundUpToMinute(gap)
		}
	}

	ded := timecalc.CalcBreakDeduction(out.Gross, out.StampedBreak, breakRules)
	if ded > out.Gross {
		ded = out.Gross
	}
	if ded < 0 {
		ded = 0
	}
	out.Deduction = ded
	out.Net = out.Gross - ded
	return out
}
