package daycalc

import (
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
)

// effectiveWorkDurationWithMeetings caps shift time at shift end when meetings are present,
// then adds meeting intervals clipped to [punch_in, punch_out] (Sitzungsbeginn-Clip).
func effectiveWorkDurationWithMeetings(wp model.WorkPeriod, shift *ShiftBounds, meetings []*ShiftBounds, loc *time.Location) (time.Duration, bool) {
	if wp.IsBreak || wp.PunchOut == nil {
		return 0, false
	}
	day := workCalendarDate(wp, loc)
	pin := wp.PunchIn
	pout := *wp.PunchOut
	if !pin.Before(pout) {
		return 0, false
	}
	if shift == nil || strings.TrimSpace(shift.Start) == "" || strings.TrimSpace(shift.End) == "" {
		if len(meetings) > 0 {
			return meetingOverlapSum(pin, pout, day, meetings, loc)
		}
		return effectiveWorkDuration(wp, shift, loc)
	}
	ss, okS := parseClockOnDay(day, shift.Start, loc)
	se, okE := parseClockOnDay(day, shift.End, loc)
	if !okS || !okE {
		if len(meetings) > 0 {
			return meetingOverlapSum(pin, pout, day, meetings, loc)
		}
		return effectiveWorkDuration(wp, shift, loc)
	}
	var total time.Duration
	s := pin
	if s.Before(ss) {
		s = ss
	}
	endCap := se
	if pout.Before(endCap) {
		endCap = pout
	}
	if s.Before(endCap) {
		total += endCap.Sub(s)
	}
	for _, m := range meetings {
		if m == nil || strings.TrimSpace(m.Start) == "" || strings.TrimSpace(m.End) == "" {
			continue
		}
		ms, ok1 := parseClockOnDay(day, m.Start, loc)
		me, ok2 := parseClockOnDay(day, m.End, loc)
		if !ok1 || !ok2 {
			continue
		}
		msEff := pin
		if msEff.Before(ms) {
			msEff = ms
		}
		meEff := pout
		if meEff.After(me) {
			meEff = me
		}
		if msEff.Before(meEff) {
			total += meEff.Sub(msEff)
		}
	}
	if total <= 0 {
		return 0, false
	}
	return total, true
}

// EffectiveWorkedHoursWithMeetings applies shift-end capping plus meeting intervals.
func EffectiveWorkedHoursWithMeetings(wp model.WorkPeriod, shift *ShiftBounds, meetings []*ShiftBounds) float64 {
	if len(meetings) == 0 {
		return EffectiveWorkedHours(wp, shift)
	}
	loc := time.Local
	d, ok := effectiveWorkDurationWithMeetings(wp, shift, meetings, loc)
	if !ok {
		return 0
	}
	return d.Hours()
}

// ShiftBoundsSliceFromMeetings maps gespeicherte Teamsitzungen auf Zeitfenster (HH:MM).
func ShiftBoundsSliceFromMeetings(meetings []model.TeamMeeting) []*ShiftBounds {
	out := make([]*ShiftBounds, 0, len(meetings))
	for i := range meetings {
		m := &meetings[i]
		if strings.TrimSpace(m.TimeStart) == "" || strings.TrimSpace(m.TimeEnd) == "" {
			continue
		}
		out = append(out, &ShiftBounds{Start: m.TimeStart, End: m.TimeEnd})
	}
	return out
}

func meetingOverlapSum(pin, pout time.Time, day string, meetings []*ShiftBounds, loc *time.Location) (time.Duration, bool) {
	var total time.Duration
	for _, m := range meetings {
		if m == nil || strings.TrimSpace(m.Start) == "" || strings.TrimSpace(m.End) == "" {
			continue
		}
		ms, ok1 := parseClockOnDay(day, m.Start, loc)
		me, ok2 := parseClockOnDay(day, m.End, loc)
		if !ok1 || !ok2 {
			continue
		}
		msEff := pin
		if msEff.Before(ms) {
			msEff = ms
		}
		meEff := pout
		if meEff.After(me) {
			meEff = me
		}
		if msEff.Before(meEff) {
			total += meEff.Sub(msEff)
		}
	}
	if total <= 0 {
		return 0, false
	}
	return total, true
}
