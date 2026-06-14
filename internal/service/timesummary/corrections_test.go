package timesummary

import (
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
)

func TestApplyLatestCorrections_DisabledExcludedAndCorrectedApplied(t *testing.T) {
	tIn := time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC)
	tOut := time.Date(2026, 3, 10, 16, 0, 0, 0, time.UTC)
	tOut9 := time.Date(2026, 3, 10, 17, 0, 0, 0, time.UTC)
	periods := []model.WorkPeriod{
		{ID: 1, PunchIn: tIn, PunchOut: &tOut, IsBreak: false},
		{ID: 2, PunchIn: tIn, PunchOut: &tOut, IsBreak: false},
	}
	corrs := []model.TimeCorrection{
		{ID: 1, WorkPeriodID: 1, CorrectedIn: tIn, CorrectedOut: tOut, Disabled: true},
		{ID: 2, WorkPeriodID: 2, CorrectedIn: tIn, CorrectedOut: tOut9, Disabled: false},
	}
	out := ApplyLatestCorrections(periods, corrs)
	if len(out) != 1 {
		t.Fatalf("want 1 period, got %d", len(out))
	}
	if out[0].ID != 2 {
		t.Fatalf("want period 2, got %d", out[0].ID)
	}
	if !out[0].PunchOut.Equal(tOut9) {
		t.Fatalf("want corrected out 17:00, got %v", out[0].PunchOut)
	}
}
