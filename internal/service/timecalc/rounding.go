package timecalc

import "time"

// RoundDown rounds duration down to the nearest multiple of unitMinutes.
func RoundDown(d time.Duration, unitMinutes int) time.Duration {
	unit := time.Duration(unitMinutes) * time.Minute
	return (d / unit) * unit
}
