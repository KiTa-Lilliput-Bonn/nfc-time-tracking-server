package model

import (
	"testing"
	"time"
)

func TestFixedNonWorkWeekdaysForDate_TimestampValidFromOnBoundaryDay(t *testing.T) {
	rows := []FixedNonWorkWeekdays{{
		ID: 1, Weekdays: []int{int(time.Friday)}, ValidFrom: "2026-05-27T00:00:00Z",
	}}
	got := FixedNonWorkWeekdaysForDate(rows, "2026-05-27")
	if len(got) != 1 || got[0] != int(time.Friday) {
		t.Fatalf("got %v want Friday on boundary day", got)
	}
}

func TestFixedNonWorkWeekdaysForDate_TieBreakPrefersHigherID(t *testing.T) {
	rows := []FixedNonWorkWeekdays{
		{ID: 1, Weekdays: nil, ValidFrom: "2026-05-27"},
		{ID: 2, Weekdays: []int{int(time.Friday)}, ValidFrom: "2026-05-27T00:00:00Z"},
	}
	got := FixedNonWorkWeekdaysForDate(rows, "2026-05-27")
	if len(got) != 1 || got[0] != int(time.Friday) {
		t.Fatalf("got %v want Friday from higher id row", got)
	}
}

func TestDailyHours_FourDayWeek(t *testing.T) {
	fixed := []int{int(time.Friday)}
	if got := DailyHours(24, fixed); got != 6 {
		t.Fatalf("got %v want 6", got)
	}
}
