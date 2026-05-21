package scheduleexport_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/scheduleexport"
	"nfc-time-tracking-server/internal/service/scheduleimport"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestBuildXLSX_holidayColumn(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	u := &model.User{
		Username: "h", PasswordHash: "x", DisplayName: "Anna Tester",
		Role: model.RoleUser, Active: true,
	}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	mon := time.Date(2026, 3, 30, 12, 0, 0, 0, time.Local)
	fri := mon.AddDate(0, 0, 4) // 03.04.2026 Karfreitag
	isoY, isoW := mon.ISOWeek()

	hs := sqlite.NewHolidayStore(db)
	if err := hs.Create(ctx, &model.Holiday{
		HolidayDate: fri.Format("2006-01-02"),
		Name:        "Karfreitag",
	}); err != nil {
		t.Fatal(err)
	}

	ss := sqlite.NewScheduleStore(db)
	if err := ss.Set(ctx, &model.Schedule{
		UserID: u.ID, ScheduleDate: mon.Format("2006-01-02"),
		ShiftStart: "08:30", ShiftEnd: "16:30",
	}); err != nil {
		t.Fatal(err)
	}

	buf, err := scheduleexport.BuildXLSX(ctx, scheduleexport.Deps{
		Users: us, Groups: sqlite.NewGroupStore(db),
		Schedules: ss, Absences: sqlite.NewAbsenceStore(db), Holidays: hs,
	}, isoY, isoW, isoY, isoW)
	if err != nil {
		t.Fatal(err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(buf))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)

	datumRow := 0
	for r := 1; r <= 30; r++ {
		b, _ := f.GetCellValue(sheet, cellRef(2, r))
		if b == "Datum" {
			datumRow = r
			break
		}
	}
	if datumRow == 0 {
		t.Fatal("Datum row not found")
	}
	friHeader, _ := f.GetCellValue(sheet, cellRef(7, datumRow))
	if friHeader != "03.04." {
		t.Fatalf("Friday header want 03.04., got %q", friHeader)
	}

	holidayCell := cellRef(7, datumRow+1)
	holidayVal, _ := f.GetCellValue(sheet, holidayCell)
	if holidayVal != "Karfreitag" {
		t.Fatalf("Friday column want Karfreitag, got %q at %s", holidayVal, holidayCell)
	}

	merges, _ := f.GetMergeCells(sheet)
	for _, m := range merges {
		start, end := m.GetStartAxis(), m.GetEndAxis()
		if start[0] == 'G' && end[0] == 'G' {
			v, _ := f.GetCellValue(sheet, start)
			if v == "Karfreitag" && start != end {
				holidayCell = start
				break
			}
		}
	}

	sid, _ := f.GetCellStyle(sheet, holidayCell)
	st, _ := f.GetStyle(sid)
	if st == nil || st.Alignment == nil {
		t.Fatalf("missing holiday cell style")
	}
	// Ein Mitarbeiter: Ausnahme waagerecht; sonst senkrecht (90°).
	if st.Alignment.TextRotation != 90 && st.Alignment.TextRotation != 0 {
		t.Fatalf("unexpected text rotation %d", st.Alignment.TextRotation)
	}

	parsed, err := scheduleimport.ParseXLSX(buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Weeks) != 1 {
		t.Fatalf("weeks: %d", len(parsed.Weeks))
	}
	w := parsed.Weeks[0]
	if !w.SkipDay[4] {
		t.Fatal("Friday should be skip day")
	}
}

func cellRef(col, row int) string {
	c, _ := excelize.CoordinatesToCellName(col, row)
	return c
}
