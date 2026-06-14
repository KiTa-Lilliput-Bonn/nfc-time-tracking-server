package saldocalc

import (
	"context"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestSumRange_NetHoursWithDefaultBreakRules(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ws := sqlite.NewWorkPeriodStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)
	ss := sqlite.NewSettingsStore(db)

	u := &model.User{Username: "net", PasswordHash: "x", DisplayName: "Net", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: u.ID, HoursPerWeek: 40, ValidFrom: "2026-03-10"}); err != nil {
		t.Fatal(err)
	}

	d1 := "2026-03-10"
	tIn := time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC)
	tOut := time.Date(2026, 3, 10, 16, 0, 0, 0, time.UTC)
	if err := ws.ReplaceForUserDate(ctx, u.ID, d1, []model.WorkPeriod{{PunchIn: tIn, PunchOut: &tOut, IsBreak: false}}); err != nil {
		t.Fatal(err)
	}

	deps := Deps{
		WorkPeriods:          ws,
		WeeklyHours:          whs,
		FixedNonWorkWeekdays: sqlite.NewFixedNonWorkWeekdaysStore(db),
		Settings:             ss,
	}
	totals, err := SumRange(ctx, deps, u.ID, d1, d1)
	if err != nil {
		t.Fatal(err)
	}
	if totals.WorkedHours != 7.5 {
		t.Fatalf("worked want 7.5 net, got %v", totals.WorkedHours)
	}
	if totals.TargetHours != 8 {
		t.Fatalf("target want 8, got %v", totals.TargetHours)
	}
	if totals.BalanceHours != -0.5 {
		t.Fatalf("balance want -0.5, got %v", totals.BalanceHours)
	}
}

func TestSumRange_FullDayVacationNeutral(t *testing.T) {
	ctx := context.Background()
	wh := stubWeeklyHoursStore{validFrom: "2026-03-01", hours: 30}
	deps := Deps{
		WorkPeriods:          stubWorkPeriodStore{},
		WeeklyHours:          wh,
		FixedNonWorkWeekdays: stubFNWStore{},
		Absences: stubAbsenceStore{byRange: []model.Absence{{
			UserID: 1, AbsenceDate: "2026-03-10", AbsenceType: model.AbsenceVacation,
		}}},
	}
	totals, err := SumRange(ctx, deps, 1, "2026-03-10", "2026-03-10")
	if err != nil {
		t.Fatal(err)
	}
	if totals.WorkedHours != 6 {
		t.Fatalf("worked want 6, got %v", totals.WorkedHours)
	}
	if totals.TargetHours != 6 {
		t.Fatalf("target want 6, got %v", totals.TargetHours)
	}
	if totals.BalanceHours != 0 {
		t.Fatalf("balance want 0, got %v", totals.BalanceHours)
	}
}
