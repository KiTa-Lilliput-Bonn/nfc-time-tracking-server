package timecalc

import (
	"testing"
	"time"
)

func TestRoundDown(t *testing.T) {
	tests := []struct {
		minutes  float64
		unit     int
		expected float64
	}{
		{443, 15, 435},
		{60, 15, 60},
		{29, 15, 15},
		{14, 15, 0},
		{480, 5, 480},
		{483, 5, 480},
	}
	for _, tt := range tests {
		got := RoundDown(time.Duration(tt.minutes)*time.Minute, tt.unit)
		expected := time.Duration(tt.expected) * time.Minute
		if got != expected {
			t.Errorf("RoundDown(%v, %d): expected %v, got %v", tt.minutes, tt.unit, expected, got)
		}
	}
}
