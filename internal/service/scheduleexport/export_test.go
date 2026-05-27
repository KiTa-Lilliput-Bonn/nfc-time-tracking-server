package scheduleexport_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/scheduleexport"
	"nfc-time-tracking-server/internal/service/scheduleimport"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestBuildXLSX_roundTrip(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	fnw := sqlite.NewFixedNonWorkWeekdaysStore(db)
	u := &model.User{
		Username: "exp", PasswordHash: "x", DisplayName: "Anna Tester",
		Role: model.RoleUser, Active: true,
	}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := fnw.Set(ctx, &model.FixedNonWorkWeekdays{
		UserID: u.ID, Weekdays: []int{5}, ValidFrom: "2000-01-01",
	}); err != nil {
		t.Fatal(err)
	}

	mon := time.Date(2026, 3, 16, 12, 0, 0, 0, time.Local)
	isoY, isoW := mon.ISOWeek()

	ss := sqlite.NewScheduleStore(db)
	if err := ss.Set(ctx, &model.Schedule{
		UserID: u.ID, ScheduleDate: mon.Format("2006-01-02"),
		ShiftStart: "08:30", ShiftEnd: "16:30",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ss.PutWeekNotes(ctx, isoY, isoW, "<p><strong>Test</strong></p>"); err != nil {
		t.Fatal(err)
	}

	as := sqlite.NewAbsenceStore(db)
	wed := mon.AddDate(0, 0, 2)
	if err := as.Create(ctx, &model.Absence{
		UserID: u.ID, AbsenceDate: wed.Format("2006-01-02"),
		AbsenceType: model.AbsenceVacation, CreatedBy: u.ID,
	}); err != nil {
		t.Fatal(err)
	}
	thu := mon.AddDate(0, 0, 3)
	if err := as.Create(ctx, &model.Absence{
		UserID: u.ID, AbsenceDate: thu.Format("2006-01-02"),
		AbsenceType: model.AbsenceSick, CreatedBy: u.ID,
	}); err != nil {
		t.Fatal(err)
	}

	buf, err := scheduleexport.BuildXLSX(ctx, scheduleexport.Deps{
		Users: us, Groups: sqlite.NewGroupStore(db),
		Schedules: ss, Absences: as, Holidays: sqlite.NewHolidayStore(db),
		FixedNonWorkWeekdays: fnw,
	}, isoY, isoW, isoY, isoW)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := scheduleimport.ParseXLSX(buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Weeks) != 1 {
		t.Fatalf("weeks: %d", len(parsed.Weeks))
	}
	w := parsed.Weeks[0]
	if w.ISOYear != isoY || w.ISOWk != isoW {
		t.Fatalf("iso: %d/%d", w.ISOYear, w.ISOWk)
	}
	if len(w.Rows) < 1 {
		t.Fatal("no employee rows")
	}
	row := w.Rows[0]
	if row.Cells[0] != "8.30-16.30" {
		t.Fatalf("Mo: %q", row.Cells[0])
	}
	if row.Cells[1] != "xxx" {
		t.Fatalf("Di ohne Planung: %q", row.Cells[1])
	}
	if row.Cells[2] != "U" {
		t.Fatalf("Mi Urlaub: %q", row.Cells[2])
	}
	if row.Cells[3] != "xxx" {
		t.Fatalf("Do Krank: %q", row.Cells[3])
	}
	if row.Cells[4] != "xxx" {
		t.Fatalf("Fr fixed free: %q", row.Cells[4])
	}
	if !strings.Contains(w.Notes, "Test") {
		t.Fatalf("notes: %q", w.Notes)
	}
}

func TestBuildXLSX_otherMeetingInNotes(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	u := &model.User{
		Username: "exp2", PasswordHash: "x", DisplayName: "Bob",
		Role: model.RoleUser, Active: true,
	}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	mon := time.Date(2026, 3, 16, 12, 0, 0, 0, time.Local)
	isoY, isoW := mon.ISOWeek()
	wed := mon.AddDate(0, 0, 2).Format("2006-01-02")

	ss := sqlite.NewScheduleStore(db)
	tms := sqlite.NewTeamMeetingStore(db)
	if err := tms.CreateWithUsers(ctx, &model.TeamMeeting{
		ISOWeekYear: isoY, ISOWeek: isoW, MeetingDate: wed,
		Kind: model.TeamMeetingKindOther, Label: "Fortbildung",
		TimeStart: "09:00", TimeEnd: "10:00", Source: "manual", SectionIndex: 0,
		UserIDs: []int{u.ID},
	}); err != nil {
		t.Fatal(err)
	}

	buf, err := scheduleexport.BuildXLSX(ctx, scheduleexport.Deps{
		Users: us, Groups: sqlite.NewGroupStore(db),
		Schedules: ss, Absences: sqlite.NewAbsenceStore(db),
		Holidays: sqlite.NewHolidayStore(db), TeamMeetings: tms,
	}, isoY, isoW, isoY, isoW)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := scheduleimport.ParseXLSX(buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Weeks) != 1 {
		t.Fatalf("weeks: %d", len(parsed.Weeks))
	}
	notes := parsed.Weeks[0].Notes
	if !strings.Contains(notes, "Fortbildung") {
		t.Fatalf("notes missing label: %q", notes)
	}
	if !strings.Contains(notes, "Mi:") {
		t.Fatalf("notes missing weekday: %q", notes)
	}
}

func TestExportDefaults(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	ss := sqlite.NewScheduleStore(db)
	us := sqlite.NewUserStore(db)
	u := &model.User{Username: "d", PasswordHash: "x", DisplayName: "D", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := ss.Set(ctx, &model.Schedule{
		UserID: u.ID, ScheduleDate: "2026-06-15",
		ShiftStart: "09:00", ShiftEnd: "17:00",
	}); err != nil {
		t.Fatal(err)
	}

	_, _, endY, endW, err := scheduleexport.ExportDefaults(ctx, ss)
	if err != nil {
		t.Fatal(err)
	}
	// 2026-06-15 is ISO week 25 / 2026
	if endY != 2026 || endW != 25 {
		t.Fatalf("end default: %d-W%d", endY, endW)
	}
}

