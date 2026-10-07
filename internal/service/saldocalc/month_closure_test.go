package saldocalc

import (
	"context"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store/sqlite"
)

// Sommerschließung: Schließtage tragen automatisch Urlaub ein. Diese Tage müssen das Soll genauso ausgleichen
// wie normaler Urlaub (früher zählte das Soll, die Gutschrift aber nicht).
func TestMonth_ClosureDaysWithVacationBalanceTarget(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)
	as := sqlite.NewAbsenceStore(db)
	cls := sqlite.NewClosureDayStore(db)

	u := &model.User{Username: "anne", PasswordHash: "x", DisplayName: "Anne", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: u.ID, HoursPerWeek: 34, ValidFrom: "2026-08-01"}); err != nil {
		t.Fatal(err)
	}

	// 03.–07.08. normaler Urlaub, 10.–28.08. Schließzeit mit automatischem Urlaub. Mo–Fr = 20 Tage.
	for d := time.Date(2026, 8, 3, 0, 0, 0, 0, time.Local); d.Day() <= 28; d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		ds := d.Format("2006-01-02")
		if d.Day() >= 10 {
			if err := cls.Create(ctx, &model.ClosureDay{ClosureDate: ds, Name: "Sommerschließung", CreatedBy: u.ID}); err != nil {
				t.Fatal(err)
			}
		}
		if err := as.Create(ctx, &model.Absence{UserID: u.ID, AbsenceDate: ds, AbsenceType: model.AbsenceVacation, CreatedBy: u.ID}); err != nil {
			t.Fatal(err)
		}
	}

	mb, err := monthFull(ctx, u.ID, 2026, 8, 0, sqlite.NewFixedNonWorkWeekdaysStore(db), nil, nil, whs, nil, as, nil, nil, cls, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 21 Werktage à 6,8 h Soll; 20 davon Urlaub/Schließzeit, der 31.08. ohne Stempel.
	if mb.TargetHours != 142.8 {
		t.Fatalf("target want 142.8, got %v", mb.TargetHours)
	}
	if mb.WorkedHours != 136 {
		t.Fatalf("worked want 136 (20 Tage Gutschrift), got %v", mb.WorkedHours)
	}
	if mb.BalanceHours != -6.8 {
		t.Fatalf("balance want -6.8, got %v", mb.BalanceHours)
	}
}
