package model

type Schedule struct {
	ID           int    `json:"id"`
	UserID       int    `json:"user_id"`
	ScheduleDate string `json:"schedule_date"`
	ShiftStart   string `json:"shift_start"`
	ShiftEnd     string `json:"shift_end"`
}
