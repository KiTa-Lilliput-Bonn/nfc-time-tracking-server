package saldocalc

import (
	"context"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
)

func accountTestDeps() Deps {
	return Deps{
		WorkPeriods:          stubWorkPeriodStore{},
		Absences:             stubAbsenceStore{},
		Holidays:             stubHolidayStore{},
		WeeklyHours:          stubWeeklyHoursStore{validFrom: "2025-12-01", hours: 40},
		FixedNonWorkWeekdays: stubFNWStore{},
	}
}

func TestMonth_CurrentMonthCountsOnlyThroughYesterday(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 15, 9, 0, 0, 0, time.Local) // gestern = Mi 14.01.
	mb, err := Month(ctx, accountTestDeps(), &model.User{ID: 1}, 2026, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	// 01.–14.01.2026: 10 Werktage à 8 h
	if mb.TargetHours != 80 || mb.BalanceHours != -80 {
		t.Fatalf("target/balance got %v/%v want 80/-80", mb.TargetHours, mb.BalanceHours)
	}
	if !mb.IsPartial || mb.IsFuture || mb.CountedThrough != "2026-01-14" || mb.CountedFrom != "2026-01-01" {
		t.Fatalf("flags/range unexpected: %+v", mb)
	}
}

func TestMonth_CarryoverSpansYearBoundary(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 15, 9, 0, 0, 0, time.Local)
	mb, err := Month(ctx, accountTestDeps(), &model.User{ID: 1}, 2026, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	// Dezember 2025: 23 Werktage à 8 h = −184 h Vortrag (früher: Vortrag ab 01.01. → 0)
	if mb.Carryover != -184 {
		t.Fatalf("carryover got %v want -184", mb.Carryover)
	}
	if mb.TotalBalance != -264 {
		t.Fatalf("total got %v want -264", mb.TotalBalance)
	}
	if mb.AccountStart != "2025-12-01" {
		t.Fatalf("account start got %q", mb.AccountStart)
	}
}

func TestMonth_FutureMonthShowsCurrentAccount(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 15, 9, 0, 0, 0, time.Local)
	mb, err := Month(ctx, accountTestDeps(), &model.User{ID: 1}, 2026, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	if !mb.IsFuture || mb.TargetHours != 0 || mb.WorkedHours != 0 || mb.CountedThrough != "" {
		t.Fatalf("future month should count nothing: %+v", mb)
	}
	if mb.TotalBalance != -264 {
		t.Fatalf("total got %v want -264 (Stand gestern)", mb.TotalBalance)
	}
}

func TestMonth_MatchesAccount(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 1, 15, 9, 0, 0, 0, time.Local)
	u := &model.User{ID: 1, OpeningHoursBalance: 10, CreatedAt: time.Date(2025, 12, 10, 8, 0, 0, 0, time.Local)}
	mb, err := Month(ctx, accountTestDeps(), u, 2026, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	acc, err := Account(ctx, accountTestDeps(), u, now)
	if err != nil {
		t.Fatal(err)
	}
	if !acc.OpeningApplied || acc.Balance != -254 || mb.TotalBalance != acc.Balance {
		t.Fatalf("account %+v month total %v, want -254 with opening", acc, mb.TotalBalance)
	}
}

func TestOpeningApplies_OutsideRange(t *testing.T) {
	loc := time.Local
	start := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	through := time.Date(2026, 3, 1, 0, 0, 0, 0, loc)
	if OpeningApplies(time.Date(2026, 1, 15, 0, 0, 0, 0, loc), start, through, loc) {
		t.Fatal("created before account start must not apply")
	}
	if !OpeningApplies(time.Date(2026, 2, 15, 0, 0, 0, 0, loc), start, through, loc) {
		t.Fatal("created within range must apply")
	}
}
