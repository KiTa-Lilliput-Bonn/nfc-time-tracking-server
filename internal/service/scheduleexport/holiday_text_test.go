package scheduleexport

import "testing"

func TestHolidayUseHorizontalException_onlyFewRowsAndLongName(t *testing.T) {
	if !holidayUseHorizontalException("Tag der Arbeit", 1) {
		t.Fatal("long name with one row should use horizontal exception")
	}
	if !holidayUseHorizontalException("Karfreitag", 2) {
		t.Fatal("Karfreitag with two rows should use horizontal exception")
	}
}

func TestHolidayUseHorizontalException_defaultVertical(t *testing.T) {
	if holidayUseHorizontalException("Karfreitag", 8) {
		t.Fatal("Karfreitag with enough rows should stay vertical")
	}
	if holidayUseHorizontalException("Ostern", 8) {
		t.Fatal("short name with enough rows should stay vertical")
	}
	if holidayUseHorizontalException("Ostern", 2) {
		t.Fatal("short name with few rows should still stay vertical")
	}
}
