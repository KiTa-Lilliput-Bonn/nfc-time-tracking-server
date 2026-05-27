package importer

import (
	"context"
	"fmt"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/compensationday"
	"nfc-time-tracking-server/internal/store"
)

// RecomputeAffectedDaysFromPunches rebuilds work periods for each (user, date) touched by raw punches.
func RecomputeAffectedDaysFromPunches(ctx context.Context, punch store.PunchStore, periods store.WorkPeriodStore, tags store.NFCTagStore, claims store.CompensationDayClaimStore, fnw store.FixedNonWorkWeekdaysStore, raw []model.RawPunch) (days int, err error) {
	type dayKey struct {
		uid int
		d   string
	}
	todo := make(map[dayKey]struct{})
	for _, p := range raw {
		uid, e := tags.ResolveUserID(ctx, p.NFCTagUID, p.PunchTime)
		if e != nil || uid == 0 {
			continue
		}
		d := p.PunchTime.UTC().Format("2006-01-02")
		todo[dayKey{uid: uid, d: d}] = struct{}{}
	}
	for k := range todo {
		punches, e := punch.ListByUserAndDate(ctx, k.uid, k.d)
		if e != nil {
			return 0, fmt.Errorf("list punches user %d date %s: %w", k.uid, k.d, e)
		}
		wps := PairPunches(k.uid, k.d, punches)
		if e := periods.ReplaceForUserDate(ctx, k.uid, k.d, wps); e != nil {
			return 0, fmt.Errorf("replace periods user %d date %s: %w", k.uid, k.d, e)
		}
		if e := compensationday.SyncClaimAfterWorkDayChange(ctx, fnw, periods, nil, claims, k.uid, k.d); e != nil {
			return 0, fmt.Errorf("sync compensation claim user %d date %s: %w", k.uid, k.d, e)
		}
	}
	return len(todo), nil
}
