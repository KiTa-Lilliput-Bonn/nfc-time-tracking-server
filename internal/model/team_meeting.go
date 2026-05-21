package model

// TeamMeetingKind unterscheidet Gruppen- vs. Gesamtteam-Sitzung (Montag).
type TeamMeetingKind string

const (
	TeamMeetingKindKT TeamMeetingKind = "kt"
	TeamMeetingKindGT TeamMeetingKind = "gt"
)

// TeamMeeting ist eine geplante Teamsitzung (ein Objekt pro Zeitfenster und Art).
type TeamMeeting struct {
	ID           int             `json:"id"`
	ISOWeekYear  int             `json:"iso_week_year"`
	ISOWeek      int             `json:"iso_week"`
	MeetingDate  string          `json:"meeting_date"` // YYYY-MM-DD (Montag)
	Kind         TeamMeetingKind `json:"kind"`
	TimeStart    string          `json:"time_start"` // HH:MM
	TimeEnd      string          `json:"time_end"`
	Source       string          `json:"source"` // excel | manual
	SectionIndex int             `json:"section_index"`
	UserIDs      []int           `json:"user_ids"`
}
