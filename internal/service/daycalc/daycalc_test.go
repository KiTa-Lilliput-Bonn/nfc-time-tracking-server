package daycalc_test

import (
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/daycalc"
)

func TestDailyTarget_AbsenceAndHalfDay(t *testing.T) {
	daily := 8.0
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, time.Local)
	abs := &model.Absence{AbsenceType: model.AbsenceVacation, HalfDay: true}
	got := daycalc.DailyTarget(day, daily, nil, nil, abs, nil)
	if got != 4.0 {
		t.Fatalf("expected 4.0, got %v", got)
	}
}

func TestDailyTarget_FixedNonWorkWeekdayZero(t *testing.T) {
	daily := 10.0
	// 2026-03-13 is Friday
	day := time.Date(2026, 3, 13, 0, 0, 0, 0, time.Local)
	fixed := []int{int(time.Friday)}
	got := daycalc.DailyTarget(day, daily, fixed, nil, nil, nil)
	if got != 0 {
		t.Fatalf("expected 0 on fixed free Friday, got %v", got)
	}
	c := daycalc.AbsenceCreditHours(day, daily, fixed, nil, &model.Absence{AbsenceType: model.AbsenceSick}, nil)
	if c != 0 {
		t.Fatalf("expected 0 sick credit on fixed free day, got %v", c)
	}
}

func TestAbsenceCreditHours_FullAndHalfDay(t *testing.T) {
	daily := 8.0
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, time.Local)

	full := &model.Absence{AbsenceType: model.AbsenceVacation, HalfDay: false}
	gotFull := daycalc.AbsenceCreditHours(day, daily, nil, nil, full, nil)
	if gotFull != 8.0 {
		t.Fatalf("expected 8.0, got %v", gotFull)
	}

	half := &model.Absence{AbsenceType: model.AbsenceVacation, HalfDay: true}
	gotHalf := daycalc.AbsenceCreditHours(day, daily, nil, nil, half, nil)
	if gotHalf != 4.0 {
		t.Fatalf("expected 4.0, got %v", gotHalf)
	}
}

func TestNetHours_RoundingAndBreaks(t *testing.T) {
	wps := []model.WorkPeriod{
		{PunchIn: tt("2026-03-10T08:00:00Z"), PunchOut: ptr(tt("2026-03-10T12:00:00Z")), IsBreak: false},
		{PunchIn: tt("2026-03-10T12:00:00Z"), PunchOut: ptr(tt("2026-03-10T12:30:00Z")), IsBreak: true},
	}
	breakRules := []model.BreakRule{{MinWorkHours: 6, BreakMinutes: 30}}
	got := daycalc.NetHours(wps, breakRules, 15, nil)
	// Nur 4h Arbeitsblock; alte Pause-Zeile zählt nicht — unter 6h-Schwelle kein Abzug
	if got != 4.0 {
		t.Fatalf("expected 4h net, got %v", got)
	}
}

func TestNetHours_PerBlockBreak_LongGapDoesNotWaiveFirstBlock(t *testing.T) {
	loc := time.Local
	day := "2026-03-10"
	tIn := time.Date(2026, 3, 10, 8, 0, 0, 0, loc)
	tMid := time.Date(2026, 3, 10, 14, 0, 0, 0, loc)   // 6h erster Block
	tBack := time.Date(2026, 3, 10, 15, 0, 0, 0, loc) // 1h Pause dazwischen (nicht als Zeile)
	tOut := time.Date(2026, 3, 10, 16, 0, 0, 0, loc)  // 1h zweiter Block
	wps := []model.WorkPeriod{
		{WorkDate: day, PunchIn: tIn, PunchOut: &tMid, IsBreak: false},
		{WorkDate: day, PunchIn: tBack, PunchOut: &tOut, IsBreak: false},
	}
	breakRules := []model.BreakRule{{MinWorkHours: 6, BreakMinutes: 30}}
	got := daycalc.NetHours(wps, breakRules, 15, nil)
	if got != 6.5 {
		t.Fatalf("expected 6.5h net (6h−30m + 1h), got %v", got)
	}
}

func TestNetHours_LegacyBreakRowBetweenBlocksSameAsImplicitGap(t *testing.T) {
	loc := time.Local
	day := "2026-03-10"
	tIn := time.Date(2026, 3, 10, 8, 0, 0, 0, loc)
	tMid := time.Date(2026, 3, 10, 14, 0, 0, 0, loc)
	tBreakEnd := time.Date(2026, 3, 10, 15, 0, 0, 0, loc)
	tOut := time.Date(2026, 3, 10, 16, 0, 0, 0, loc)
	wps := []model.WorkPeriod{
		{WorkDate: day, PunchIn: tIn, PunchOut: &tMid, IsBreak: false},
		{WorkDate: day, PunchIn: tMid, PunchOut: &tBreakEnd, IsBreak: true},
		{WorkDate: day, PunchIn: tBreakEnd, PunchOut: &tOut, IsBreak: false},
	}
	breakRules := []model.BreakRule{{MinWorkHours: 6, BreakMinutes: 30}}
	got := daycalc.NetHours(wps, breakRules, 15, nil)
	if got != 6.5 {
		t.Fatalf("expected 6.5h net ignoring legacy pause row, got %v", got)
	}
}

func tt(s string) time.Time {
	tm, _ := time.Parse(time.RFC3339, s)
	return tm
}

func ptr(tt time.Time) *time.Time {
	return &tt
}

func TestNetHours_ShiftStartDoesNotClipManual(t *testing.T) {
	loc := time.Local
	day := "2026-03-10"
	pin := time.Date(2026, 3, 10, 7, 30, 0, 0, loc)
	pout := time.Date(2026, 3, 10, 12, 0, 0, 0, loc)
	wps := []model.WorkPeriod{
		{WorkDate: day, PunchIn: pin, PunchOut: &pout, IsBreak: false, Source: "manual"},
	}
	shift := &daycalc.ShiftBounds{Start: "08:00"}
	got := daycalc.NetHours(wps, nil, 15, shift)
	if got != 4.5 {
		t.Fatalf("manual before shift start: expected 4.5h, got %v", got)
	}
}

func TestNetHours_ShiftStartClipsEarlyPunch(t *testing.T) {
	loc := time.Local
	day := "2026-03-10"
	pin := time.Date(2026, 3, 10, 7, 30, 0, 0, loc)
	pout := time.Date(2026, 3, 10, 12, 0, 0, 0, loc)
	wps := []model.WorkPeriod{
		{WorkDate: day, PunchIn: pin, PunchOut: &pout, IsBreak: false},
	}
	shift := &daycalc.ShiftBounds{Start: "08:00"}
	got := daycalc.NetHours(wps, nil, 15, shift)
	if got != 4.0 {
		t.Fatalf("with shift 08:00, expected 4h net (08:00–12:00), got %v", got)
	}
	raw := daycalc.NetHours(wps, nil, 15, nil)
	if raw != 4.5 {
		t.Fatalf("without shift expected 4.5h, got %v", raw)
	}
}
