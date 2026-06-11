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
