package timesummary

import (
	"context"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
)

type stubWeeklyHoursForDateStore struct {
	byDate map[string]float64
}

func (s stubWeeklyHoursForDateStore) GetForDate(ctx context.Context, userID int, date string) (*model.WeeklyHours, error) {
	h, ok := s.byDate[date]
	if !ok {
		return nil, nil
	}
	return &model.WeeklyHours{UserID: userID, HoursPerWeek: h, ValidFrom: date}, nil
}

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
	fnwRows := []model.FixedNonWorkWeekdays{{Weekdays: fixed, ValidFrom: "2000-01-01"}}
	loc := time.Local
	var want float64
	d := model.DailyHours(40, fixed)
	for d0 := time.Date(2026, 3, 1, 0, 0, 0, 0, loc); d0.Month() == time.March; d0 = d0.AddDate(0, 0, 1) {
		if model.IsEmployeeWorkday(d0, fixed) {
			want += d
		}
	}
	if got := TargetHoursMonth(40, 2026, 3, fnwRows); got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestTargetHoursMonth_ChangeMidMonth(t *testing.T) {
	fnwRows := []model.FixedNonWorkWeekdays{
		{Weekdays: nil, ValidFrom: "2000-01-01"},
		{Weekdays: []int{int(time.Friday)}, ValidFrom: "2026-03-16"},
	}
	loc := time.Local
	var want float64
	for d0 := time.Date(2026, 3, 1, 0, 0, 0, 0, loc); d0.Month() == time.March; d0 = d0.AddDate(0, 0, 1) {
		ds := d0.Format("2006-01-02")
		fixed := model.FixedNonWorkWeekdaysForDate(fnwRows, ds)
		if model.IsEmployeeWorkday(d0, fixed) {
			want += model.DailyHours(40, fixed)
		}
	}
	if got := TargetHoursMonth(40, 2026, 3, fnwRows); got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestTargetHoursMonthByWeekHistory_ChangeMidMonth(t *testing.T) {
	ctx := context.Background()
	loc := time.Local
	wh := stubWeeklyHoursForDateStore{byDate: map[string]float64{}}
	for d0 := time.Date(2026, 5, 25, 0, 0, 0, 0, loc); d0.Month() == time.May; d0 = d0.AddDate(0, 0, 1) {
		wh.byDate[d0.Format("2006-01-02")] = 40
	}
	got, err := TargetHoursMonthByWeekHistory(ctx, 1, 2026, 5, nil, wh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var want float64
	for d0 := time.Date(2026, 5, 25, 0, 0, 0, 0, loc); d0.Month() == time.May; d0 = d0.AddDate(0, 0, 1) {
		if d0.Weekday() != time.Saturday && d0.Weekday() != time.Sunday {
			want += 8
		}
	}
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestTargetHoursMonthByWeekHistory_FourDayWeekTimestampValidFrom(t *testing.T) {
	ctx := context.Background()
	loc := time.Local
	wh := stubWeeklyHoursForDateStore{byDate: map[string]float64{}}
	for d0 := time.Date(2026, 5, 27, 0, 0, 0, 0, loc); d0.Month() == time.May; d0 = d0.AddDate(0, 0, 1) {
		wh.byDate[d0.Format("2006-01-02")] = 24
	}
	fnwRows := []model.FixedNonWorkWeekdays{{
		Weekdays: []int{int(time.Friday)}, ValidFrom: "2026-05-27T00:00:00Z",
	}}
	got, err := TargetHoursMonthByWeekHistory(ctx, 1, 2026, 5, fnwRows, wh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fixed := []int{int(time.Friday)}
	var want float64
	for d0 := time.Date(2026, 5, 27, 0, 0, 0, 0, loc); d0.Month() == time.May; d0 = d0.AddDate(0, 0, 1) {
		ds := d0.Format("2006-01-02")
		if _, ok := wh.byDate[ds]; !ok {
			continue
		}
		f := model.FixedNonWorkWeekdaysForDate(fnwRows, ds)
		if model.IsEmployeeWorkday(d0, f) {
			want += model.DailyHours(24, fixed)
		}
	}
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
	if got != 12 {
		t.Fatalf("got %v want 12 (Wed+Thu at 6h each)", got)
	}
}

func TestTargetHoursMonthByWeekHistory_NoEntryBeforeMonthStart(t *testing.T) {
	ctx := context.Background()
	loc := time.Local
	wh := stubWeeklyHoursForDateStore{byDate: map[string]float64{}}
	for d0 := time.Date(2026, 5, 20, 0, 0, 0, 0, loc); d0.Month() == time.May; d0 = d0.AddDate(0, 0, 1) {
		wh.byDate[d0.Format("2006-01-02")] = 30
	}
	got, err := TargetHoursMonthByWeekHistory(ctx, 1, 2026, 5, nil, wh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var want float64
	for d0 := time.Date(2026, 5, 20, 0, 0, 0, 0, loc); d0.Month() == time.May; d0 = d0.AddDate(0, 0, 1) {
		if d0.Weekday() != time.Saturday && d0.Weekday() != time.Sunday {
			want += 6
		}
	}
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}
