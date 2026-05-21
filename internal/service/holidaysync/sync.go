package holidaysync

import (
	"context"
	"fmt"
	"time"

	"nfc-time-tracking-server/internal/service/timecalc"
	"nfc-time-tracking-server/internal/store"
)

// HolidayCalendarLocation loads Europe/Berlin for calendar-year boundaries;
// if that fails (e.g. tzdata missing), it returns UTC.
func HolidayCalendarLocation() (loc *time.Location, berlin bool) {
	l, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return time.UTC, false
	}
	return l, true
}

// CurrentAndNextCalendarYears returns [Y, Y+1] where Y is the calendar year of now in loc.
func CurrentAndNextCalendarYears(now time.Time, loc *time.Location) []int {
	if loc == nil {
		loc = time.UTC
	}
	y := now.In(loc).Year()
	return []int{y, y + 1}
}

// EnsureNRWHolidays inserts each NRW public holiday for the given years when no row
// exists for that date (idempotent; does not replace manual entries on the same date).
func EnsureNRWHolidays(ctx context.Context, holidays store.HolidayStore, years []int) error {
	for _, y := range years {
		for _, h := range timecalc.GenerateNRWHolidays(y) {
			hh := h
			existing, err := holidays.GetForDate(ctx, hh.HolidayDate)
			if err != nil {
				return fmt.Errorf("holiday get %s: %w", hh.HolidayDate, err)
			}
			if existing != nil {
				continue
			}
			if err := holidays.Create(ctx, &hh); err != nil {
				return fmt.Errorf("holiday create %s: %w", hh.HolidayDate, err)
			}
		}
	}
	return nil
}
