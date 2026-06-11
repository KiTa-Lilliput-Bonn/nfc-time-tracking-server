package timesummary

import (
	"context"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestSumWorkedHoursFromStore_ScheduleUnboundSkipsShiftClip(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	users := sqlite.NewUserStore(db)
	schedules := sqlite.NewScheduleStore(db)
	wps := sqlite.NewWorkPeriodStore(db)
	sb := sqlite.NewScheduleBoundStore(db)

	u := &model.User{
		Username: "u1", DisplayName: "U1", Role: model.RoleUser, Active: true,
		PasswordHash: "x",
	}
	if err := users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := sb.Set(ctx, &model.ScheduleBoundSetting{
		UserID: u.ID, ScheduleBound: false, ValidFrom: "2026-06-01",
	}); err != nil {
		t.Fatal(err)
	}
	if err := schedules.Set(ctx, &model.Schedule{
		UserID: u.ID, ScheduleDate: "2026-06-03", ShiftStart: "08:00", ShiftEnd: "16:00",
	}); err != nil {
		t.Fatal(err)
	}

	loc := time.Local
	day, _ := time.ParseInLocation("2006-01-02", "2026-06-03", loc)
	pin := time.Date(day.Year(), day.Month(), day.Day(), 7, 30, 0, 0, loc)
	pout := time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, loc)
	wp := &model.WorkPeriod{
		UserID: u.ID, WorkDate: "2026-06-03", PunchIn: pin, PunchOut: &pout,
	}
	if err := wps.CreateManual(ctx, wp); err != nil {
		t.Fatal(err)
	}
	periods, err := wps.ListByUserDateRange(ctx, u.ID, "2026-06-03", "2026-06-03")
	if err != nil {
		t.Fatal(err)
	}

	got, err := SumWorkedHoursFromStore(ctx, u.ID, periods, schedules, nil, sb)
	if err != nil {
		t.Fatal(err)
	}
	want := 8.5
	if got != want {
		t.Fatalf("unbound hours got %v want %v (no shift-start clip)", got, want)
	}
}

func TestSumWorkedHoursFromStore_ManualBeforeShiftStartNotClippedWhenBound(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	users := sqlite.NewUserStore(db)
	schedules := sqlite.NewScheduleStore(db)
	wps := sqlite.NewWorkPeriodStore(db)
	sb := sqlite.NewScheduleBoundStore(db)

	u := &model.User{
		Username: "u2", DisplayName: "U2", Role: model.RoleUser, Active: true,
		PasswordHash: "x",
	}
	if err := users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := sb.Set(ctx, &model.ScheduleBoundSetting{
		UserID: u.ID, ScheduleBound: true, ValidFrom: "2026-06-01",
	}); err != nil {
		t.Fatal(err)
	}
	if err := schedules.Set(ctx, &model.Schedule{
		UserID: u.ID, ScheduleDate: "2026-06-03", ShiftStart: "08:00", ShiftEnd: "16:00",
	}); err != nil {
		t.Fatal(err)
	}

	loc := time.Local
	day, _ := time.ParseInLocation("2006-01-02", "2026-06-03", loc)
	pin := time.Date(day.Year(), day.Month(), day.Day(), 7, 30, 0, 0, loc)
	pout := time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, loc)
	wp := &model.WorkPeriod{
		UserID: u.ID, WorkDate: "2026-06-03", PunchIn: pin, PunchOut: &pout,
	}
	if err := wps.CreateManual(ctx, wp); err != nil {
		t.Fatal(err)
	}
	periods, err := wps.ListByUserDateRange(ctx, u.ID, "2026-06-03", "2026-06-03")
	if err != nil {
		t.Fatal(err)
	}

	got, err := SumWorkedHoursFromStore(ctx, u.ID, periods, schedules, nil, sb)
	if err != nil {
		t.Fatal(err)
	}
	want := 8.5
	if got != want {
		t.Fatalf("bound manual before shift start: got %v want %v (full hours, no clip)", got, want)
	}
}
