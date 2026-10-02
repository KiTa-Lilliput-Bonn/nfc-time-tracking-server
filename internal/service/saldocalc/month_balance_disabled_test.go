package saldocalc

import (
	"context"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestMonthWithOpening_ExcludesDisabledWorkPeriod(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ws := sqlite.NewWorkPeriodStore(db)
	cs := sqlite.NewCorrectionStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)

	u := &model.User{Username: "bal", PasswordHash: "x", DisplayName: "Bal", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: u.ID, HoursPerWeek: 40, ValidFrom: "2026-03-01"}); err != nil {
		t.Fatal(err)
	}

	tIn := time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC)
	tOut := time.Date(2026, 3, 10, 16, 0, 0, 0, time.UTC)
	if err := ws.ReplaceForUserDate(ctx, u.ID, "2026-03-10", []model.WorkPeriod{{PunchIn: tIn, PunchOut: &tOut, IsBreak: false}}); err != nil {
		t.Fatal(err)
	}
	wps, err := ws.ListByUserDateRange(ctx, u.ID, "2026-03-10", "2026-03-10")
	if err != nil || len(wps) != 1 {
		t.Fatalf("wps: %v %+v", err, wps)
	}
	if err := cs.Create(ctx, &model.TimeCorrection{
		WorkPeriodID: wps[0].ID,
		CorrectedIn:  tIn,
		CorrectedOut: tOut,
		Reason:       "Falscher Stempel",
		CorrectedBy:  u.ID,
		Disabled:     true,
	}); err != nil {
		t.Fatal(err)
	}

	mb, err := monthFull(ctx, u.ID, 2026, 3, 0, sqlite.NewFixedNonWorkWeekdaysStore(db), ws, cs, whs, nil, nil, nil, nil, sqlite.NewClosureDayStore(db), sqlite.NewSettingsStore(db))
	if err != nil {
		t.Fatal(err)
	}
	if mb.WorkedHours != 0 {
		t.Fatalf("worked want 0 (disabled), got %v", mb.WorkedHours)
	}
}
