package model

import "time"

type ShiftAlertDismissal struct {
	UserID      int       `json:"user_id"`
	WorkDate    string    `json:"work_date"`
	DismissedBy int       `json:"dismissed_by"`
	CreatedAt   time.Time `json:"created_at"`
}
