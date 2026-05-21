package timecalc

import (
	"time"

	"nfc-time-tracking-server/internal/model"
)

// CalcBreakDeduction returns minutes to deduct when stamped breaks are shorter than required.
// Callers may pass stampedBreaks=0 to get the full required break for grossWork (e.g. per work block).
func CalcBreakDeduction(grossWork, stampedBreaks time.Duration, rules []model.BreakRule) time.Duration {
	if len(rules) == 0 {
		return 0
	}
	grossH := grossWork.Hours()
	var required int
	var bestThreshold float64
	for _, r := range rules {
		if grossH+1e-9 >= r.MinWorkHours && r.MinWorkHours >= bestThreshold {
			bestThreshold = r.MinWorkHours
			required = r.BreakMinutes
		}
	}
	if required == 0 {
		return 0
	}
	stampedMin := int(stampedBreaks / time.Minute)
	if stampedMin >= required {
		return 0
	}
	return time.Duration(required-stampedMin) * time.Minute
}
