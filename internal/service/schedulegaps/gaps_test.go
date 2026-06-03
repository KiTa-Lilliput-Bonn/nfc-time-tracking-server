package schedulegaps

import (
	"context"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestBuild_PlannedYesterdayNoWorkOrAbsence(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ss := sqlite.NewScheduleStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)

	u := &model.User{Username: "gap1", PasswordHash: "x", DisplayName: "Gap User", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: u.ID, HoursPerWeek: 40, ValidFrom: "2026-03-01"}); err != nil {
		t.Fatal(err)
	}

	yesterday := "2026-03-11"
	if err := ss.Set(ctx, &model.Schedule{
		UserID: u.ID, ScheduleDate: yesterday, ShiftStart: "08:00", ShiftEnd: "16:00",
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local)
	res, err := Build(ctx, Deps{Users: us, Schedules: ss, WorkPeriods: sqlite.NewWorkPeriodStore(db), Absences: sqlite.NewAbsenceStore(db), WeeklyHours: whs}, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 1 || len(res.Items) != 1 {
		t.Fatalf("want 1 gap, got count=%d items=%d", res.Count, len(res.Items))
	}
	if res.Items[0].ScheduleDate != yesterday || res.Items[0].UserID != u.ID {
		t.Fatalf("unexpected item: %+v", res.Items[0])
	}
	if res.Through != yesterday {
		t.Fatalf("through: want %s, got %s", yesterday, res.Through)
	}
}

func TestBuild_SickAbsenceExcluded(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ss := sqlite.NewScheduleStore(db)
	as := sqlite.NewAbsenceStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)

	u := &model.User{Username: "gap2", PasswordHash: "x", DisplayName: "Sick User", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	yesterday := "2026-03-11"
	if err := ss.Set(ctx, &model.Schedule{UserID: u.ID, ScheduleDate: yesterday, ShiftStart: "08:00", ShiftEnd: "16:00"}); err != nil {
		t.Fatal(err)
	}
	if err := as.Create(ctx, &model.Absence{UserID: u.ID, AbsenceDate: yesterday, AbsenceType: model.AbsenceSick, CreatedBy: u.ID}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local)
	res, err := Build(ctx, Deps{Users: us, Schedules: ss, WorkPeriods: sqlite.NewWorkPeriodStore(db), Absences: as, WeeklyHours: whs}, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 0 {
		t.Fatalf("want 0 gaps with sick absence, got %d", res.Count)
	}
}

func TestBuild_WorkPeriodExcluded(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ss := sqlite.NewScheduleStore(db)
	ws := sqlite.NewWorkPeriodStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)

	u := &model.User{Username: "gap3", PasswordHash: "x", DisplayName: "Worked User", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	yesterday := "2026-03-11"
	if err := ss.Set(ctx, &model.Schedule{UserID: u.ID, ScheduleDate: yesterday, ShiftStart: "08:00", ShiftEnd: "16:00"}); err != nil {
		t.Fatal(err)
	}
	tIn := time.Date(2026, 3, 11, 8, 0, 0, 0, time.UTC)
	tOut := time.Date(2026, 3, 11, 16, 0, 0, 0, time.UTC)
	if err := ws.ReplaceForUserDate(ctx, u.ID, yesterday, []model.WorkPeriod{{PunchIn: tIn, PunchOut: &tOut, IsBreak: false}}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local)
	res, err := Build(ctx, Deps{Users: us, Schedules: ss, WorkPeriods: ws, Absences: sqlite.NewAbsenceStore(db), WeeklyHours: whs}, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 0 {
		t.Fatalf("want 0 gaps with work period, got %d", res.Count)
	}
}

func TestBuild_TodayScheduleExcluded(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ss := sqlite.NewScheduleStore(db)

	u := &model.User{Username: "gap4", PasswordHash: "x", DisplayName: "Today User", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	today := "2026-03-12"
	if err := ss.Set(ctx, &model.Schedule{UserID: u.ID, ScheduleDate: today, ShiftStart: "08:00", ShiftEnd: "16:00"}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local)
	res, err := Build(ctx, Deps{Users: us, Schedules: ss, WorkPeriods: sqlite.NewWorkPeriodStore(db), Absences: sqlite.NewAbsenceStore(db), WeeklyHours: sqlite.NewWeeklyHoursStore(db)}, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 0 {
		t.Fatalf("want 0 gaps for today-only schedule, got %d", res.Count)
	}
}

func TestBuild_EmptyShiftExcluded(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ss := sqlite.NewScheduleStore(db)

	u := &model.User{Username: "gap5", PasswordHash: "x", DisplayName: "Empty Shift", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := ss.Set(ctx, &model.Schedule{UserID: u.ID, ScheduleDate: "2026-03-11", ShiftStart: "", ShiftEnd: ""}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local)
	res, err := Build(ctx, Deps{Users: us, Schedules: ss, WorkPeriods: sqlite.NewWorkPeriodStore(db), Absences: sqlite.NewAbsenceStore(db), WeeklyHours: sqlite.NewWeeklyHoursStore(db)}, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 0 {
		t.Fatalf("want 0 gaps for empty shift, got %d", res.Count)
	}
}
