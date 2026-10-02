package vacationbalance

import (
	"context"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func setup(t *testing.T) (context.Context, *sqlite.UserStore, *sqlite.AbsenceStore, *sqlite.VacationEntitlementStore, *model.User) {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	us := sqlite.NewUserStore(db)
	u := &model.User{Username: "anna", PasswordHash: "x", DisplayName: "Anna", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	return ctx, us, sqlite.NewAbsenceStore(db), sqlite.NewVacationEntitlementStore(db), u
}

func addVacation(t *testing.T, ctx context.Context, as *sqlite.AbsenceStore, uid int, half bool, dates ...string) {
	t.Helper()
	for _, d := range dates {
		if err := as.Create(ctx, &model.Absence{UserID: uid, AbsenceDate: d, AbsenceType: model.AbsenceVacation, HalfDay: half, CreatedBy: uid}); err != nil {
			t.Fatal(err)
		}
	}
}

// Szenario aus dem Review: 30 Tage/Jahr seit 2025, 18 Tage 2025 genommen → 12 Übertrag;
// 2026: 10,5 genommen, 5 geplant.
func TestCompute_BreakdownAddsUp(t *testing.T) {
	ctx, us, as, ves, u := setup(t)
	if err := ves.Set(ctx, &model.VacationEntitlement{UserID: u.ID, DaysPerYear: 30, ValidFrom: "2025-01-01"}); err != nil {
		t.Fatal(err)
	}
	var prev []string
	for d := time.Date(2025, 7, 1, 0, 0, 0, 0, time.Local); len(prev) < 18; d = d.AddDate(0, 0, 1) {
		prev = append(prev, d.Format("2006-01-02"))
	}
	addVacation(t, ctx, as, u.ID, false, prev...)
	addVacation(t, ctx, as, u.ID, false, "2026-04-07", "2026-04-08", "2026-04-09", "2026-08-03", "2026-08-04",
		"2026-08-05", "2026-08-06", "2026-08-10", "2026-08-11", "2026-08-12")
	addVacation(t, ctx, as, u.ID, true, "2026-09-15")
	addVacation(t, ctx, as, u.ID, false, "2026-10-19", "2026-10-20", "2026-10-21", "2026-12-28", "2026-12-29")

	today := time.Date(2026, 10, 2, 12, 0, 0, 0, time.Local)
	vb, err := ComputeForUser(ctx, u.ID, us, ves, as, today)
	if err != nil {
		t.Fatal(err)
	}
	want := model.VacationBalance{Year: 2026, CarriedOver: 0, Carryover: 12, Entitlement: 30, Total: 42, Taken: 10.5, Remaining: 31.5, Planned: 5, Free: 26.5}
	if *vb != want {
		t.Fatalf("got %+v\nwant %+v", *vb, want)
	}
}

func TestCompute_OpeningAndRulesStartingThisYear(t *testing.T) {
	ctx, us, as, ves, u := setup(t)
	u.OpeningVacationDays = 4
	if err := us.Update(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := ves.Set(ctx, &model.VacationEntitlement{UserID: u.ID, DaysPerYear: 24, ValidFrom: "2026-07-01"}); err != nil {
		t.Fatal(err)
	}
	addVacation(t, ctx, as, u.ID, false, "2026-03-02") // vor Regelbeginn: zählt nicht als genommen
	addVacation(t, ctx, as, u.ID, false, "2026-08-03")
	vb, err := ComputeForUser(ctx, u.ID, us, ves, as, time.Date(2026, 10, 2, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if vb.Carryover != 0 || vb.Entitlement != 12 || vb.CarriedOver != 4 || vb.Total != 16 || vb.Taken != 1 || vb.Remaining != 15 {
		t.Fatalf("unexpected %+v", *vb)
	}
}

func TestCompute_NoRules(t *testing.T) {
	ctx, us, as, ves, u := setup(t)
	addVacation(t, ctx, as, u.ID, false, "2025-05-05", "2026-05-05")
	vb, err := ComputeForUser(ctx, u.ID, us, ves, as, time.Date(2026, 10, 2, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if vb.Carryover != 0 || vb.Entitlement != 0 || vb.Taken != 1 || vb.Remaining != -1 {
		t.Fatalf("unexpected %+v", *vb)
	}
}
