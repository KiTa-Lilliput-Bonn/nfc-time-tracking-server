package daycalc_test

import (
	"math"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/daycalc"
)

func TestEffectiveWorkedHoursWithMeetings_MeetingClipEarly(t *testing.T) {
	loc := time.Local
	day := "2026-03-16"
	pin := time.Date(2026, 3, 16, 16, 30, 0, 0, loc)
	pout := time.Date(2026, 3, 16, 18, 0, 0, 0, loc)
	wp := model.WorkPeriod{WorkDate: day, PunchIn: pin, PunchOut: &pout, IsBreak: false}
	sh := &daycalc.ShiftBounds{Start: "08:00", End: "17:00"}
	ms := []*daycalc.ShiftBounds{{Start: "17:00", End: "19:00"}}
	got := daycalc.EffectiveWorkedHoursWithMeetings(wp, sh, ms)
	// Schicht 16:30–17:00 (0,5 h) + Sitzung 17:00–18:00 (1 h)
	want := 1.5
	if math.Abs(got-want) > 0.02 {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestEffectiveWorkedHoursWithMeetings_AdjoinNoDouble(t *testing.T) {
	loc := time.Local
	day := "2026-03-16"
	pin := time.Date(2026, 3, 16, 8, 0, 0, 0, loc)
	pout := time.Date(2026, 3, 16, 18, 30, 0, 0, loc)
	wp := model.WorkPeriod{WorkDate: day, PunchIn: pin, PunchOut: &pout, IsBreak: false}
	sh := &daycalc.ShiftBounds{Start: "08:00", End: "17:00"}
	ms := []*daycalc.ShiftBounds{{Start: "17:00", End: "19:00"}}
	got := daycalc.EffectiveWorkedHoursWithMeetings(wp, sh, ms)
	want := 10.5
	if math.Abs(got-want) > 0.02 {
		t.Fatalf("got %v want %v", got, want)
	}
}
