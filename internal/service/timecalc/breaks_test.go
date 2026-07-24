package timecalc

import (
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
)

func defaultRules() []model.BreakRule {
	return []model.BreakRule{
		{MinWorkHours: 6.0, BreakMinutes: 30},
		{MinWorkHours: 9.0, BreakMinutes: 45},
	}
}

func TestCalcBreakDeduction_Under6h(t *testing.T) {
	deduction := CalcBreakDeduction(5*time.Hour+59*time.Minute, 0, defaultRules())
	if deduction != 0 {
		t.Errorf("expected 0 deduction for 5:59, got %v", deduction)
	}
}

func TestCalcBreakDeduction_ProgressiveOver6h(t *testing.T) {
	tests := []struct {
		name string
		gross time.Duration
		want  time.Duration
	}{
		{"6:10", 6*time.Hour + 10*time.Minute, 10 * time.Minute},
		{"6:30", 6*time.Hour + 30*time.Minute, 30 * time.Minute},
		{"6:35", 6*time.Hour + 35*time.Minute, 30 * time.Minute},
		{"7:00", 7 * time.Hour, 30 * time.Minute},
		{"exactly 6h", 6 * time.Hour, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalcBreakDeduction(tt.gross, 0, defaultRules())
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalcBreakDeduction_ProgressiveOver9h(t *testing.T) {
	tests := []struct {
		name  string
		gross time.Duration
		want  time.Duration
	}{
		{"exactly 9h", 9 * time.Hour, 30 * time.Minute},
		{"9:10", 9*time.Hour + 10*time.Minute, 40 * time.Minute},
		{"9:15", 9*time.Hour + 15*time.Minute, 45 * time.Minute},
		{"9:20", 9*time.Hour + 20*time.Minute, 45 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalcBreakDeduction(tt.gross, 0, defaultRules())
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalcBreakDeduction_ShortStampedBreak(t *testing.T) {
	gross := 7 * time.Hour
	stamped := 20 * time.Minute
	deduction := CalcBreakDeduction(gross, stamped, defaultRules())
	if deduction != 10*time.Minute {
		t.Errorf("expected 10m deduction, got %v", deduction)
	}
}

func TestCalcBreakDeduction_SufficientStampedBreak(t *testing.T) {
	gross := 7 * time.Hour
	stamped := 35 * time.Minute
	deduction := CalcBreakDeduction(gross, stamped, defaultRules())
	if deduction != 0 {
		t.Errorf("expected 0 deduction, got %v", deduction)
	}
}

func TestCalcBreakDeduction_ProgressiveWithPartialStamped(t *testing.T) {
	// 6:10 requires 10; 5 stamped → deduct 5
	got := CalcBreakDeduction(6*time.Hour+10*time.Minute, 5*time.Minute, defaultRules())
	if got != 5*time.Minute {
		t.Errorf("got %v, want 5m", got)
	}
}
