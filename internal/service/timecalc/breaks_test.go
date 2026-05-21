package timecalc

import (
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
)

func TestCalcBreakDeduction_NoStampedBreak_Over6h(t *testing.T) {
	rules := []model.BreakRule{
		{MinWorkHours: 6.0, BreakMinutes: 30},
		{MinWorkHours: 9.0, BreakMinutes: 45},
	}
	gross := 7 * time.Hour
	stamped := time.Duration(0)
	deduction := CalcBreakDeduction(gross, stamped, rules)
	if deduction != 30*time.Minute {
		t.Errorf("expected 30m deduction, got %v", deduction)
	}
}

func TestCalcBreakDeduction_ShortStampedBreak(t *testing.T) {
	rules := []model.BreakRule{
		{MinWorkHours: 6.0, BreakMinutes: 30},
	}
	gross := 7 * time.Hour
	stamped := 20 * time.Minute
	deduction := CalcBreakDeduction(gross, stamped, rules)
	if deduction != 10*time.Minute {
		t.Errorf("expected 10m deduction, got %v", deduction)
	}
}

func TestCalcBreakDeduction_SufficientStampedBreak(t *testing.T) {
	rules := []model.BreakRule{
		{MinWorkHours: 6.0, BreakMinutes: 30},
	}
	gross := 7 * time.Hour
	stamped := 35 * time.Minute
	deduction := CalcBreakDeduction(gross, stamped, rules)
	if deduction != 0 {
		t.Errorf("expected 0 deduction, got %v", deduction)
	}
}

func TestCalcBreakDeduction_Under6h(t *testing.T) {
	rules := []model.BreakRule{
		{MinWorkHours: 6.0, BreakMinutes: 30},
	}
	gross := 5 * time.Hour
	stamped := time.Duration(0)
	deduction := CalcBreakDeduction(gross, stamped, rules)
	if deduction != 0 {
		t.Errorf("expected 0 deduction for <6h, got %v", deduction)
	}
}
