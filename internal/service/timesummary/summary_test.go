package timesummary

import (
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
)

func TestSumWorkedHours(t *testing.T) {
	t1 := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	periods := []model.WorkPeriod{
		{PunchIn: t1, PunchOut: &t2, IsBreak: false},
	}
	if got := SumWorkedHoursWithMap(periods, nil); got != 4.0 {
		t.Fatalf("got %v want 4", got)
	}
}

func TestTargetHoursMonth(t *testing.T) {
	// March 2026 has 22 weekdays
	if got := TargetHoursMonth(40, 2026, 3, nil); got != 176.0 {
		t.Fatalf("got %v want 176 (40/5*22)", got)
	}
}

func TestTargetHoursMonth_FourDayWeek(t *testing.T) {
	fixed := []int{int(time.Friday)}
	loc := time.Local
	var want float64
	d := model.DailyHours(40, fixed)
	for d0 := time.Date(2026, 3, 1, 0, 0, 0, 0, loc); d0.Month() == time.March; d0 = d0.AddDate(0, 0, 1) {
		if model.IsEmployeeWorkday(d0, fixed) {
			want += d
		}
	}
	if got := TargetHoursMonth(40, 2026, 3, fixed); got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}
