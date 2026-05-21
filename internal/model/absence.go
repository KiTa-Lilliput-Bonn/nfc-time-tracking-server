package model

import "time"

type AbsenceType string

const (
	AbsenceSick            AbsenceType = "sick"
	AbsenceVacation        AbsenceType = "vacation"
	AbsenceOther           AbsenceType = "other"
	AbsenceCompensationDay AbsenceType = "compensation_day"
)

type Absence struct {
	ID          int         `json:"id"`
	UserID      int         `json:"user_id"`
	AbsenceDate string      `json:"absence_date"`
	AbsenceType AbsenceType `json:"absence_type"`
	HalfDay     bool        `json:"half_day"`
	CreatedBy   int         `json:"created_by"`
	CreatedAt   time.Time   `json:"created_at"`
}
