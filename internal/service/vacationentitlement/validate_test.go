package vacationentitlement

import "testing"

func TestParseValidFromCalendarDay_ok(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2026-04-01", "2026-04-01"},
		{"2026-04-15", "2026-04-15"},
		{"2026-04-01 00:00:00", "2026-04-01"},
		{" 2026-01-01 ", "2026-01-01"},
	}
	for _, tc := range cases {
		got, err := ParseValidFromCalendarDay(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseValidFromCalendarDay_rejectsInvalid(t *testing.T) {
	_, err := ParseValidFromCalendarDay("not-a-date")
	if err == nil {
		t.Fatal("expected error")
	}
}
