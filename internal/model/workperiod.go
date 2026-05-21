package model

import "time"

type WorkPeriod struct {
	ID       int        `json:"id"`
	UserID   int        `json:"user_id"`
	WorkDate string     `json:"work_date"`
	PunchIn  time.Time  `json:"punch_in"`
	PunchOut *time.Time `json:"punch_out"`
	IsBreak  bool       `json:"is_break"`
	Source   string     `json:"source"`
}

type TimeCorrection struct {
	ID           int       `json:"id"`
	WorkPeriodID int       `json:"work_period_id"`
	CorrectedIn  time.Time `json:"corrected_in"`
	CorrectedOut time.Time `json:"corrected_out"`
	Reason       string    `json:"reason"`
	CorrectedBy  int       `json:"corrected_by"`
	CreatedAt    time.Time `json:"created_at"`
}
