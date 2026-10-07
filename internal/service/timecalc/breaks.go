package timecalc

import (
	"math"
	"sort"
	"time"

	"nfc-time-tracking-server/internal/model"
)

// CalcBreakDeduction returns the progressive break deduction for grossWork.
// Required break builds up minute-by-minute past each threshold until that rule's
// break_minutes (absolute) is reached. stampedBreaks are credited against the required amount.
func CalcBreakDeduction(grossWork, stampedBreaks time.Duration, rules []model.BreakRule) time.Duration {
	required := progressiveRequiredBreak(grossWork, rules)
	if required <= 0 {
		return 0
	}
	stampedMin := int(stampedBreaks / time.Minute)
	if stampedMin >= required {
		return 0
	}
	return time.Duration(required-stampedMin) * time.Minute
}

// progressiveRequiredBreak returns how many break minutes are required for grossWork
// under the configured rules (sorted by threshold ascending, incremental toward absolute break_minutes).
func progressiveRequiredBreak(grossWork time.Duration, rules []model.BreakRule) int {
	if len(rules) == 0 || grossWork <= 0 {
		return 0
	}
	sorted := append([]model.BreakRule(nil), rules...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].MinWorkHours == sorted[j].MinWorkHours {
			return sorted[i].BreakMinutes < sorted[j].BreakMinutes
		}
		return sorted[i].MinWorkHours < sorted[j].MinWorkHours
	})

	var required int
	prevRequired := 0
	for _, r := range sorted {
		incremental := r.BreakMinutes - prevRequired
		if incremental < 0 {
			incremental = 0
		}
		thresholdMin := int(math.Round(r.MinWorkHours * 60))
		threshold := time.Duration(thresholdMin) * time.Minute
		over := grossWork - threshold
		if over < 0 {
			over = 0
		}
		overMin := int(over / time.Minute)
		if overMin < incremental {
			required += overMin
		} else {
			required += incremental
		}
		prevRequired = r.BreakMinutes
	}
	return required
}

// RequiredBreakMinutes returns the progressive required break (minutes) for a planned shift of grossWork.
func RequiredBreakMinutes(grossWork time.Duration, rules []model.BreakRule) int {
	return progressiveRequiredBreak(grossWork, rules)
}
