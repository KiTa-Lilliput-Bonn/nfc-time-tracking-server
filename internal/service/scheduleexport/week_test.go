package scheduleexport

import "testing"

func TestIterateISOWeeks_range(t *testing.T) {
	weeks, err := iterateISOWeeks(2026, 10, 2026, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(weeks) != 3 {
		t.Fatalf("weeks: %d", len(weeks))
	}
	if weeks[0].Week != 10 || weeks[2].Week != 12 {
		t.Fatalf("%+v", weeks)
	}
}

func TestIterateISOWeeks_endBeforeStart(t *testing.T) {
	_, err := iterateISOWeeks(2026, 20, 2026, 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCompareISOWeek_yearBoundary(t *testing.T) {
	// 2025-12-29 is in ISO week 1 / 2026
	a := isoWeekPair{Year: 2026, Week: 1}
	b := isoWeekPair{Year: 2025, Week: 52}
	if compareISOWeek(a, b) <= 0 {
		t.Fatalf("2026-W1 should be after 2025-W52")
	}
}
