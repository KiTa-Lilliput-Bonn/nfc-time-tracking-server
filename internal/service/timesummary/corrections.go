package timesummary

import "nfc-time-tracking-server/internal/model"

// LatestCorrectionByWorkPeriod maps each work_period_id to its latest correction
// (highest id / most recent created_at).
func LatestCorrectionByWorkPeriod(corrs []model.TimeCorrection) map[int]model.TimeCorrection {
	out := make(map[int]model.TimeCorrection)
	for _, c := range corrs {
		if prev, ok := out[c.WorkPeriodID]; ok {
			if c.ID <= prev.ID {
				continue
			}
		}
		out[c.WorkPeriodID] = c
	}
	return out
}

// ApplyLatestCorrections returns periods for hour summation: disabled periods are
// dropped; other latest corrections replace punch_in/out.
func ApplyLatestCorrections(periods []model.WorkPeriod, corrs []model.TimeCorrection) []model.WorkPeriod {
	latest := LatestCorrectionByWorkPeriod(corrs)
	out := make([]model.WorkPeriod, 0, len(periods))
	for _, wp := range periods {
		if c, ok := latest[wp.ID]; ok {
			if c.Disabled {
				continue
			}
			wp.PunchIn = c.CorrectedIn
			co := c.CorrectedOut
			wp.PunchOut = &co
		}
		out = append(out, wp)
	}
	return out
}
