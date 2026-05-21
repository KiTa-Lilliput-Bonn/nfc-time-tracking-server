package model

import "time"

type CompensationDayClaimStatus string

const (
	CompensationDayClaimOpen   CompensationDayClaimStatus = "open"
	CompensationDayClaimUsed   CompensationDayClaimStatus = "used"
	CompensationDayClaimWaived CompensationDayClaimStatus = "waived"
)

type CompensationDayClaim struct {
	ID            int                        `json:"id"`
	UserID        int                        `json:"user_id"`
	WorkDate      string                     `json:"work_date"`
	Status        CompensationDayClaimStatus `json:"status"`
	UsedAbsenceID *int                       `json:"used_absence_id"`
	CreatedAt     time.Time                  `json:"created_at"`
	UpdatedAt     time.Time                  `json:"updated_at"`
}
