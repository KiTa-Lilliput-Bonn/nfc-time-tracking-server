package teamoverview

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestOverview_HoursBalanceSinceAsOf(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ws := sqlite.NewWorkPeriodStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)
	ss := sqlite.NewSettingsStore(db)
	cs := sqlite.NewCorrectionStore(db)

	u := &model.User{Username: "bal", PasswordHash: "x", DisplayName: "Balance User", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: u.ID, HoursPerWeek: 40, ValidFrom: "2026-03-10"}); err != nil {
		t.Fatal(err)
	}

	// now = 2026-03-12 local → today 2026-03-12, yesterday 2026-03-11
	// Range frühestes Stundensoll..yesterday = 2026-03-10 .. 2026-03-11 (weekdays, 8h target each day)
	now := time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local)

	// Mar 10: 08:00–16:00 UTC → 8h gross, default break_rules deduct 30m → 7.5h net; Mar 11: no periods → 0 net
	d1 := "2026-03-10"
	tIn := time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC)
	tOut := time.Date(2026, 3, 10, 16, 0, 0, 0, time.UTC)
	if err := ws.ReplaceForUserDate(ctx, u.ID, d1, []model.WorkPeriod{{PunchIn: tIn, PunchOut: &tOut, IsBreak: false}}); err != nil {
		t.Fatal(err)
	}

	d := Deps{
		Users:       us,
		WorkPeriods: ws,
		Corrections: cs,
		Absences:    sqlite.NewAbsenceStore(db),
		Holidays:    sqlite.NewHolidayStore(db),
		Closures:    sqlite.NewClosureDayStore(db),
		WeeklyHours: whs,
		Settings:    ss,
		VacationEnt: sqlite.NewVacationEntitlementStore(db),
	}

	rows, _, err := Build(ctx, d, 2026, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	// Day1: 7.5 net, 8 target → -0.5; Day2: 0 net, 8 target → -8
	want := -8.5
	if rows[0].HoursBalance != want {
		t.Fatalf("hours_balance: want %v, got %v", want, rows[0].HoursBalance)
	}
}

func TestOverview_HoursBalance_AddsAbsenceCredit(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ws := sqlite.NewWorkPeriodStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)
	ss := sqlite.NewSettingsStore(db)
	as := sqlite.NewAbsenceStore(db)

	u := &model.User{Username: "abscred", PasswordHash: "x", DisplayName: "Abs Credit", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: u.ID, HoursPerWeek: 40, ValidFrom: "2026-03-10"}); err != nil {
		t.Fatal(err)
	}

	// One vacation day with 8h worked; expected: +8 credit and target 8 → hours balance +7.5 (net 7.5 + credit 8 - target 8)
	d1 := "2026-03-10"
	tIn := time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC)
	tOut := time.Date(2026, 3, 10, 16, 0, 0, 0, time.UTC)
	if err := ws.ReplaceForUserDate(ctx, u.ID, d1, []model.WorkPeriod{{PunchIn: tIn, PunchOut: &tOut, IsBreak: false}}); err != nil {
		t.Fatal(err)
	}
	if err := as.Create(ctx, &model.Absence{UserID: u.ID, AbsenceDate: d1, AbsenceType: model.AbsenceVacation, HalfDay: false, CreatedBy: u.ID}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 11, 9, 0, 0, 0, time.Local) // yesterday = 2026-03-10
	d := Deps{
		Users:       us,
		WorkPeriods: ws,
		Corrections: sqlite.NewCorrectionStore(db),
		Absences:    as,
		Holidays:    sqlite.NewHolidayStore(db),
		Closures:    sqlite.NewClosureDayStore(db),
		WeeklyHours: whs,
		Settings:    ss,
		VacationEnt: sqlite.NewVacationEntitlementStore(db),
	}
	rows, _, err := Build(ctx, d, 2026, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].HoursBalance != 7.5 {
		t.Fatalf("hours_balance want 7.5, got %v", rows[0].HoursBalance)
	}
}

func TestOverview_HoursBalance_CompensationDayConsumesDailyTarget(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)
	as := sqlite.NewAbsenceStore(db)

	u := &model.User{Username: "compdaytarget", PasswordHash: "x", DisplayName: "Comp Day Target", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: u.ID, HoursPerWeek: 40, ValidFrom: "2026-04-07"}); err != nil {
		t.Fatal(err)
	}
	if err := as.Create(ctx, &model.Absence{UserID: u.ID, AbsenceDate: "2026-04-07", AbsenceType: model.AbsenceCompensationDay, HalfDay: false, CreatedBy: u.ID}); err != nil {
		t.Fatal(err)
	}

	rows, _, err := Build(ctx, Deps{
		Users:       us,
		WorkPeriods: sqlite.NewWorkPeriodStore(db),
		Corrections: sqlite.NewCorrectionStore(db),
		Absences:    as,
		Holidays:    sqlite.NewHolidayStore(db),
		Closures:    sqlite.NewClosureDayStore(db),
		WeeklyHours: whs,
		Settings:    sqlite.NewSettingsStore(db),
		VacationEnt: sqlite.NewVacationEntitlementStore(db),
	}, 2026, time.Date(2026, 4, 8, 10, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].HoursBalance != -8 {
		t.Fatalf("hours_balance want -8, got %v", rows[0].HoursBalance)
	}
}

func TestOverview_HoursBalance_WaivingClaimDoesNotChangeWeekendOvertime(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ws := sqlite.NewWorkPeriodStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)
	claims := sqlite.NewCompensationDayClaimStore(db)

	u := &model.User{Username: "waivebalance", PasswordHash: "x", DisplayName: "Waive Balance", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: u.ID, HoursPerWeek: 40, ValidFrom: "2026-04-04"}); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 4, 4, 9, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	if err := ws.ReplaceForUserDate(ctx, u.ID, "2026-04-04", []model.WorkPeriod{{PunchIn: start, PunchOut: &end}}); err != nil {
		t.Fatal(err)
	}
	if err := claims.EnsureForWorkDate(ctx, u.ID, "2026-04-04", true); err != nil {
		t.Fatal(err)
	}

	deps := Deps{
		Users:                 us,
		WorkPeriods:           ws,
		Corrections:           sqlite.NewCorrectionStore(db),
		Absences:              sqlite.NewAbsenceStore(db),
		Holidays:              sqlite.NewHolidayStore(db),
		Closures:              sqlite.NewClosureDayStore(db),
		WeeklyHours:           whs,
		Settings:              sqlite.NewSettingsStore(db),
		VacationEnt:           sqlite.NewVacationEntitlementStore(db),
		CompensationDayClaims: claims,
	}
	now := time.Date(2026, 4, 5, 10, 0, 0, 0, time.Local)
	before, _, err := Build(ctx, deps, 2026, now)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := claims.GetOldestOpen(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil {
		t.Fatal("expected open claim")
	}
	if err := claims.Waive(ctx, u.ID, claim.ID); err != nil {
		t.Fatal(err)
	}
	after, _, err := Build(ctx, deps, 2026, now)
	if err != nil {
		t.Fatal(err)
	}

	if len(before) != 1 || len(after) != 1 {
		t.Fatalf("expected one row before/after, got %d/%d", len(before), len(after))
	}
	if before[0].HoursBalance != 1 {
		t.Fatalf("hours_balance before waive want 1, got %v", before[0].HoursBalance)
	}
	if after[0].HoursBalance != before[0].HoursBalance {
		t.Fatalf("hours_balance after waive want %v, got %v", before[0].HoursBalance, after[0].HoursBalance)
	}
	if after[0].CompensationDayClaimsOpen != 0 {
		t.Fatalf("open compensation claims after waive want 0, got %d", after[0].CompensationDayClaimsOpen)
	}
}

func TestOverview_VacationPlannedAndFree(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	as := sqlite.NewAbsenceStore(db)
	ves := sqlite.NewVacationEntitlementStore(db)
	ss := sqlite.NewSettingsStore(db)

	u := &model.User{Username: "vac", PasswordHash: "x", DisplayName: "Vac User", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := ves.Set(ctx, &model.VacationEntitlement{UserID: u.ID, DaysPerYear: 25, ValidFrom: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}

	// today = 2026-06-15 local
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.Local)
	// taken: full 06-10, half 06-15; planned: full 06-16, half 06-20
	absences := []model.Absence{
		{UserID: u.ID, AbsenceDate: "2026-06-10", AbsenceType: model.AbsenceVacation, HalfDay: false, CreatedBy: u.ID},
		{UserID: u.ID, AbsenceDate: "2026-06-15", AbsenceType: model.AbsenceVacation, HalfDay: true, CreatedBy: u.ID},
		{UserID: u.ID, AbsenceDate: "2026-06-16", AbsenceType: model.AbsenceVacation, HalfDay: false, CreatedBy: u.ID},
		{UserID: u.ID, AbsenceDate: "2026-06-20", AbsenceType: model.AbsenceVacation, HalfDay: true, CreatedBy: u.ID},
	}
	for i := range absences {
		a := absences[i]
		if err := as.Create(ctx, &a); err != nil {
			t.Fatal(err)
		}
	}

	d := Deps{
		Users:       us,
		WorkPeriods: sqlite.NewWorkPeriodStore(db),
		Corrections: sqlite.NewCorrectionStore(db),
		Absences:    as,
		Holidays:    sqlite.NewHolidayStore(db),
		Closures:    sqlite.NewClosureDayStore(db),
		WeeklyHours: sqlite.NewWeeklyHoursStore(db),
		Settings:    ss,
		VacationEnt: ves,
	}

	// kein Stundensoll → Start 1.1. des Jahres von „gestern“; Urlaub unverändert
	rows, _, err := Build(ctx, d, 2026, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.HoursBalance != 0 {
		t.Fatalf("hours_balance want 0 (empty span), got %v", r.HoursBalance)
	}
	// Anspruch bis 31.12.2026: 12×25/12 = 25; taken 1.5; planned 1.5; remaining 23.5; free 22
	if r.VacationTaken != 1.5 {
		t.Fatalf("vacation_taken want 1.5, got %v", r.VacationTaken)
	}
	if r.VacationPlanned != 1.5 {
		t.Fatalf("vacation_planned want 1.5, got %v", r.VacationPlanned)
	}
	if r.VacationRemainingTotal != 23.5 {
		t.Fatalf("vacation_remaining_total want 23.5, got %v", r.VacationRemainingTotal)
	}
	if r.VacationFree != 22 {
		t.Fatalf("vacation_free want 22, got %v", r.VacationFree)
	}
	if r.VacationEntitlement != 25 {
		t.Fatalf("vacation_entitlement want 25, got %v", r.VacationEntitlement)
	}
	if r.VacationCarryover != 0 {
		t.Fatalf("vacation_carryover want 0, got %v", r.VacationCarryover)
	}
}

// Stunden-Startsaldo gilt nur, wenn das Nutzer-Erstellungsdatum im Auswertungsfenster liegt
// (frühestes Stundensoll .. gestern). Urlaubs-Startsaldo wird wie GET /me/vacation immer zum Rest gesamt addiert.
func TestOverview_OpeningBalancesApplyOnlyWhenOpeningDateIsInRange(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)
	ves := sqlite.NewVacationEntitlementStore(db)
	ss := sqlite.NewSettingsStore(db)

	uIn := &model.User{
		Username:            "opening_in",
		PasswordHash:        "x",
		DisplayName:         "Opening In",
		Role:                model.RoleUser,
		Active:              true,
		OpeningHoursBalance: 10,
		OpeningVacationDays: 5,
	}
	uOut := &model.User{
		Username:            "opening_out",
		PasswordHash:        "x",
		DisplayName:         "Opening Out",
		Role:                model.RoleUser,
		Active:              true,
		OpeningHoursBalance: 10,
		OpeningVacationDays: 5,
	}
	if err := us.Create(ctx, uIn); err != nil {
		t.Fatal(err)
	}
	if err := us.Create(ctx, uOut); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx, `
		UPDATE users SET opening_hours_balance = 10, opening_vacation_days = 5, created_at = '2026-01-01 00:00:00' WHERE id = ?`, uIn.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx, `
		UPDATE users SET opening_hours_balance = 10, opening_vacation_days = 5, created_at = '2026-01-01 00:00:00' WHERE id = ?`, uOut.ID); err != nil {
		t.Fatal(err)
	}
	for _, uid := range []int{uIn.ID, uOut.ID} {
		if err := ves.Set(ctx, &model.VacationEntitlement{UserID: uid, DaysPerYear: 20, ValidFrom: "2026-01-01"}); err != nil {
			t.Fatal(err)
		}
	}
	// Stundensoll ab 1.1. → Erstellungsdatum 1.1. liegt im Fenster → +10 h Startsaldo
	// Zweite Person: Stundensoll erst ab 1.2. → Erstellung vor Fensterbeginn → kein Stunden-Startsaldo (1.2.2026 ist Sonntag → kein Ziel an dem Tag)
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: uOut.ID, HoursPerWeek: 40, ValidFrom: "2026-02-01"}); err != nil {
		t.Fatal(err)
	}

	d := Deps{
		Users:       us,
		WorkPeriods: sqlite.NewWorkPeriodStore(db),
		Corrections: sqlite.NewCorrectionStore(db),
		Absences:    sqlite.NewAbsenceStore(db),
		Holidays:    sqlite.NewHolidayStore(db),
		Closures:    sqlite.NewClosureDayStore(db),
		WeeklyHours: whs,
		Settings:    ss,
		VacationEnt: ves,
	}
	now := time.Date(2026, 2, 2, 12, 0, 0, 0, time.Local)

	rows, _, err := Build(ctx, d, 2026, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	byName := map[string]float64{}
	for _, r := range rows {
		byName[r.DisplayName] = r.HoursBalance
	}
	if byName["Opening In"] != 10 {
		t.Fatalf("Opening In hours_balance: want 10, got %v", byName["Opening In"])
	}
	if byName["Opening Out"] != 0 {
		t.Fatalf("Opening Out hours_balance: want 0, got %v", byName["Opening Out"])
	}
	for _, r := range rows {
		if r.VacationCarryover != 0 || r.VacationRemainingTotal != 25 || r.VacationOpeningDays != 5 {
			t.Fatalf("vacation buckets for %q: want carryover 0, remaining 25, opening 5, got %+v", r.DisplayName, r)
		}
	}
}

func TestOverview_VacationCarryoverPrevYear(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	as := sqlite.NewAbsenceStore(db)
	ves := sqlite.NewVacationEntitlementStore(db)
	ss := sqlite.NewSettingsStore(db)

	u := &model.User{Username: "vacprev", PasswordHash: "x", DisplayName: "Vac Prev", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := ves.Set(ctx, &model.VacationEntitlement{UserID: u.ID, DaysPerYear: 25, ValidFrom: "2025-01-01"}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"2025-03-01", "2025-03-02", "2025-03-03", "2025-03-04", "2025-03-05", "2025-06-10", "2025-06-11", "2025-06-12", "2025-06-13", "2025-06-14"} {
		if err := as.Create(ctx, &model.Absence{UserID: u.ID, AbsenceDate: d, AbsenceType: model.AbsenceVacation, HalfDay: false, CreatedBy: u.ID}); err != nil {
			t.Fatal(err)
		}
	}

	d := Deps{
		Users:       us,
		WorkPeriods: sqlite.NewWorkPeriodStore(db),
		Corrections: sqlite.NewCorrectionStore(db),
		Absences:    as,
		Holidays:    sqlite.NewHolidayStore(db),
		Closures:    sqlite.NewClosureDayStore(db),
		WeeklyHours: sqlite.NewWeeklyHoursStore(db),
		Settings:    ss,
		VacationEnt: ves,
	}

	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.Local)
	rows, _, err := Build(ctx, d, 2026, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.VacationCarryover != 15 {
		t.Fatalf("vacation_carryover (Vorjahr 25−10): want 15, got %v", r.VacationCarryover)
	}
	if r.VacationEntitlement != 25 {
		t.Fatalf("vacation_entitlement (nur Jahr 2026): want 25, got %v", r.VacationEntitlement)
	}
	if r.VacationTaken != 0 {
		t.Fatalf("vacation_taken (nur 2026): want 0, got %v", r.VacationTaken)
	}
	if r.VacationRemainingTotal != 40 {
		t.Fatalf("vacation_remaining_total: want 40 (50−10), got %v", r.VacationRemainingTotal)
	}
}

func TestOverview_LatestCorrectionReplacesPunches(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ws := sqlite.NewWorkPeriodStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)
	cs := sqlite.NewCorrectionStore(db)
	ss := sqlite.NewSettingsStore(db)
	// Avoid default 9h/45m break tier so corrected 9h net − 8h target = 1
	if err := ss.Set(ctx, "break_rules", "[]"); err != nil {
		t.Fatal(err)
	}

	u := &model.User{Username: "corr", PasswordHash: "x", DisplayName: "Corr", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: u.ID, HoursPerWeek: 40, ValidFrom: "2026-03-10"}); err != nil {
		t.Fatal(err)
	}

	d1 := "2026-03-10"
	tIn := time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC)
	tOut8 := time.Date(2026, 3, 10, 16, 0, 0, 0, time.UTC)
	if err := ws.ReplaceForUserDate(ctx, u.ID, d1, []model.WorkPeriod{{PunchIn: tIn, PunchOut: &tOut8, IsBreak: false}}); err != nil {
		t.Fatal(err)
	}
	wps, err := ws.ListByUserDateRange(ctx, u.ID, d1, d1)
	if err != nil || len(wps) != 1 {
		t.Fatalf("wps: %v %+v", err, wps)
	}
	wpID := wps[0].ID
	tOut9 := time.Date(2026, 3, 10, 17, 0, 0, 0, time.UTC)
	if err := cs.Create(ctx, &model.TimeCorrection{WorkPeriodID: wpID, CorrectedIn: tIn, CorrectedOut: tOut9, Reason: "fix", CorrectedBy: u.ID}); err != nil {
		t.Fatal(err)
	}

	// now = 2026-03-11 → yesterday = 2026-03-10, single day in range [2026-03-10 .. 2026-03-10]
	now := time.Date(2026, 3, 11, 9, 0, 0, 0, time.Local)

	d := Deps{
		Users:       us,
		WorkPeriods: ws,
		Corrections: cs,
		Absences:    sqlite.NewAbsenceStore(db),
		Holidays:    sqlite.NewHolidayStore(db),
		Closures:    sqlite.NewClosureDayStore(db),
		WeeklyHours: whs,
		Settings:    ss,
		VacationEnt: sqlite.NewVacationEntitlementStore(db),
	}

	rows, _, err := Build(ctx, d, 2026, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	// 9h net − 8h target = +1 (no extra break deduction; break_rules cleared above)
	if rows[0].HoursBalance != 1 {
		t.Fatalf("hours_balance want 1 (corrected 9h), got %v", rows[0].HoursBalance)
	}
}

func TestOverview_ExcludesSuperadmin(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ss := sqlite.NewSettingsStore(db)

	reg := &model.User{Username: "u1", PasswordHash: "x", DisplayName: "Regular", Role: model.RoleUser, Active: true}
	sa := &model.User{Username: "sa", PasswordHash: "x", DisplayName: "Super", Role: model.RoleSuperadmin, Active: true}
	if err := us.Create(ctx, reg); err != nil {
		t.Fatal(err)
	}
	if err := us.Create(ctx, sa); err != nil {
		t.Fatal(err)
	}

	d := Deps{
		Users:       us,
		WorkPeriods: sqlite.NewWorkPeriodStore(db),
		Corrections: sqlite.NewCorrectionStore(db),
		Absences:    sqlite.NewAbsenceStore(db),
		Holidays:    sqlite.NewHolidayStore(db),
		Closures:    sqlite.NewClosureDayStore(db),
		WeeklyHours: sqlite.NewWeeklyHoursStore(db),
		Settings:    ss,
		VacationEnt: sqlite.NewVacationEntitlementStore(db),
	}

	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.Local)
	rows, _, err := Build(ctx, d, 2026, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != reg.ID {
		t.Fatalf("expected only regular user, got %+v", rows)
	}
}

func TestOverview_DefaultVacationYearFromNow(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ves := sqlite.NewVacationEntitlementStore(db)
	ss := sqlite.NewSettingsStore(db)

	u := &model.User{Username: "vy", PasswordHash: "x", DisplayName: "Y", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := ves.Set(ctx, &model.VacationEntitlement{UserID: u.ID, DaysPerYear: 20, ValidFrom: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}

	d := Deps{
		Users:       us,
		WorkPeriods: sqlite.NewWorkPeriodStore(db),
		Corrections: sqlite.NewCorrectionStore(db),
		Absences:    sqlite.NewAbsenceStore(db),
		Holidays:    sqlite.NewHolidayStore(db),
		Closures:    sqlite.NewClosureDayStore(db),
		WeeklyHours: sqlite.NewWeeklyHoursStore(db),
		Settings:    ss,
		VacationEnt: ves,
	}

	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.Local)
	rows, _, err := Build(ctx, d, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	// „now“ Juli 2026 → Anspruch Kalenderjahr 2026: volles Jahr = 20
	if len(rows) != 1 || rows[0].VacationEntitlement != 20 {
		t.Fatalf("expected entitlement 20 (calendar year 2026), got %+v", rows[0])
	}
}

func TestOverview_LoadsBreakRulesLikeExport(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	ws := sqlite.NewWorkPeriodStore(db)
	whs := sqlite.NewWeeklyHoursStore(db)
	ss := sqlite.NewSettingsStore(db)

	// 6h+ work → require 30 min break; gross 6h, stamped break 0 → deduction applies
	rules := []model.BreakRule{{MinWorkHours: 6, BreakMinutes: 30}}
	b, _ := json.Marshal(rules)
	if err := ss.Set(ctx, "break_rules", string(b)); err != nil {
		t.Fatal(err)
	}
	if err := ss.Set(ctx, "rounding_minutes", "15"); err != nil {
		t.Fatal(err)
	}

	u := &model.User{Username: "br", PasswordHash: "x", DisplayName: "BR", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: u.ID, HoursPerWeek: 40, ValidFrom: "2026-03-10"}); err != nil {
		t.Fatal(err)
	}

	d1 := "2026-03-10"
	tIn := time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC)
	tOut := time.Date(2026, 3, 10, 14, 0, 0, 0, time.UTC) // 6h gross
	if err := ws.ReplaceForUserDate(ctx, u.ID, d1, []model.WorkPeriod{{PunchIn: tIn, PunchOut: &tOut, IsBreak: false}}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 11, 9, 0, 0, 0, time.Local)

	d := Deps{
		Users:       us,
		WorkPeriods: ws,
		Corrections: sqlite.NewCorrectionStore(db),
		Absences:    sqlite.NewAbsenceStore(db),
		Holidays:    sqlite.NewHolidayStore(db),
		Closures:    sqlite.NewClosureDayStore(db),
		WeeklyHours: whs,
		Settings:    ss,
		VacationEnt: sqlite.NewVacationEntitlementStore(db),
	}

	rows, _, err := Build(ctx, d, 2026, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatal(len(rows))
	}
	// Net ≈ 5.5h after 30 min deduction, rounded down to 15 min → 5.5h; target 8 → balance -2.5
	hb := rows[0].HoursBalance
	if hb != -2.5 {
		t.Fatalf("hours_balance want -2.5, got %v", hb)
	}
}

func TestOverview_IncludesOpenCompensationDayClaims(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	claims := sqlite.NewCompensationDayClaimStore(db)
	u := &model.User{Username: "claimoverview", PasswordHash: "x", DisplayName: "Claim Overview", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := claims.EnsureForWorkDate(ctx, u.ID, "2026-04-04", true); err != nil {
		t.Fatal(err)
	}
	if err := claims.EnsureForWorkDate(ctx, u.ID, "2026-04-05", true); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.Local)
	rows, _, err := Build(ctx, Deps{
		Users:                 us,
		WorkPeriods:           sqlite.NewWorkPeriodStore(db),
		Corrections:           sqlite.NewCorrectionStore(db),
		Absences:              sqlite.NewAbsenceStore(db),
		Holidays:              sqlite.NewHolidayStore(db),
		Closures:              sqlite.NewClosureDayStore(db),
		WeeklyHours:           sqlite.NewWeeklyHoursStore(db),
		Settings:              sqlite.NewSettingsStore(db),
		VacationEnt:           sqlite.NewVacationEntitlementStore(db),
		CompensationDayClaims: claims,
	}, 2026, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].CompensationDayClaimsOpen != 2 {
		t.Fatalf("open compensation day claims want 2, got %d", rows[0].CompensationDayClaimsOpen)
	}
}

