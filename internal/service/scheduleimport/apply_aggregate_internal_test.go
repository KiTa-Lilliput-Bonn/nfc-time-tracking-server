package scheduleimport

import "testing"

func TestAggregateSkippedHolidayCells_duplicateWeekHalvesTotal(t *testing.T) {
	perWeek := map[isoWeekKey]int{
		{year: 2026, week: 17}: 100,
	}
	occ := map[isoWeekKey]int{
		{year: 2026, week: 17}: 2,
	}
	rep := &Report{}
	got := aggregateSkippedHolidayCells(perWeek, occ, rep)
	if got != 50 {
		t.Fatalf("got %d want 50", got)
	}
	if len(rep.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(rep.Warnings), rep.Warnings)
	}
}

func TestAggregateSkippedHolidayCells_singleWeekUnchanged(t *testing.T) {
	perWeek := map[isoWeekKey]int{
		{year: 2026, week: 17}: 12,
	}
	occ := map[isoWeekKey]int{
		{year: 2026, week: 17}: 1,
	}
	rep := &Report{}
	got := aggregateSkippedHolidayCells(perWeek, occ, rep)
	if got != 12 {
		t.Fatalf("got %d want 12", got)
	}
	if len(rep.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", rep.Warnings)
	}
}
