package model

import (
	"fmt"
	"time"
)

// ValidateFixedNonWorkWeekdays ensures each entry is Monday–Friday (1–5 in time.Weekday),
// no duplicates, and at least one weekday remains a workday.
func ValidateFixedNonWorkWeekdays(fixed []int) error {
	if len(fixed) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(fixed))
	for _, w := range fixed {
		if w < int(time.Monday) || w > int(time.Friday) {
			return fmt.Errorf("fixed_non_work_weekdays: weekday %d out of range (only Monday=1 .. Friday=5)", w)
		}
		if _, ok := seen[w]; ok {
			return fmt.Errorf("fixed_non_work_weekdays: duplicate weekday %d", w)
		}
		seen[w] = struct{}{}
	}
	if len(seen) >= 5 {
		return fmt.Errorf("fixed_non_work_weekdays: at least one weekday must remain a workday")
	}
	return nil
}

// ScheduledWorkdaysPerWeek counts Monday–Friday minus fixed non-work weekdays.
func ScheduledWorkdaysPerWeek(fixed []int) int {
	n := 5
	seen := make(map[int]struct{})
	for _, w := range fixed {
		if w < int(time.Monday) || w > int(time.Friday) {
			continue
		}
		if _, ok := seen[w]; ok {
			continue
		}
		seen[w] = struct{}{}
		n--
	}
	if n < 1 {
		return 1
	}
	return n
}

// IsEmployeeWorkday is true for Monday–Friday that are not in fixed non-work weekdays.
func IsEmployeeWorkday(day time.Time, fixed []int) bool {
	wd := day.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}
	for _, w := range fixed {
		if int(wd) == w {
			return false
		}
	}
	return true
}

// DailyHours returns hours_per_week / scheduled workdays (Mon–Fri count).
func DailyHours(hoursPerWeek float64, fixed []int) float64 {
	if hoursPerWeek <= 0 {
		return 0
	}
	sw := ScheduledWorkdaysPerWeek(fixed)
	return hoursPerWeek / float64(sw)
}

// FixedNonWorkWeekdaysForDate returns weekdays from the greatest valid_from row with valid_from <= date.
func FixedNonWorkWeekdaysForDate(rows []FixedNonWorkWeekdays, date string) []int {
	if len(rows) == 0 {
		return nil
	}
	date = NormCalendarDate(date)
	var best *FixedNonWorkWeekdays
	for i := range rows {
		row := &rows[i]
		vf := NormCalendarDate(row.ValidFrom)
		if vf > date {
			continue
		}
		if best == nil || vf > NormCalendarDate(best.ValidFrom) ||
			(vf == NormCalendarDate(best.ValidFrom) && row.ID > best.ID) {
			best = row
		}
	}
	if best == nil {
		return nil
	}
	return append([]int(nil), best.Weekdays...)
}

// IsFixedNonWorkWeekday reports whether wd (Monday..Friday) is a fixed non-work day.
func IsFixedNonWorkWeekday(wd time.Weekday, fixed []int) bool {
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}
	for _, w := range fixed {
		if int(wd) == w {
			return true
		}
	}
	return false
}
