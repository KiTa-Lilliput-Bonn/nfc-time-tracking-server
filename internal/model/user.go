package model

import "time"

type Role string

const (
	RoleUser       Role = "user"
	RoleLeitung    Role = "leitung"
	RoleSuperadmin Role = "superadmin"
)

type User struct {
	ID                 int       `json:"id"`
	Username           string    `json:"username"`
	PasswordHash       string    `json:"-"`
	DisplayName        string    `json:"display_name"`
	// GroupID: höchstens eine Gruppe pro Benutzer; nil = keiner Gruppe zugeordnet.
	GroupID *int `json:"group_id,omitempty"`
	Role               Role      `json:"role"`
	Active             bool      `json:"active"`
	MustChangePassword bool      `json:"must_change_password"`
	// DefaultTeamMeetingParticipant: bei false nicht per „Alle“/Gruppe/Excel-Import vorausgewählt.
	DefaultTeamMeetingParticipant bool `json:"default_team_meeting_participant"`
	// OpeningHoursBalance: Stundensaldo aus Alt-/Fremdsystem, wird fürs Jahr mit Ist−Soll addiert.
	OpeningHoursBalance float64 `json:"opening_hours_balance"`
	// OpeningVacationDays: Urlaub-Startwert (Tage), addiert zu Anspruch − genommen.
	OpeningVacationDays float64 `json:"opening_vacation_days"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// RoleMayHaveNFCTag reports whether a user with this role may be assigned an NFC tag
// (employees and Leitung; not superadmin).
func RoleMayHaveNFCTag(role Role) bool {
	return role == RoleUser || role == RoleLeitung
}

// ActorMayManageUser reports whether actorRole may perform management actions on target
// (employee PATCH, time corrections, schedules for that user, etc.).
// Only superadmin may manage accounts whose role is RoleSuperadmin.
func ActorMayManageUser(actorRole string, target *User) bool {
	if target == nil {
		return false
	}
	if target.Role == RoleSuperadmin {
		return actorRole == string(RoleSuperadmin)
	}
	return true
}
