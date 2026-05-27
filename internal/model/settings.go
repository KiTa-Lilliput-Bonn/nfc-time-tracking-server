package model

import "time"

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
