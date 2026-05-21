package scheduleexport

import "testing"

func TestFormatExcelClock(t *testing.T) {
	if got := formatExcelClock("08:30"); got != "8.30" {
		t.Fatalf("got %q", got)
	}
	if got := formatExcelShiftRange("08:30", "16:30"); got != "8.30-16.30" {
		t.Fatalf("got %q", got)
	}
}
