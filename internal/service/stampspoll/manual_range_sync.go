package stampspoll

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const maxInclusiveManualSyncBerlinDays = 14

// ErrManualRangeInverted is returned when from-date is after to-date (Berlin calendar).
var ErrManualRangeInverted = errors.New("manual range sync: from after to")

// ErrManualRangeTooLarge is returned when the inclusive Berlin-day span exceeds 14 days.
var ErrManualRangeTooLarge = errors.New("manual range sync: more than 14 inclusive calendar days")

// LanRangeSyncTargetRow summarizes one LAN target after manual range sync.
type LanRangeSyncTargetRow struct {
	TargetID       string `json:"target_id"`
	Label          string `json:"label,omitempty"`
	PullError      string `json:"pull_error,omitempty"`
	RowsInserted   int    `json:"rows_inserted"`
	PushDaysOK     int    `json:"push_days_ok"`
	PushDaysFailed int    `json:"push_days_failed"`
}

// LanRangeSyncResult is returned by RunManualRangeSync.
type LanRangeSyncResult struct {
	From                string                `json:"from"`
	To                  string                `json:"to"`
	UtcDaysConsidered   int                   `json:"utc_days_considered"`
	Targets             []LanRangeSyncTargetRow `json:"targets"`
}

func inclusiveBerlinCalendarDays(fromBerlin, toBerlin time.Time) int {
	n := 0
	for d := fromBerlin; !d.After(toBerlin); d = d.AddDate(0, 0, 1) {
		n++
	}
	return n
}

func utcCalendarDatesCoveringRange(startBerlin, endBerlinInclusive time.Time) []string {
	a := startBerlin.UTC()
	b := endBerlinInclusive.UTC()
	lo := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	hi := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	var out []string
	for !lo.After(hi) {
		out = append(out, lo.Format("2006-01-02"))
		lo = lo.AddDate(0, 0, 1)
	}
	return out
}

// RunManualRangeSync pulls stamps from each pollable LAN target for the inclusive Berlin calendar
// range [fromBerlin, toBerlin] (date parts only; wall-clock Berlin midnight for each), then pushes
// each UTC calendar day overlapping that window to all pollable targets. Watermarks are never updated.
func (s *Service) RunManualRangeSync(ctx context.Context, fromBerlin, toBerlin time.Time) (*LanRangeSyncResult, error) {
	loc := europeBerlinLocation()
	from0 := time.Date(fromBerlin.Year(), fromBerlin.Month(), fromBerlin.Day(), 0, 0, 0, 0, loc)
	to0 := time.Date(toBerlin.Year(), toBerlin.Month(), toBerlin.Day(), 0, 0, 0, 0, loc)
	if from0.After(to0) {
		return nil, ErrManualRangeInverted
	}
	if inclusiveBerlinCalendarDays(from0, to0) > maxInclusiveManualSyncBerlinDays {
		return nil, ErrManualRangeTooLarge
	}
	endInclusive := time.Date(to0.Year(), to0.Month(), to0.Day(), 23, 59, 59, 999999999, loc)
	q := url.Values{}
	q.Set("from", from0.Format(time.RFC3339Nano))
	q.Set("to", endInclusive.Format(time.RFC3339Nano))

	pts, err := s.preparePollTargets(ctx)
	if err != nil {
		return nil, err
	}
	utcDays := utcCalendarDatesCoveringRange(from0, endInclusive)
	out := &LanRangeSyncResult{
		From:              from0.Format("2006-01-02"),
		To:                to0.Format("2006-01-02"),
		UtcDaysConsidered: len(utcDays),
		Targets:           make([]LanRangeSyncTargetRow, 0, len(pts)),
	}
	if len(pts) == 0 {
		return out, nil
	}

	rows := make([]LanRangeSyncTargetRow, len(pts))
	for i, pt := range pts {
		rows[i].TargetID = pt.Target.ID
		rows[i].Label = pt.Target.Label
		ins, perr := s.runPollOneWithQuery(ctx, pt, q, "", false)
		rows[i].RowsInserted = ins
		if perr != nil {
			rows[i].PullError = perr.Error()
		}
	}

	for _, utcDay := range utcDays {
		body, berr := s.lanPushJSONForUtcDate(ctx, utcDay)
		if berr != nil {
			for i := range rows {
				rows[i].PushDaysFailed++
			}
			continue
		}
		for i, pt := range pts {
			if rows[i].PullError != "" {
				rows[i].PushDaysFailed++
				continue
			}
			if s.pushStampsPOSTOne(ctx, pt, body) {
				rows[i].PushDaysOK++
			} else {
				rows[i].PushDaysFailed++
			}
		}
	}

	out.Targets = append(out.Targets, rows...)
	return out, nil
}

// ParseBerlinYMD parses YYYY-MM-DD in Europe/Berlin (midnight on that calendar day).
func ParseBerlinYMD(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	loc := europeBerlinLocation()
	t, err := time.ParseInLocation("2006-01-02", s, loc)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}
