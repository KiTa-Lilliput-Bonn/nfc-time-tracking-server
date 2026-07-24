package timecalc

import "time"

// RoundUpToMinute rounds duration up to the next whole minute.
// Exact multiples of a minute are unchanged; zero or negative durations become 0.
func RoundUpToMinute(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	rem := d % time.Minute
	if rem == 0 {
		return d
	}
	return d + (time.Minute - rem)
}
