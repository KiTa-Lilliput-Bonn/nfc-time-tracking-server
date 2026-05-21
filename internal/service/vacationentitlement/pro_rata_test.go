package vacationentitlement

import (
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
)

func TestAccruedTwelfthsThroughDateFromList_cumulativeMonths(t *testing.T) {
	loc := time.Local
	list := []model.VacationEntitlement{
		{DaysPerYear: 40, ValidFrom: "2025-01-01"},
	}
	through := time.Date(2025, 6, 15, 0, 0, 0, 0, loc)
	got := AccruedTwelfthsThroughDateFromList(list, through, loc)
	// Jan–Jun 2025: 6 × 40/12 = 20
	if got != 20 {
		t.Fatalf("got %v want 20", got)
	}
	yearEnd2025 := EndOfCalendarYear(through, loc)
	gotYear := AccruedTwelfthsThroughDateFromList(list, yearEnd2025, loc)
	if gotYear != 40 {
		t.Fatalf("through year-end 2025: got %v want 40", gotYear)
	}
	through2026 := time.Date(2026, 3, 1, 0, 0, 0, 0, loc)
	got2 := AccruedTwelfthsThroughDateFromList(list, through2026, loc)
	// Jan 2025 – Mär 2026 = 15 Monate × 40/12 = 50
	if got2 != 50 {
		t.Fatalf("got %v want 50", got2)
	}
}

func TestAnnualProRataFromList_twoRulesIn2026(t *testing.T) {
	loc := time.Local
	list := []model.VacationEntitlement{
		{DaysPerYear: 40, ValidFrom: "2025-01-01"},
		{DaysPerYear: 30, ValidFrom: "2026-04-01"},
	}
	got := AnnualProRataFromList(list, 2026, loc)
	// Jan–März: 3/12·40, Apr–Dez: 9/12·30 → 32.5 → Aufrundung auf halbe Tage 32.5
	want := 32.5
	if got != want {
		t.Fatalf("annual entitlement 2026: got %v want %v", got, want)
	}
}

func TestAnnualProRataFromList_midJanuary36DaysRestYear(t *testing.T) {
	loc := time.Local
	list := []model.VacationEntitlement{
		{DaysPerYear: 36, ValidFrom: "2026-01-15"},
	}
	got := AnnualProRataFromList(list, 2026, loc)
	// 11 volle Monate × 3 + Jan-Anteil (31−14)/31 × 3 ≈ 34.645 → Aufrundung halbe Tage → 35
	if got != 35 {
		t.Fatalf("annual entitlement 2026 mid-Jan: got %v want 35", got)
	}
}

func TestAnnualProRataFromList_singleRuleFullYear(t *testing.T) {
	list := []model.VacationEntitlement{
		{DaysPerYear: 25, ValidFrom: "2026-01-01"},
	}
	got := AnnualProRataFromList(list, 2026, time.Local)
	if got < 24.99 || got > 25.01 {
		t.Fatalf("want ~25, got %v", got)
	}
}

func TestAnnualProRataFromList_validFromWithTimestampStillFullYear(t *testing.T) {
	// SQLite DATE kann als "YYYY-MM-DD HH:MM:SS" gelesen werden — muss nicht den 1.1. aus der Summe werfen.
	list := []model.VacationEntitlement{
		{DaysPerYear: 20, ValidFrom: "2026-01-01 00:00:00"},
	}
	got := AnnualProRataFromList(list, 2026, time.Local)
	if got < 19.99 || got > 20.01 {
		t.Fatalf("want ~20, got %v", got)
	}
}

func TestAnnualProRataFromList_leapYear2024(t *testing.T) {
	list := []model.VacationEntitlement{
		{DaysPerYear: 30, ValidFrom: "2024-01-01"},
	}
	got := AnnualProRataFromList(list, 2024, time.Local)
	if got < 29.99 || got > 30.01 {
		t.Fatalf("want ~30 for leap year, got %v", got)
	}
}

func TestAnnualProRataFromList_may10_40Only2026(t *testing.T) {
	loc := time.Local
	list := []model.VacationEntitlement{
		{DaysPerYear: 40, ValidFrom: "2026-05-10"},
	}
	got := AnnualProRataFromList(list, 2026, loc)
	if got != 26 {
		t.Fatalf("want 26 (7 full months + partial May, ceil half), got %v", got)
	}
}

func TestAnnualProRataFromList_jan25_and_may40_2026(t *testing.T) {
	loc := time.Local
	only25 := []model.VacationEntitlement{{DaysPerYear: 25, ValidFrom: "2026-01-01"}}
	if v := AnnualProRataFromList(only25, 2026, loc); v != 25 {
		t.Fatalf("25 ab 1.1.: want 25, got %v", v)
	}
	both := []model.VacationEntitlement{
		{DaysPerYear: 25, ValidFrom: "2026-01-01"},
		{DaysPerYear: 40, ValidFrom: "2026-05-10"},
	}
	got := AnnualProRataFromList(both, 2026, loc)
	if got != 35 {
		t.Fatalf("25 ab 1.1. + 40 ab 10.5.: Erwartung 35, got %v", got)
	}
}
