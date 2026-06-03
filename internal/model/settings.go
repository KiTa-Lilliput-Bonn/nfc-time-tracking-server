package model

import (
	"strings"
	"time"
)

type Setting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type BreakRule struct {
	MinWorkHours float64 `json:"min_work_hours"`
	BreakMinutes int     `json:"break_minutes"`
}

type WeeklyHours struct {
	ID           int       `json:"id"`
	UserID       int       `json:"user_id"`
	HoursPerWeek float64   `json:"hours_per_week"`
	ValidFrom    string    `json:"valid_from"`
	CreatedAt    time.Time `json:"created_at"`
}

type VacationEntitlement struct {
	ID          int       `json:"id"`
	UserID      int       `json:"user_id"`
	DaysPerYear float64   `json:"days_per_year"`
	ValidFrom   string    `json:"valid_from"`
	CreatedAt   time.Time `json:"created_at"`
}

// FixedNonWorkWeekdays is one versioned block of fixed free weekdays (Mo–Fr, 1=Monday … 5=Friday).
type FixedNonWorkWeekdays struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Weekdays  []int     `json:"weekdays"`
	ValidFrom string    `json:"valid_from"`
	CreatedAt time.Time `json:"created_at"`
}

// ScheduleBoundSetting is one versioned row: whether worked hours are clipped to the planned shift.
type ScheduleBoundSetting struct {
	ID            int       `json:"id"`
	UserID        int       `json:"user_id"`
	ScheduleBound bool      `json:"schedule_bound"`
	ValidFrom     string    `json:"valid_from"`
	CreatedAt     time.Time `json:"created_at"`
}

// NormCalendarDate returns YYYY-MM-DD. SQLite DATE columns may scan as RFC3339 timestamps.
func NormCalendarDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return s
}

// ScheduleBoundForDate returns schedule_bound from the greatest valid_from row with valid_from <= date.
// Default true when no row applies (all employees bound to schedule by default).
func ScheduleBoundForDate(rows []ScheduleBoundSetting, date string) bool {
	if len(rows) == 0 {
		return true
	}
	date = NormCalendarDate(date)
	var best *ScheduleBoundSetting
	for i := range rows {
		row := &rows[i]
		vf := NormCalendarDate(row.ValidFrom)
		if vf > date {
			continue
		}
		if best == nil || vf > NormCalendarDate(best.ValidFrom) {
			best = row
		}
	}
	if best == nil {
		return true
	}
	return best.ScheduleBound
}
