package scheduleimport

import "testing"

func TestParseCellContent_TimeStripsSuffix(t *testing.T) {
	pc, meta := ParseCellContent("8.30-16.30 B")
	if pc.Kind != CellWorkTimes {
		t.Fatalf("Kind: %v", pc.Kind)
	}
	if pc.ShiftStart != "08:30" || pc.ShiftEnd != "16:30" {
		t.Fatalf("times: %s %s", pc.ShiftStart, pc.ShiftEnd)
	}
	if meta.DotInsteadOfHyphenBetweenTimes {
		t.Fatal("expected hyphen form, no dot typo meta")
	}
}

func TestParseCellContent_DotInsteadOfHyphenBetweenTimes(t *testing.T) {
	pc, meta := ParseCellContent("9:00.17:00")
	if pc.Kind != CellWorkTimes {
		t.Fatalf("Kind: %v", pc.Kind)
	}
	if pc.ShiftStart != "09:00" || pc.ShiftEnd != "17:00" {
		t.Fatalf("times: %s %s", pc.ShiftStart, pc.ShiftEnd)
	}
	if !meta.DotInsteadOfHyphenBetweenTimes {
		t.Fatal("expected DotInsteadOfHyphenBetweenTimes")
	}

	pc2, meta2 := ParseCellContent("8.30.16.30")
	if pc2.Kind != CellWorkTimes || meta2.DotInsteadOfHyphenBetweenTimes != true {
		t.Fatalf("dot form with . as minute sep: %+v meta=%+v", pc2, meta2)
	}
	if pc2.ShiftStart != "08:30" || pc2.ShiftEnd != "16:30" {
		t.Fatalf("times: %s %s", pc2.ShiftStart, pc2.ShiftEnd)
	}
}

func TestParseCellContent_Absences(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want CellKind
	}{
		{"U", CellVacation},
		{"AT", CellCompensationDay},
		{"xxx", CellFreeDay},
		{"Schule", CellOtherAbsence},
		{"seminar", CellOtherAbsence},
		{"", CellEmpty},
	} {
		pc, _ := ParseCellContent(tc.raw)
		if pc.Kind != tc.want {
			t.Fatalf("%q: got %v want %v", tc.raw, pc.Kind, tc.want)
		}
	}
}
