package daycalc

import (
	"strconv"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
)

// ShiftBounds holds planned shift clock times for one calendar day (HH:MM in local time).
// Empty Start means: do not clip work intervals to the shift (no schedule or open day).
type ShiftBounds struct {
	Start string
	End   string
}

// ShiftBoundsFromSchedule returns bounds for NetHours/SumWorkedHours, or nil if no planned shift start.
func ShiftBoundsFromSchedule(sch *model.Schedule) *ShiftBounds {
	if sch == nil {
		return nil
	}
	start := strings.TrimSpace(sch.ShiftStart)
	if start == "" {
		return nil
	}
	return &ShiftBounds{
		Start: start,
		End:   strings.TrimSpace(sch.ShiftEnd),
	}
}

func normDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return s
}

// workCalendarDate returns YYYY-MM-DD for tying shift clocks to the business day.
func workCalendarDate(wp model.WorkPeriod, loc *time.Location) string {
	if ds := normDate(wp.WorkDate); len(ds) == 10 {
		return ds
	}
	return wp.PunchIn.In(loc).Format("2006-01-02")
}

// WorkCalendarDate is the exported variant (for callers that group by calendar day).
func WorkCalendarDate(wp model.WorkPeriod, loc *time.Location) string {
	return workCalendarDate(wp, loc)
}

func parseClockOnDay(dayYYYYMMDD, hhmm string, loc *time.Location) (time.Time, bool) {
	dayYYYYMMDD = normDate(dayYYYYMMDD)
	if len(dayYYYYMMDD) != 10 {
		return time.Time{}, false
	}
	base, err := time.ParseInLocation("2006-01-02", dayYYYYMMDD, loc)
	if err != nil {
		return time.Time{}, false
	}
	hhmm = strings.TrimSpace(hhmm)
	parts := strings.Split(hhmm, ":")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return time.Time{}, false
	}
	return time.Date(base.Year(), base.Month(), base.Day(), h, m, 0, 0, loc), true
}

// effectiveWorkDuration returns the duration for a closed non-break period after applying
// effektiver_start = max(punch_in, shift_start) when shift bounds have a start time.
func effectiveWorkDuration(wp model.WorkPeriod, shift *ShiftBounds, loc *time.Location) (time.Duration, bool) {
	if wp.IsBreak || wp.PunchOut == nil {
		return 0, false
	}
	day := workCalendarDate(wp, loc)
	pin := wp.PunchIn
	pout := *wp.PunchOut
	if shift == nil || strings.TrimSpace(shift.Start) == "" {
		if !pin.Before(pout) {
			return 0, false
		}
		return pout.Sub(pin), true
	}
	st, ok := parseClockOnDay(day, shift.Start, loc)
	if !ok {
		if !pin.Before(pout) {
			return 0, false
		}
		return pout.Sub(pin), true
	}
	if pin.Before(st) {
		pin = st
	}
	if !pin.Before(pout) {
		return 0, false
	}
	return pout.Sub(pin), true
}

// EffectiveWorkedHours returns the duration of one closed non-break work period in hours,
// after shift-start clipping (same rule as NetHours / GrossWorkHours).
func EffectiveWorkedHours(wp model.WorkPeriod, shift *ShiftBounds) float64 {
	loc := time.Local
	d, ok := effectiveWorkDuration(wp, shift, loc)
	if !ok {
		return 0
	}
	return d.Hours()
}

// GrossWorkHours sums durations of closed non-break work periods (hours), applying the same
// shift-start rule as NetHours. Break periods are excluded.
func GrossWorkHours(wps []model.WorkPeriod, shift *ShiftBounds) float64 {
	loc := time.Local
	var sum time.Duration
	for _, wp := range wps {
		if wp.IsBreak || wp.PunchOut == nil {
			continue
		}
		d, ok := effectiveWorkDuration(wp, shift, loc)
		if ok {
			sum += d
		}
	}
	return sum.Hours()
}
