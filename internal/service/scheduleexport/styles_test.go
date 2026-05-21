package scheduleexport

import "testing"

func TestUsesMutedShiftFill(t *testing.T) {
	for _, v := range []string{"U", "AT", "xxx", "Schule"} {
		if !usesMutedShiftFill(v) {
			t.Fatalf("%q should use muted fill", v)
		}
	}
	if usesMutedShiftFill("8.30-16.30") {
		t.Fatal("shift range should not use muted fill")
	}
}

func TestWeekFillColors_matchTemplate(t *testing.T) {
	want := [5]string{"00B0F0", "FFF2CC", "FDA5A5", "BDD7EE", "E2F0D9"}
	if weekFillColors != want {
		t.Fatalf("colors: %v", weekFillColors)
	}
}
