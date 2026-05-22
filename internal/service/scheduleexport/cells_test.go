package scheduleexport

import (
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
)

func TestFormatExcelClock(t *testing.T) {
	if got := formatExcelClock("08:30"); got != "8.30" {
		t.Fatalf("got %q", got)
	}
	if got := formatExcelShiftRange("08:30", "16:30"); got != "8.30-16.30" {
		t.Fatalf("got %q", got)
	}
}

func TestCellValue_unplannedDay(t *testing.T) {
	u := &model.User{ID: 1}
	date := time.Date(2026, 3, 17, 0, 0, 0, 0, time.UTC) // Tuesday
	data := &weekCellData{
		schedules: map[int]map[string]*model.Schedule{1: {}},
		absences:  map[int]map[string]*model.Absence{1: {}},
	}
	if got := data.cellValue(u, 1, date); got != "xxx" {
		t.Fatalf("got %q, want xxx", got)
	}
}

func TestCellValue_halfDayVacationStaysEmpty(t *testing.T) {
	u := &model.User{ID: 1}
	date := time.Date(2026, 3, 17, 0, 0, 0, 0, time.UTC)
	iso := date.Format("2006-01-02")
	data := &weekCellData{
		schedules: map[int]map[string]*model.Schedule{1: {}},
		absences: map[int]map[string]*model.Absence{
			1: {iso: {AbsenceType: model.AbsenceVacation, HalfDay: true}},
		},
	}
	if got := data.cellValue(u, 1, date); got != "" {
		t.Fatalf("got %q, want empty for half-day vacation", got)
	}
}
