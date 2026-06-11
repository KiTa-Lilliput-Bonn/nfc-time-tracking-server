package handler

import (
	"context"
	"testing"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestBuildAbsenceCredits_ZeroWithoutWeeklyHours(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)
	as := sqlite.NewAbsenceStore(db)
	fnw := sqlite.NewFixedNonWorkWeekdaysStore(db)

	u := &model.User{Username: "acred", PasswordHash: "x", DisplayName: "ACred", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := as.Create(ctx, &model.Absence{
		UserID: u.ID, AbsenceDate: "2026-03-10", AbsenceType: model.AbsenceVacation, CreatedBy: u.ID,
	}); err != nil {
		t.Fatal(err)
	}

	out := buildAbsenceCredits(ctx, u.ID, "2026-03-01", "2026-03-31", fnw, whs, nil, as)
	if len(out) != 1 {
		t.Fatalf("want 1 credit row, got %d", len(out))
	}
	if out[0].CreditHours != 0 {
		t.Fatalf("credit_hours want 0, got %v", out[0].CreditHours)
	}
}

func TestBuildAbsenceCredits_UsesDailyTarget(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)
	as := sqlite.NewAbsenceStore(db)
	fnw := sqlite.NewFixedNonWorkWeekdaysStore(db)

	u := &model.User{Username: "acred2", PasswordHash: "x", DisplayName: "ACred2", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: u.ID, HoursPerWeek: 30, ValidFrom: "2026-03-01"}); err != nil {
		t.Fatal(err)
	}
	if err := as.Create(ctx, &model.Absence{
		UserID: u.ID, AbsenceDate: "2026-03-10", AbsenceType: model.AbsenceVacation, CreatedBy: u.ID,
	}); err != nil {
		t.Fatal(err)
	}

	out := buildAbsenceCredits(ctx, u.ID, "2026-03-01", "2026-03-31", fnw, whs, nil, as)
	if len(out) != 1 {
		t.Fatalf("want 1 credit row, got %d", len(out))
	}
	if out[0].CreditHours != 6 {
		t.Fatalf("credit_hours want 6, got %v", out[0].CreditHours)
	}
}

func TestBuildAbsenceCredits_FourDayWeekOnBoundaryDay(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)
	as := sqlite.NewAbsenceStore(db)
	fnw := sqlite.NewFixedNonWorkWeekdaysStore(db)

	u := &model.User{Username: "acred3", PasswordHash: "x", DisplayName: "ACred3", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: u.ID, HoursPerWeek: 24, ValidFrom: "2026-05-27"}); err != nil {
		t.Fatal(err)
	}
	if err := fnw.Set(ctx, &model.FixedNonWorkWeekdays{
		UserID: u.ID, Weekdays: []int{5}, ValidFrom: "2026-05-27",
	}); err != nil {
		t.Fatal(err)
	}
	if err := as.Create(ctx, &model.Absence{
		UserID: u.ID, AbsenceDate: "2026-05-27", AbsenceType: model.AbsenceVacation, CreatedBy: u.ID,
	}); err != nil {
		t.Fatal(err)
	}

	list, err := fnw.ListByUser(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 fnw row, got %d", len(list))
	}
	list[0].ValidFrom = "2026-05-27T00:00:00Z"

	out := buildAbsenceCredits(ctx, u.ID, "2026-05-01", "2026-05-31", stubFNWListStore{rows: list}, whs, nil, as)
	if len(out) != 1 {
		t.Fatalf("want 1 credit row, got %d", len(out))
	}
	if out[0].CreditHours != 6 {
		t.Fatalf("credit_hours want 6 (24h/4-day week), got %v", out[0].CreditHours)
	}
}

type stubFNWListStore struct {
	rows []model.FixedNonWorkWeekdays
}

func (s stubFNWListStore) Set(ctx context.Context, row *model.FixedNonWorkWeekdays) error { return nil }
func (s stubFNWListStore) Delete(ctx context.Context, userID int, id int) error           { return nil }
func (s stubFNWListStore) GetByID(ctx context.Context, userID int, id int) (*model.FixedNonWorkWeekdays, error) {
	return nil, nil
}
func (s stubFNWListStore) GetForDate(ctx context.Context, userID int, date string) (*model.FixedNonWorkWeekdays, error) {
	return nil, nil
}
func (s stubFNWListStore) ListByUser(ctx context.Context, userID int) ([]model.FixedNonWorkWeekdays, error) {
	return s.rows, nil
}
