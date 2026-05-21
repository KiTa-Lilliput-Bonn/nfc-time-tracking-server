package compensationday

import (
	"context"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestSyncClaimAfterWorkDayChange_CreatesSeparateWeekendClaims(t *testing.T) {
	db := openCompensationDayTestDB(t)
	ctx := context.Background()
	users := sqlite.NewUserStore(db)
	workPeriods := sqlite.NewWorkPeriodStore(db)
	corrections := sqlite.NewCorrectionStore(db)
	claims := sqlite.NewCompensationDayClaimStore(db)
	user := createCompensationDayTestUser(t, ctx, users, "weekendclaims")

	insertImportedPeriod(t, ctx, workPeriods, user.ID, "2026-04-04", 9, 10, false)
	insertImportedPeriod(t, ctx, workPeriods, user.ID, "2026-04-05", 9, 10, false)

	if err := SyncClaimAfterWorkDayChange(ctx, users, workPeriods, corrections, claims, user.ID, "2026-04-04"); err != nil {
		t.Fatal(err)
	}
	if err := SyncClaimAfterWorkDayChange(ctx, users, workPeriods, corrections, claims, user.ID, "2026-04-05"); err != nil {
		t.Fatal(err)
	}

	open, err := claims.CountOpen(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if open != 2 {
		t.Fatalf("open claims want 2, got %d", open)
	}
}

func TestSyncClaimAfterWorkDayChange_KeepsOneClaimForMultipleSameDayPeriods(t *testing.T) {
	db := openCompensationDayTestDB(t)
	ctx := context.Background()
	users := sqlite.NewUserStore(db)
	workPeriods := sqlite.NewWorkPeriodStore(db)
	corrections := sqlite.NewCorrectionStore(db)
	claims := sqlite.NewCompensationDayClaimStore(db)
	user := createCompensationDayTestUser(t, ctx, users, "onedayclaim")

	insertImportedPeriod(t, ctx, workPeriods, user.ID, "2026-04-04", 9, 10, false)
	insertManualPeriod(t, ctx, workPeriods, user.ID, "2026-04-04", 12, 13, false)

	if err := SyncClaimAfterWorkDayChange(ctx, users, workPeriods, corrections, claims, user.ID, "2026-04-04"); err != nil {
		t.Fatal(err)
	}
	if err := SyncClaimAfterWorkDayChange(ctx, users, workPeriods, corrections, claims, user.ID, "2026-04-04"); err != nil {
		t.Fatal(err)
	}

	list, err := claims.ListByUser(ctx, user.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Status != model.CompensationDayClaimOpen {
		t.Fatalf("expected one open claim, got %+v", list)
	}
}

func TestSyncClaimAfterWorkDayChange_IgnoresBreakOnlyWeekendWork(t *testing.T) {
	db := openCompensationDayTestDB(t)
	ctx := context.Background()
	users := sqlite.NewUserStore(db)
	workPeriods := sqlite.NewWorkPeriodStore(db)
	corrections := sqlite.NewCorrectionStore(db)
	claims := sqlite.NewCompensationDayClaimStore(db)
	user := createCompensationDayTestUser(t, ctx, users, "breakonly")

	insertImportedPeriod(t, ctx, workPeriods, user.ID, "2026-04-04", 9, 10, true)

	if err := SyncClaimAfterWorkDayChange(ctx, users, workPeriods, corrections, claims, user.ID, "2026-04-04"); err != nil {
		t.Fatal(err)
	}

	open, err := claims.CountOpen(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if open != 0 {
		t.Fatalf("open claims want 0, got %d", open)
	}
}

func TestSyncClaimAfterWorkDayChange_RemovesOpenClaimWhenCorrectionEliminatesEligibleWork(t *testing.T) {
	db := openCompensationDayTestDB(t)
	ctx := context.Background()
	users := sqlite.NewUserStore(db)
	workPeriods := sqlite.NewWorkPeriodStore(db)
	corrections := sqlite.NewCorrectionStore(db)
	claims := sqlite.NewCompensationDayClaimStore(db)
	user := createCompensationDayTestUser(t, ctx, users, "correctedaway")

	insertImportedPeriod(t, ctx, workPeriods, user.ID, "2026-04-04", 9, 10, false)
	if err := SyncClaimAfterWorkDayChange(ctx, users, workPeriods, corrections, claims, user.ID, "2026-04-04"); err != nil {
		t.Fatal(err)
	}

	periods, err := workPeriods.ListByUserDateRange(ctx, user.ID, "2026-04-04", "2026-04-04")
	if err != nil {
		t.Fatal(err)
	}
	if len(periods) != 1 {
		t.Fatalf("expected one work period, got %d", len(periods))
	}
	corrected := timeForDateHour("2026-04-04", 9)
	if err := corrections.Create(ctx, &model.TimeCorrection{
		WorkPeriodID: periods[0].ID,
		CorrectedIn:  corrected,
		CorrectedOut: corrected,
		Reason:       "no work",
		CorrectedBy:  user.ID,
	}); err != nil {
		t.Fatal(err)
	}

	if err := SyncClaimAfterWorkDayChange(ctx, users, workPeriods, corrections, claims, user.ID, "2026-04-04"); err != nil {
		t.Fatal(err)
	}
	open, err := claims.CountOpen(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if open != 0 {
		t.Fatalf("open claims after correction want 0, got %d", open)
	}
}

func openCompensationDayTestDB(t *testing.T) *sqlite.DB {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func createCompensationDayTestUser(t *testing.T, ctx context.Context, users *sqlite.UserStore, username string) *model.User {
	t.Helper()
	user := &model.User{
		Username:     username,
		PasswordHash: "x",
		DisplayName:  username,
		Role:         model.RoleUser,
		Active:       true,
	}
	if err := users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	return user
}

func insertImportedPeriod(t *testing.T, ctx context.Context, workPeriods *sqlite.WorkPeriodStore, userID int, date string, startHour int, endHour int, isBreak bool) {
	t.Helper()
	start := timeForDateHour(date, startHour)
	end := timeForDateHour(date, endHour)
	if err := workPeriods.ReplaceForUserDate(ctx, userID, date, []model.WorkPeriod{{
		PunchIn:  start,
		PunchOut: &end,
		IsBreak:  isBreak,
	}}); err != nil {
		t.Fatal(err)
	}
}

func insertManualPeriod(t *testing.T, ctx context.Context, workPeriods *sqlite.WorkPeriodStore, userID int, date string, startHour int, endHour int, isBreak bool) {
	t.Helper()
	start := timeForDateHour(date, startHour)
	end := timeForDateHour(date, endHour)
	if err := workPeriods.CreateManual(ctx, &model.WorkPeriod{
		UserID:   userID,
		WorkDate: date,
		PunchIn:  start,
		PunchOut: &end,
		IsBreak:  isBreak,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSyncClaimAfterWorkDayChange_FixedNonWorkWeekday(t *testing.T) {
	db := openCompensationDayTestDB(t)
	ctx := context.Background()
	users := sqlite.NewUserStore(db)
	workPeriods := sqlite.NewWorkPeriodStore(db)
	corrections := sqlite.NewCorrectionStore(db)
	claims := sqlite.NewCompensationDayClaimStore(db)
	user := createCompensationDayTestUser(t, ctx, users, "fixedfriday")
	user.FixedNonWorkWeekdays = []int{int(time.Friday)}
	if err := users.Update(ctx, user); err != nil {
		t.Fatal(err)
	}
	// 2026-03-13 is a Friday
	insertImportedPeriod(t, ctx, workPeriods, user.ID, "2026-03-13", 9, 10, false)
	if err := SyncClaimAfterWorkDayChange(ctx, users, workPeriods, corrections, claims, user.ID, "2026-03-13"); err != nil {
		t.Fatal(err)
	}
	open, err := claims.CountOpen(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if open != 1 {
		t.Fatalf("open claims want 1 for work on fixed-free Friday, got %d", open)
	}
}

func timeForDateHour(date string, hour int) time.Time {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		panic(err)
	}
	return time.Date(d.Year(), d.Month(), d.Day(), hour, 0, 0, 0, time.UTC)
}
