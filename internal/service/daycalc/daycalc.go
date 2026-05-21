package daycalc

import (
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

// NetHours mirrors export row logic: for each closed non-break work period, gross duration (after shift-start
// clipping) minus timecalc.CalcBreakDeduction for that block alone (stamped breaks are not used). Legacy rows
// with is_break are ignored. Sum is clamped at zero, then timecalc.RoundDown to roundMin-minute grid.
// Incomplete periods (no punch-out) are skipped.
func NetHours(wps []model.WorkPeriod, breakRules []model.BreakRule, roundMin int, shift *ShiftBounds) float64 {
	loc := time.Local
	var gross time.Duration
	var ded time.Duration
	for _, wp := range wps {
		if wp.PunchOut == nil || wp.IsBreak {
			continue
		}
		dur, ok := effectiveWorkDuration(wp, shift, loc)
		if !ok {
			continue
		}
		blockDed := timecalc.CalcBreakDeduction(dur, 0, breakRules)
		if blockDed > dur {
			blockDed = dur
		}
		gross += dur
		ded += blockDed
	}
	net := gross - ded
	if net < 0 {
		net = 0
	}
	net = timecalc.RoundDown(net, roundMin)
	return net.Hours()
}
