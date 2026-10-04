package model

import "time"

// ChangeRequestKind unterscheidet die Antragsarten von Mitarbeitenden.
type ChangeRequestKind string

const (
	// RequestTimeCorrection ändert Kommen/Gehen eines vorhandenen Zeiteintrags.
	RequestTimeCorrection ChangeRequestKind = "time_correction"
	// RequestTimeEntry trägt eine fehlende Arbeitszeit nach (z. B. Stempeln vergessen).
	RequestTimeEntry ChangeRequestKind = "time_entry"
	// RequestVacation beantragt Urlaub für einen Zeitraum.
	RequestVacation ChangeRequestKind = "vacation"
)

type ChangeRequestStatus string

const (
	RequestPending   ChangeRequestStatus = "pending"
	RequestApproved  ChangeRequestStatus = "approved"
	RequestRejected  ChangeRequestStatus = "rejected"
	RequestWithdrawn ChangeRequestStatus = "withdrawn"
)

// ChangeRequest ist ein Antrag, der erst nach Freigabe durch die Leitung wirksam wird.
type ChangeRequest struct {
	ID     int                 `json:"id"`
	UserID int                 `json:"user_id"`
	Kind   ChangeRequestKind   `json:"kind"`
	Status ChangeRequestStatus `json:"status"`

	// Zeitkorrektur / Zeiteintrag
	WorkPeriodID *int       `json:"work_period_id,omitempty"`
	WorkDate     string     `json:"work_date,omitempty"`
	OriginalIn   *time.Time `json:"original_in,omitempty"`
	OriginalOut  *time.Time `json:"original_out,omitempty"`
	PunchIn      *time.Time `json:"punch_in,omitempty"`
	PunchOut     *time.Time `json:"punch_out,omitempty"`

	// Urlaub
	DateFrom string `json:"date_from,omitempty"`
	DateTo   string `json:"date_to,omitempty"`
	HalfDay  bool   `json:"half_day"`

	Reason          string     `json:"reason"`
	DecidedBy       *int       `json:"decided_by,omitempty"`
	DecidedAt       *time.Time `json:"decided_at,omitempty"`
	DecisionComment string     `json:"decision_comment"`
	CreatedAt       time.Time  `json:"created_at"`
}
