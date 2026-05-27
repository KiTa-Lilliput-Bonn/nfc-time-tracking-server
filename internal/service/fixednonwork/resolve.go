package fixednonwork

import (
	"context"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store"
)

// WeekdaysFromRows resolves fixed free weekdays on date from version history.
func WeekdaysFromRows(rows []model.FixedNonWorkWeekdays, date string) []int {
	return model.FixedNonWorkWeekdaysForDate(rows, date)
}

// WeekdaysForUserDate loads the effective fixed free weekdays for user on date.
func WeekdaysForUserDate(ctx context.Context, s store.FixedNonWorkWeekdaysStore, userID int, date string) []int {
	if s == nil {
		return nil
	}
	row, err := s.GetForDate(ctx, userID, date)
	if err != nil || row == nil {
		return nil
	}
	return append([]int(nil), row.Weekdays...)
}
