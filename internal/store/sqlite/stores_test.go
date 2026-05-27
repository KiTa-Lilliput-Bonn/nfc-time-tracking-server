package sqlite

import (
	"context"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
)

func TestPunchStore_InsertBatchAndDedup(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	ps := NewPunchStore(db)
	ns := NewNFCTagStore(db)

	u := &model.User{Username: "p1", PasswordHash: "x", DisplayName: "P1", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	tag := &model.NFCTag{TagUID: "TAG1", UserID: u.ID, AssignedFrom: "2026-01-01"}
	if err := ns.Assign(ctx, tag); err != nil {
		t.Fatal(err)
	}

	ts := time.Date(2026, 3, 26, 8, 0, 0, 0, time.UTC)
	punches := []model.RawPunch{
		{PunchTime: ts, NFCTagUID: "TAG1", SourceFile: "a.csv", DeviceName: "d1"},
	}
	n, err := ps.InsertBatch(ctx, punches)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 inserted, got %d", n)
	}
	n2, err := ps.InsertBatch(ctx, punches)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Fatalf("expected 0 on duplicate, got %d", n2)
	}

	list, err := ps.ListByUserAndDate(ctx, u.ID, "2026-03-26")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 punch for user/date, got %d", len(list))
	}
}

func TestPunchStore_ListByUTCDateForLanSync(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	ps := NewPunchStore(db)
	ns := NewNFCTagStore(db)

	u := &model.User{Username: "syn", PasswordHash: "x", DisplayName: "S", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	inactive := &model.User{Username: "off", PasswordHash: "x", DisplayName: "O", Role: model.RoleUser, Active: false}
	if err := us.Create(ctx, inactive); err != nil {
		t.Fatal(err)
	}
	if err := ns.Assign(ctx, &model.NFCTag{TagUID: "TAGA", UserID: u.ID, AssignedFrom: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	if err := ns.Assign(ctx, &model.NFCTag{TagUID: "TAGOFF", UserID: inactive.ID, AssignedFrom: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 4, 10, 15, 30, 0, 0, time.UTC)
	if _, err := ps.InsertBatch(ctx, []model.RawPunch{
		{PunchTime: ts, NFCTagUID: "TAGA", SourceFile: "lan_stamps", DeviceName: "x"},
		{PunchTime: ts, NFCTagUID: "TAGOFF", SourceFile: "lan_stamps", DeviceName: "x"},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := ps.ListByUTCDateForLanSync(ctx, "2026-04-10")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 sync row (active user only), got %d", len(got))
	}
	if got[0].UserID != u.ID || got[0].Punch.NFCTagUID != "TAGA" {
		t.Fatalf("unexpected %+v", got[0])
	}
}

func TestNFCTagStore_AssignAndResolve(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	ns := NewNFCTagStore(db)

	u := &model.User{Username: "nfc", PasswordHash: "x", DisplayName: "N", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	tag := &model.NFCTag{TagUID: "ABC", UserID: u.ID, AssignedFrom: "2026-06-01"}
	if err := ns.Assign(ctx, tag); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	uid, err := ns.ResolveUserID(ctx, "ABC", at)
	if err != nil {
		t.Fatal(err)
	}
	if uid != u.ID {
		t.Fatalf("expected user %d, got %d", u.ID, uid)
	}

	leitung := &model.User{Username: "boss", PasswordHash: "x", DisplayName: "B", Role: model.RoleLeitung, Active: true}
	if err := us.Create(ctx, leitung); err != nil {
		t.Fatal(err)
	}
	if err := ns.Assign(ctx, &model.NFCTag{TagUID: "X", UserID: leitung.ID, AssignedFrom: "2026-01-01"}); err != nil {
		t.Fatalf("assign NFC to leitung: %v", err)
	}
	uid, err = ns.ResolveUserID(ctx, "X", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || uid != leitung.ID {
		t.Fatalf("resolve leitung tag: uid=%d err=%v", uid, err)
	}

	admin := &model.User{Username: "root", PasswordHash: "x", DisplayName: "A", Role: model.RoleSuperadmin, Active: true}
	if err := us.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	err = ns.Assign(ctx, &model.NFCTag{TagUID: "Y", UserID: admin.ID, AssignedFrom: "2026-01-01"})
	if err == nil {
		t.Fatal("expected error assigning NFC to superadmin")
	}
}

func TestWorkPeriodStore_ReplaceForUserDate(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	ws := NewWorkPeriodStore(db)

	u := &model.User{Username: "wp", PasswordHash: "x", DisplayName: "W", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	t1 := time.Date(2026, 3, 26, 8, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	manual := &model.WorkPeriod{UserID: u.ID, WorkDate: "2026-03-26", PunchIn: t1, PunchOut: &t2, IsBreak: false}
	if err := ws.CreateManual(ctx, manual); err != nil {
		t.Fatal(err)
	}
	imported := []model.WorkPeriod{
		{PunchIn: time.Date(2026, 3, 26, 7, 0, 0, 0, time.UTC), PunchOut: ptrTime(time.Date(2026, 3, 26, 11, 0, 0, 0, time.UTC)), IsBreak: false},
	}
	if err := ws.ReplaceForUserDate(ctx, u.ID, "2026-03-26", imported); err != nil {
		t.Fatal(err)
	}
	all, err := ws.ListByUserDateRange(ctx, u.ID, "2026-03-26", "2026-03-26")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected manual + imported = 2 periods, got %d", len(all))
	}
	var manualCount, stampCount int
	for _, p := range all {
		if p.Source == "manual" {
			manualCount++
		}
		if p.Source == "" {
			stampCount++
		}
	}
	if manualCount != 1 || stampCount != 1 {
		t.Fatalf("expected 1 manual 1 stamp/default, got manual=%d stamp=%d", manualCount, stampCount)
	}
}

func TestWorkPeriodStore_GetByID(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	ws := NewWorkPeriodStore(db)

	u := &model.User{Username: "gwid", PasswordHash: "x", DisplayName: "G", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	tin := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	tout := time.Date(2026, 5, 1, 16, 0, 0, 0, time.UTC)
	wp := &model.WorkPeriod{UserID: u.ID, WorkDate: "2026-05-01", PunchIn: tin, PunchOut: &tout, IsBreak: false}
	if err := ws.CreateManual(ctx, wp); err != nil {
		t.Fatal(err)
	}
	got, err := ws.GetByID(ctx, wp.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v %+v", err, got)
	}
	if got.UserID != u.ID || got.WorkDate != "2026-05-01" || got.Source != "manual" {
		t.Fatalf("GetByID mismatch %+v", got)
	}
	miss, err := ws.GetByID(ctx, 999999)
	if err != nil || miss != nil {
		t.Fatalf("GetByID missing row want nil, got err=%v wp=%+v", err, miss)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestWorkPeriodStore_CreateManual_DisallowsOverlap(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	ws := NewWorkPeriodStore(db)

	u := &model.User{Username: "ov1", PasswordHash: "x", DisplayName: "O", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	t1 := time.Date(2026, 4, 2, 8, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	if err := ws.CreateManual(ctx, &model.WorkPeriod{UserID: u.ID, WorkDate: "2026-04-02", PunchIn: t1, PunchOut: ptrTime(t2), IsBreak: false}); err != nil {
		t.Fatal(err)
	}
	// Overlaps 11:00-13:00 with existing 08:00-12:00.
	o1 := time.Date(2026, 4, 2, 11, 0, 0, 0, time.UTC)
	o2 := time.Date(2026, 4, 2, 13, 0, 0, 0, time.UTC)
	if err := ws.CreateManual(ctx, &model.WorkPeriod{UserID: u.ID, WorkDate: "2026-04-02", PunchIn: o1, PunchOut: ptrTime(o2), IsBreak: false}); err == nil {
		t.Fatal("expected overlap error, got nil")
	}
}

func TestWorkPeriodStore_CreateManual_ConsidersLatestCorrection(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	ws := NewWorkPeriodStore(db)
	cs := NewCorrectionStore(db)

	u := &model.User{Username: "ov2", PasswordHash: "x", DisplayName: "O2", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	baseIn := time.Date(2026, 4, 3, 8, 0, 0, 0, time.UTC)
	baseOut := time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)
	wp := &model.WorkPeriod{UserID: u.ID, WorkDate: "2026-04-03", PunchIn: baseIn, PunchOut: ptrTime(baseOut), IsBreak: false}
	if err := ws.CreateManual(ctx, wp); err != nil {
		t.Fatal(err)
	}
	// Extend interval via correction to 08:00-11:00.
	cOut := time.Date(2026, 4, 3, 11, 0, 0, 0, time.UTC)
	if err := cs.Create(ctx, &model.TimeCorrection{
		WorkPeriodID: wp.ID,
		CorrectedIn:  baseIn,
		CorrectedOut: cOut,
		Reason:       "extend",
		CorrectedBy:  u.ID,
	}); err != nil {
		t.Fatal(err)
	}
	// New manual period overlaps corrected effective interval (10:30-12:00 overlaps until 11:00).
	nIn := time.Date(2026, 4, 3, 10, 30, 0, 0, time.UTC)
	nOut := time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)
	if err := ws.CreateManual(ctx, &model.WorkPeriod{UserID: u.ID, WorkDate: "2026-04-03", PunchIn: nIn, PunchOut: ptrTime(nOut), IsBreak: false}); err == nil {
		t.Fatal("expected overlap error due to correction, got nil")
	}
}

func TestScheduleStore_SetAndGet(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	ss := NewScheduleStore(db)

	u := &model.User{Username: "sch", PasswordHash: "x", DisplayName: "S", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	sch := &model.Schedule{UserID: u.ID, ScheduleDate: "2026-04-01", ShiftStart: "08:00", ShiftEnd: "16:30"}
	if err := ss.Set(ctx, sch); err != nil {
		t.Fatal(err)
	}
	if sch.ID == 0 {
		t.Fatal("expected schedule id set")
	}
	got, err := ss.GetForUserDate(ctx, u.ID, "2026-04-01")
	if err != nil {
		t.Fatal(err)
	}
	if got.ShiftStart != "08:00" || got.ShiftEnd != "16:30" {
		t.Fatalf("unexpected schedule %+v", got)
	}
}

func TestScheduleStore_ListByUserDateRange(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	ss := NewScheduleStore(db)

	u := &model.User{Username: "schr", PasswordHash: "x", DisplayName: "S", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	for _, day := range []string{"2026-04-01", "2026-04-03", "2026-04-05"} {
		sch := &model.Schedule{UserID: u.ID, ScheduleDate: day, ShiftStart: "09:00", ShiftEnd: "17:00"}
		if err := ss.Set(ctx, sch); err != nil {
			t.Fatal(err)
		}
	}
	list, err := ss.ListByUserDateRange(ctx, u.ID, "2026-04-02", "2026-04-04")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ScheduleDate != "2026-04-03" {
		t.Fatalf("expected one schedule on 2026-04-03, got %+v", list)
	}
}

// ListByWeek must include December days that belong to ISO week 1 of the following year (same as the Vue grid).
func TestScheduleStore_ListByWeek_ISOWeek1SpansDecember(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	ss := NewScheduleStore(db)

	u := &model.User{Username: "schw", PasswordHash: "x", DisplayName: "S", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	// Monday of ISO week 1 / 2026 is 2025-12-29 in local calendar.
	sch := &model.Schedule{UserID: u.ID, ScheduleDate: "2025-12-29", ShiftStart: "08:00", ShiftEnd: "16:00"}
	if err := ss.Set(ctx, sch); err != nil {
		t.Fatal(err)
	}
	list, err := ss.ListByWeek(ctx, 2026, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 schedule in 2026-W1, got %d (isoWeekRange must include late-December dates)", len(list))
	}
	if list[0].ScheduleDate != "2025-12-29" || list[0].ShiftStart != "08:00" {
		t.Fatalf("unexpected %+v", list[0])
	}
	mon, fri, err := ISOWeekMondayFriday(2026, 1)
	if err != nil {
		t.Fatal(err)
	}
	if mon != "2025-12-29" || fri != "2026-01-02" {
		t.Fatalf("ISOWeekMondayFriday 2026-W1: got %s .. %s", mon, fri)
	}
}

func TestScheduleStore_LastISOWeekWithShift(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	ss := NewScheduleStore(db)

	y, w, ok, err := ss.LastISOWeekWithShift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no shift")
	}
	if y != 0 || w != 0 {
		t.Fatalf("got %d/%d", y, w)
	}

	us := NewUserStore(db)
	u := &model.User{Username: "l", PasswordHash: "x", DisplayName: "L", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := ss.Set(ctx, &model.Schedule{
		UserID: u.ID, ScheduleDate: "2026-08-10",
		ShiftStart: "08:00", ShiftEnd: "16:00",
	}); err != nil {
		t.Fatal(err)
	}
	y, w, ok, err = ss.LastISOWeekWithShift(ctx)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if y != 2026 || w != 33 {
		t.Fatalf("got %d-W%d", y, w)
	}
}

func TestScheduleStore_WeekNotes(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	ss := NewScheduleStore(db)

	got, err := ss.GetWeekNotes(ctx, 2026, 12)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if err := ss.PutWeekNotes(ctx, 2026, 12, "Feierabend"); err != nil {
		t.Fatal(err)
	}
	got, err = ss.GetWeekNotes(ctx, 2026, 12)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Feierabend" {
		t.Fatalf("unexpected %q", got)
	}
	if err := ss.PutWeekNotes(ctx, 2026, 12, ""); err != nil {
		t.Fatal(err)
	}
	got, err = ss.GetWeekNotes(ctx, 2026, 12)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("expected empty after clear, got %q", got)
	}
}

func TestAbsenceStore_CRUD(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	as := NewAbsenceStore(db)

	u := &model.User{Username: "abs", PasswordHash: "x", DisplayName: "A", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	a := &model.Absence{UserID: u.ID, AbsenceDate: "2026-05-10", AbsenceType: model.AbsenceSick, HalfDay: false, CreatedBy: u.ID}
	if err := as.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	got, err := as.GetForUserDate(ctx, u.ID, "2026-05-10")
	if err != nil || got == nil {
		t.Fatalf("get: %v %+v", err, got)
	}
	if err := as.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	got2, _ := as.GetForUserDate(ctx, u.ID, "2026-05-10")
	if got2 != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestAbsenceStore_ListByDateRangeTypes(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	as := NewAbsenceStore(db)

	u := &model.User{Username: "abs2", PasswordHash: "x", DisplayName: "B", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	for _, x := range []struct {
		date string
		typ  model.AbsenceType
	}{
		{"2026-06-01", model.AbsenceVacation},
		{"2026-06-02", model.AbsenceSick},
		{"2026-06-03", model.AbsenceCompensationDay},
	} {
		a := &model.Absence{UserID: u.ID, AbsenceDate: x.date, AbsenceType: x.typ, HalfDay: false, CreatedBy: u.ID}
		if err := as.Create(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	list, err := as.ListByDateRangeTypes(ctx, "2026-06-01", "2026-06-03", []model.AbsenceType{
		model.AbsenceVacation, model.AbsenceCompensationDay,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 absences, got %d", len(list))
	}
}

func TestCompensationDayClaimStore_EnsureForWorkDate(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	cs := NewCompensationDayClaimStore(db)

	u := &model.User{Username: "claim", PasswordHash: "x", DisplayName: "Claim", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	if err := cs.EnsureForWorkDate(ctx, u.ID, "2026-04-04", true); err != nil {
		t.Fatal(err)
	}
	if err := cs.EnsureForWorkDate(ctx, u.ID, "2026-04-04", true); err != nil {
		t.Fatal(err)
	}
	list, err := cs.ListByUser(ctx, u.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Status != model.CompensationDayClaimOpen {
		t.Fatalf("expected one open claim, got %+v", list)
	}

	if err := cs.EnsureForWorkDate(ctx, u.ID, "2026-04-04", false); err != nil {
		t.Fatal(err)
	}
	list, err = cs.ListByUser(ctx, u.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected open claim to be removed, got %+v", list)
	}
}

func TestCompensationDayClaimStore_UseReopenAndWaive(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	as := NewAbsenceStore(db)
	cs := NewCompensationDayClaimStore(db)

	u := &model.User{Username: "claim2", PasswordHash: "x", DisplayName: "Claim 2", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := cs.EnsureForWorkDate(ctx, u.ID, "2026-04-04", true); err != nil {
		t.Fatal(err)
	}
	a := &model.Absence{UserID: u.ID, AbsenceDate: "2026-04-07", AbsenceType: model.AbsenceCompensationDay, HalfDay: false, CreatedBy: u.ID}
	if err := as.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	claim, err := cs.GetOldestOpen(ctx, u.ID)
	if err != nil || claim == nil {
		t.Fatalf("open claim: %v %+v", err, claim)
	}
	if err := cs.MarkUsed(ctx, claim.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	open, err := cs.CountOpen(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if open != 0 {
		t.Fatalf("open count want 0, got %d", open)
	}
	if err := cs.ReopenByAbsenceID(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	open, err = cs.CountOpen(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if open != 1 {
		t.Fatalf("open count after reopen want 1, got %d", open)
	}
	if err := cs.Waive(ctx, u.ID, claim.ID); err != nil {
		t.Fatal(err)
	}
	open, err = cs.CountOpen(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if open != 0 {
		t.Fatalf("open count after waive want 0, got %d", open)
	}
}

func TestHolidayStore_CRUD(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	hs := NewHolidayStore(db)

	h := &model.Holiday{HolidayDate: "2026-12-25", Name: "Test", Kind: model.HolidayKindFeiertag, AutoGenerated: false}
	if err := hs.Create(ctx, h); err != nil {
		t.Fatal(err)
	}
	got, err := hs.GetForDate(ctx, "2026-12-25")
	if err != nil || got == nil || got.Kind != model.HolidayKindFeiertag {
		t.Fatalf("GetForDate kind: %+v err %v", got, err)
	}
	b := &model.Holiday{HolidayDate: "2026-12-24", Name: "Heiligabend", Kind: model.HolidayKindBrauchtum, AutoGenerated: true}
	if err := hs.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	gotB, err := hs.GetForDate(ctx, "2026-12-24")
	if err != nil || gotB == nil || gotB.Kind != model.HolidayKindBrauchtum {
		t.Fatalf("brauchtum kind: %+v err %v", gotB, err)
	}
	list, err := hs.ListByYear(ctx, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 1 {
		t.Fatal("expected at least one holiday")
	}
	if err := hs.Delete(ctx, h.ID); err != nil {
		t.Fatal(err)
	}
}

func TestClosureDayStore_CRUD(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	cs := NewClosureDayStore(db)

	u := &model.User{Username: "cl", PasswordHash: "x", DisplayName: "C", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	c := &model.ClosureDay{ClosureDate: "2026-07-01", Name: "Betrieb", CreatedBy: u.ID}
	if err := cs.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	list, err := cs.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v len=%d", err, len(list))
	}
	if err := cs.Delete(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsStore_GetAndSet(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	ss := NewSettingsStore(db)

	v, err := ss.Get(ctx, "rounding_minutes")
	if err != nil || v != "15" {
		t.Fatalf("default rounding: %v %q", err, v)
	}
	if err := ss.Set(ctx, "rounding_minutes", "30"); err != nil {
		t.Fatal(err)
	}
	v2, err := ss.Get(ctx, "rounding_minutes")
	if err != nil || v2 != "30" {
		t.Fatalf("after set: %v %q", err, v2)
	}
}

func TestWeeklyHoursStore_SetAndGetForDate(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	whs := NewWeeklyHoursStore(db)

	u := &model.User{Username: "wh", PasswordHash: "x", DisplayName: "W", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: u.ID, HoursPerWeek: 40, ValidFrom: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: u.ID, HoursPerWeek: 30, ValidFrom: "2026-03-01"}); err != nil {
		t.Fatal(err)
	}
	gotFeb, err := whs.GetForDate(ctx, u.ID, "2026-02-15")
	if err != nil {
		t.Fatal(err)
	}
	if gotFeb == nil || gotFeb.HoursPerWeek != 40 {
		t.Fatalf("expected 40h in February, got %+v", gotFeb)
	}
	got, err := whs.GetForDate(ctx, u.ID, "2026-03-15")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.HoursPerWeek != 30 {
		t.Fatalf("expected 30h from March, got %+v", got)
	}
}

func TestFixedNonWorkWeekdaysStore_SetAndGetForDate(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	fnw := NewFixedNonWorkWeekdaysStore(db)

	u := &model.User{Username: "fnw", PasswordHash: "x", DisplayName: "F", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := fnw.Set(ctx, &model.FixedNonWorkWeekdays{UserID: u.ID, Weekdays: []int{5}, ValidFrom: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	if err := fnw.Set(ctx, &model.FixedNonWorkWeekdays{UserID: u.ID, Weekdays: []int{4, 5}, ValidFrom: "2026-03-01"}); err != nil {
		t.Fatal(err)
	}
	gotFeb, err := fnw.GetForDate(ctx, u.ID, "2026-02-15")
	if err != nil {
		t.Fatal(err)
	}
	if gotFeb == nil || len(gotFeb.Weekdays) != 1 || gotFeb.Weekdays[0] != 5 {
		t.Fatalf("expected Friday only in February, got %+v", gotFeb)
	}
	got, err := fnw.GetForDate(ctx, u.ID, "2026-03-15")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Weekdays) != 2 {
		t.Fatalf("expected Thu+Fri from March, got %+v", got)
	}
}

func TestWeeklyHoursStore_Delete(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	whs := NewWeeklyHoursStore(db)

	u := &model.User{Username: "whdel", PasswordHash: "x", DisplayName: "WD", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	wh := &model.WeeklyHours{UserID: u.ID, HoursPerWeek: 40, ValidFrom: "2026-01-01"}
	if err := whs.Set(ctx, wh); err != nil {
		t.Fatal(err)
	}
	if wh.ID == 0 {
		t.Fatal("expected id after Set")
	}
	if err := whs.Delete(ctx, u.ID+999, wh.ID); err == nil {
		t.Fatal("expected error deleting wrong user")
	}
	if err := whs.Delete(ctx, u.ID, wh.ID); err != nil {
		t.Fatal(err)
	}
	list, err := whs.ListByUser(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %+v", list)
	}
}

func TestVacationEntitlementStore_SetAndGetForDate(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	ves := NewVacationEntitlementStore(db)

	u := &model.User{Username: "ve", PasswordHash: "x", DisplayName: "V", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := ves.Set(ctx, &model.VacationEntitlement{UserID: u.ID, DaysPerYear: 28, ValidFrom: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	got, err := ves.GetForDate(ctx, u.ID, "2026-06-01")
	if err != nil || got == nil || got.DaysPerYear != 28 {
		t.Fatalf("get: %v %+v", err, got)
	}
	if err := ves.Delete(ctx, u.ID, got.ID); err != nil {
		t.Fatal(err)
	}
	list, err := ves.ListByUser(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %+v", list)
	}
}

func TestCorrectionStore_CreateAndGetLatest(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	ws := NewWorkPeriodStore(db)
	cs := NewCorrectionStore(db)

	u := &model.User{Username: "corr", PasswordHash: "x", DisplayName: "C", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	tin := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	tout := time.Date(2026, 4, 1, 16, 0, 0, 0, time.UTC)
	if err := ws.CreateManual(ctx, &model.WorkPeriod{UserID: u.ID, WorkDate: "2026-04-01", PunchIn: tin, PunchOut: &tout, IsBreak: false}); err != nil {
		t.Fatal(err)
	}
	wps, _ := ws.ListByUserDateRange(ctx, u.ID, "2026-04-01", "2026-04-01")
	if len(wps) != 1 {
		t.Fatalf("expected 1 wp, got %d", len(wps))
	}
	wpID := wps[0].ID
	c1 := &model.TimeCorrection{WorkPeriodID: wpID, CorrectedIn: tin, CorrectedOut: tout, Reason: "first", CorrectedBy: u.ID}
	if err := cs.Create(ctx, c1); err != nil {
		t.Fatal(err)
	}
	c2 := &model.TimeCorrection{WorkPeriodID: wpID, CorrectedIn: tin, CorrectedOut: time.Date(2026, 4, 1, 17, 0, 0, 0, time.UTC), Reason: "second", CorrectedBy: u.ID}
	if err := cs.Create(ctx, c2); err != nil {
		t.Fatal(err)
	}
	latest, err := cs.GetLatestForPeriod(ctx, wpID)
	if err != nil || latest == nil {
		t.Fatalf("latest: %v %+v", err, latest)
	}
	if latest.Reason != "second" {
		t.Fatalf("expected latest second correction, got %q", latest.Reason)
	}
}
