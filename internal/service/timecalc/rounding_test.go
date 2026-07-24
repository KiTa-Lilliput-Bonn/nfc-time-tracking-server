package timecalc

import (
	"testing"
	"time"
)

func TestRoundUpToMinute(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want time.Duration
	}{
		{"zero", 0, 0},
		{"negative", -time.Second, 0},
		{"exact minute", 5 * time.Minute, 5 * time.Minute},
		{"one second over", 5*time.Minute + time.Second, 6 * time.Minute},
		{"almost next", 5*time.Minute + 59*time.Second, 6 * time.Minute},
		{"exact hour", time.Hour, time.Hour},
		{"hour plus tick", time.Hour + time.Nanosecond, time.Hour + time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RoundUpToMinute(tt.in)
			if got != tt.want {
				t.Errorf("RoundUpToMinute(%v): got %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
