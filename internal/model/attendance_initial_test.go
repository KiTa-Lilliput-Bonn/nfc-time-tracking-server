package model

import "testing"

func TestLastNameInitial(t *testing.T) {
	for in, want := range map[string]string{"Dorn": "D", " özdemir ": "Ö", "D.": "D", "": ""} {
		if got := LastNameInitial(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
