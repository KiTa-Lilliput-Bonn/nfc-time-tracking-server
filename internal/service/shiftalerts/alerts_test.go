package shiftalerts

import (
	"context"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func shiftAlertDeps(db *sqlite.DB) Deps {
	return Deps{
		Users:       sqlite.NewUserStore(db),
		WorkPeriods: sqlite.NewWorkPeriodStore(db),
		Corrections: sqlite.NewCorrectionStore(db),
		WeeklyHours: sqlite.NewWeeklyHoursStore(db),
		Dismissals:  sqlite.NewShiftAlertDismissalStore(db),
	}
}

func defaultConfig() Config {
	return Config{MaxHours: 11, LateEnd: "21:00"}
}

func TestBuild_LongDurationDay(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ws := sqlite.NewWorkPeriodStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)

	u := &model.User{Username: "long1", PasswordHash: "x", DisplayName: "Long User", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: u.ID, HoursPerWeek: 40, ValidFrom: "2026-03-01"}); err != nil {
		t.Fatal(err)
	}

	workDate := "2026-03-11"
	tIn := time.Date(2026, 3, 11, 6, 0, 0, 0, time.Local)
	tOut := time.Date(2026, 3, 11, 18, 30, 0, 0, time.Local)
	if err := ws.ReplaceForUserDate(ctx, u.ID, workDate, []model.WorkPeriod{{PunchIn: tIn, PunchOut: &tOut, IsBreak: false}}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local)
	res, err := Build(ctx, shiftAlertDeps(db), defaultConfig(), now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 1 {
		t.Fatalf("want 1 alert, got %d", res.Count)
	}
	if len(res.Items[0].Reasons) != 1 || res.Items[0].Reasons[0] != ReasonLongDuration {
		t.Fatalf("unexpected reasons: %v", res.Items[0].Reasons)
	}
}

func TestBuild_LateEnd(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ws := sqlite.NewWorkPeriodStore(db)

	u := &model.User{Username: "late1", PasswordHash: "x", DisplayName: "Late User", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	workDate := "2026-03-11"
	tIn := time.Date(2026, 3, 11, 14, 0, 0, 0, time.Local)
	tOut := time.Date(2026, 3, 11, 21, 30, 0, 0, time.Local)
	if err := ws.ReplaceForUserDate(ctx, u.ID, workDate, []model.WorkPeriod{{PunchIn: tIn, PunchOut: &tOut, IsBreak: false}}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local)
	res, err := Build(ctx, shiftAlertDeps(db), defaultConfig(), now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 1 {
		t.Fatalf("want 1 alert, got %d", res.Count)
	}
	if res.Items[0].Reasons[0] != ReasonLateEnd {
		t.Fatalf("want late_end, got %v", res.Items[0].Reasons)
	}
}

func TestBuild_CorrectionShortensBelowThreshold(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ws := sqlite.NewWorkPeriodStore(db)
	cs := sqlite.NewCorrectionStore(db)

	u := &model.User{Username: "corr1", PasswordHash: "x", DisplayName: "Corrected User", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	lead := &model.User{Username: "lead1", PasswordHash: "x", DisplayName: "Lead", Role: model.RoleLeitung, Active: true}
	if err := us.Create(ctx, lead); err != nil {
		t.Fatal(err)
	}

	workDate := "2026-03-11"
	tIn := time.Date(2026, 3, 11, 6, 0, 0, 0, time.Local)
	tOut := time.Date(2026, 3, 11, 18, 30, 0, 0, time.Local)
	if err := ws.ReplaceForUserDate(ctx, u.ID, workDate, []model.WorkPeriod{{PunchIn: tIn, PunchOut: &tOut, IsBreak: false}}); err != nil {
		t.Fatal(err)
	}
	wps, err := ws.ListByUserDateRange(ctx, u.ID, workDate, workDate)
	if err != nil || len(wps) != 1 {
		t.Fatal(err)
	}
	cOut := time.Date(2026, 3, 11, 16, 0, 0, 0, time.Local)
	if err := cs.Create(ctx, &model.TimeCorrection{
		WorkPeriodID: wps[0].ID, CorrectedIn: tIn, CorrectedOut: cOut, Reason: "fix", CorrectedBy: lead.ID,
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local)
	res, err := Build(ctx, shiftAlertDeps(db), defaultConfig(), now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 0 {
		t.Fatalf("want 0 after correction, got %d", res.Count)
	}
}

func TestBuild_DismissalExcluded(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ws := sqlite.NewWorkPeriodStore(db)
	ds := sqlite.NewShiftAlertDismissalStore(db)

	u := &model.User{Username: "dis1", PasswordHash: "x", DisplayName: "Dismissed User", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	lead := &model.User{Username: "lead2", PasswordHash: "x", DisplayName: "Lead", Role: model.RoleLeitung, Active: true}
	if err := us.Create(ctx, lead); err != nil {
		t.Fatal(err)
	}

	workDate := "2026-03-11"
	tIn := time.Date(2026, 3, 11, 6, 0, 0, 0, time.Local)
	tOut := time.Date(2026, 3, 11, 18, 30, 0, 0, time.Local)
	if err := ws.ReplaceForUserDate(ctx, u.ID, workDate, []model.WorkPeriod{{PunchIn: tIn, PunchOut: &tOut, IsBreak: false}}); err != nil {
		t.Fatal(err)
	}
	if err := ds.Create(ctx, u.ID, workDate, lead.ID); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local)
	res, err := Build(ctx, shiftAlertDeps(db), defaultConfig(), now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 0 {
		t.Fatalf("want 0 with dismissal, got %d", res.Count)
	}
}

func TestBuild_TodayExcluded(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ws := sqlite.NewWorkPeriodStore(db)

	u := &model.User{Username: "today1", PasswordHash: "x", DisplayName: "Today User", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	workDate := "2026-03-12"
	tIn := time.Date(2026, 3, 12, 6, 0, 0, 0, time.Local)
	tOut := time.Date(2026, 3, 12, 18, 30, 0, 0, time.Local)
	if err := ws.ReplaceForUserDate(ctx, u.ID, workDate, []model.WorkPeriod{{PunchIn: tIn, PunchOut: &tOut, IsBreak: false}}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local)
	res, err := Build(ctx, shiftAlertDeps(db), defaultConfig(), now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 0 {
		t.Fatalf("want 0 for today, got %d", res.Count)
	}
}

func TestBuild_DisabledChecks(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ws := sqlite.NewWorkPeriodStore(db)

	u := &model.User{Username: "off1", PasswordHash: "x", DisplayName: "Off User", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	workDate := "2026-03-11"
	tIn := time.Date(2026, 3, 11, 6, 0, 0, 0, time.Local)
	tOut := time.Date(2026, 3, 11, 22, 0, 0, 0, time.Local)
	if err := ws.ReplaceForUserDate(ctx, u.ID, workDate, []model.WorkPeriod{{PunchIn: tIn, PunchOut: &tOut, IsBreak: false}}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local)
	res, err := Build(ctx, shiftAlertDeps(db), Config{MaxHours: 0, LateEnd: ""}, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 0 {
		t.Fatalf("want 0 with disabled checks, got %d", res.Count)
	}
}

func TestValidateConfig(t *testing.T) {
	if err := ValidateConfig(Config{MaxHours: 11, LateEnd: "21:00"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateConfig(Config{MaxHours: 25, LateEnd: "21:00"}); err == nil {
		t.Fatal("expected error for max_hours > 24")
	}
	if err := ValidateConfig(Config{MaxHours: 11, LateEnd: "25:00"}); err == nil {
		t.Fatal("expected error for invalid late_end")
	}
}
