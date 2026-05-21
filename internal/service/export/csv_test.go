package export

import (
	"strings"
	"testing"
)

func TestMonthDateRange(t *testing.T) {
	from, to, err := MonthDateRange(2026, 3)
	if err != nil {
		t.Fatal(err)
	}
	if from != "2026-03-01" || to != "2026-03-31" {
		t.Fatalf("got %s %s", from, to)
	}
}

func TestMonthDateRange_invalid(t *testing.T) {
	_, _, err := MonthDateRange(2026, 13)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteCSV_smoke(t *testing.T) {
	var buf strings.Builder
	rows := []DayRow{{Date: "2026-03-01", Weekday: "So", NetHours: 1.5}}
	if err := WriteCSV(&buf, rows); err != nil {
		t.Fatal(err)
	}
	if buf.Len() < 20 {
		t.Fatal("expected output")
	}
}
