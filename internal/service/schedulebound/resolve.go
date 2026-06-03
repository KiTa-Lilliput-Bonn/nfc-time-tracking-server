package schedulebound

import (
	"context"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store"
)

// BoundForUserDateFromRows resolves whether the user is bound to the schedule on date (YYYY-MM-DD).
func BoundForUserDateFromRows(rows []model.ScheduleBoundSetting, date string) bool {
	return model.ScheduleBoundForDate(rows, date)
}

// BoundForUserDate loads history and resolves schedule-bound for date.
func BoundForUserDate(ctx context.Context, s store.ScheduleBoundStore, userID int, date string) (bool, error) {
	if s == nil {
		return true, nil
	}
	rows, err := s.ListByUser(ctx, userID)
	if err != nil {
		return true, err
	}
	return BoundForUserDateFromRows(rows, date), nil
}
