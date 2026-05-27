package compensationday

import (
	"context"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/fixednonwork"
	"nfc-time-tracking-server/internal/store"
)

// SyncClaimAfterWorkDayChange maintains open compensation-day claims for weekend dates
// and for fixed non-work weekdays when the user worked that calendar day:
// one open claim exists iff there is at least one closed non-break work interval (after latest correction).
func SyncClaimAfterWorkDayChange(
	ctx context.Context,
	fnw store.FixedNonWorkWeekdaysStore,
	wp store.WorkPeriodStore,
	corrs store.CorrectionStore,
	claims store.CompensationDayClaimStore,
	userID int,
	dateStr string,
) error {
	if claims == nil {
		return nil
	}
	fixed := fixednonwork.WeekdaysForUserDate(ctx, fnw, userID, dateStr)
	if !isCompensationClaimEligibleDate(dateStr, fixed) {
		return nil
	}
	ok, err := dayHasEligibleWork(ctx, wp, corrs, userID, dateStr)
	if err != nil {
		return err
	}
	return claims.EnsureForWorkDate(ctx, userID, dateStr, ok)
}

func isWeekendDate(dateStr string) bool {
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return false
	}
	switch t.Weekday() {
	case time.Saturday, time.Sunday:
		return true
	default:
		return false
	}
}

func isCompensationClaimEligibleDate(dateStr string, fixedNonWork []int) bool {
	if isWeekendDate(dateStr) {
		return true
	}
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return false
	}
	return model.IsFixedNonWorkWeekday(t.Weekday(), fixedNonWork)
}

func dayHasEligibleWork(ctx context.Context, wp store.WorkPeriodStore, corrs store.CorrectionStore, userID int, dateStr string) (bool, error) {
	periods, err := wp.ListByUserDateRange(ctx, userID, dateStr, dateStr)
	if err != nil {
		return false, err
	}
	for _, p := range periods {
		if p.IsBreak {
			continue
		}
		start := p.PunchIn
		end := p.PunchOut
		if corrs != nil {
			if c, err := corrs.GetLatestForPeriod(ctx, p.ID); err == nil && c != nil {
				start = c.CorrectedIn
				e := c.CorrectedOut
				end = &e
			}
		}
		if end != nil && end.After(start) {
			return true, nil
		}
	}
	return false, nil
}

// BootstrapScanUsers recomputes weekend and fixed-non-work claims from stored work periods (import/manual).
func BootstrapScanUsers(ctx context.Context, users store.UserStore, fnw store.FixedNonWorkWeekdaysStore, wp store.WorkPeriodStore, corrs store.CorrectionStore, claims store.CompensationDayClaimStore) error {
	if claims == nil {
		return nil
	}
	list, err := users.List(ctx, true)
	if err != nil {
		return err
	}
	for _, u := range list {
		periods, err := wp.ListByUserDateRange(ctx, u.ID, "1970-01-01", "2099-12-31")
		if err != nil {
			return err
		}
		seen := map[string]struct{}{}
		for _, p := range periods {
			fixed := fixednonwork.WeekdaysForUserDate(ctx, fnw, u.ID, p.WorkDate)
			if !isCompensationClaimEligibleDate(p.WorkDate, fixed) {
				continue
			}
			if _, ok := seen[p.WorkDate]; ok {
				continue
			}
			seen[p.WorkDate] = struct{}{}
			if err := SyncClaimAfterWorkDayChange(ctx, fnw, wp, corrs, claims, u.ID, p.WorkDate); err != nil {
				return err
			}
		}
	}
	return nil
}
