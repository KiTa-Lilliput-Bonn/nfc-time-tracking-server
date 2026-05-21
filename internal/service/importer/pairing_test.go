package importer

import (
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
)

func TestPairPunches_FourStamps_TwoWorkBlocks(t *testing.T) {
	day := "2026-03-26"
	punches := []model.RawPunch{
		{PunchTime: time.Date(2026, 3, 26, 7, 55, 0, 0, time.Local)},
		{PunchTime: time.Date(2026, 3, 26, 12, 0, 0, 0, time.Local)},
		{PunchTime: time.Date(2026, 3, 26, 12, 30, 0, 0, time.Local)},
		{PunchTime: time.Date(2026, 3, 26, 16, 5, 0, 0, time.Local)},
	}

	periods := PairPunches(1, day, punches)
	if len(periods) != 2 {
		t.Fatalf("expected 2 work periods for 4 punches, got %d", len(periods))
	}
	if periods[0].IsBreak || periods[0].PunchOut == nil {
		t.Errorf("period0: closed work, got break=%v outNil=%v", periods[0].IsBreak, periods[0].PunchOut == nil)
	}
	if periods[0].Source != "" {
		t.Errorf("period0: want empty source (Standard-Stempel), got %q", periods[0].Source)
	}
	if periods[1].IsBreak || periods[1].PunchOut == nil {
		t.Errorf("period1: closed work, got break=%v outNil=%v", periods[1].IsBreak, periods[1].PunchOut == nil)
	}
}

func TestPairPunches_ThreeStamps_ClosedThenOpen(t *testing.T) {
	day := "2026-03-26"
	punches := []model.RawPunch{
		{PunchTime: time.Date(2026, 3, 26, 8, 0, 0, 0, time.Local)},
		{PunchTime: time.Date(2026, 3, 26, 12, 0, 0, 0, time.Local)},
		{PunchTime: time.Date(2026, 3, 26, 12, 30, 0, 0, time.Local)},
	}

	periods := PairPunches(1, day, punches)
	if len(periods) != 2 {
		t.Fatalf("expected 2 periods (1 closed + 1 open), got %d", len(periods))
	}
	if periods[0].PunchOut == nil || periods[0].IsBreak {
		t.Error("first period should be closed work")
	}
	if periods[1].PunchOut != nil {
		t.Error("last period should be open (nil punch_out)")
	}
	if periods[1].IsBreak {
		t.Error("open tail should be work, not break")
	}
}

func TestPairPunches_SinglePunch(t *testing.T) {
	punches := []model.RawPunch{
		{PunchTime: time.Date(2026, 3, 26, 8, 0, 0, 0, time.Local)},
	}
	periods := PairPunches(1, "2026-03-26", punches)
	if len(periods) != 1 {
		t.Fatalf("expected 1 open period, got %d", len(periods))
	}
	if periods[0].PunchOut != nil {
		t.Error("single punch should have nil punch_out")
	}
	if periods[0].IsBreak {
		t.Error("single open period should be work")
	}
}
