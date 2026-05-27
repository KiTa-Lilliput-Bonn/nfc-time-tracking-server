package scheduleimport

// Report ist die JSON-Antwort des Excel-Imports (Ergebnisbericht).
type Report struct {
	Weeks []WeekReport `json:"weeks"`

	SchedulesWritten int `json:"schedules_written"`
	SchedulesDeleted int `json:"schedules_deleted"`

	AbsencesCreated  int `json:"absences_created"`
	AbsencesReplaced int `json:"absences_replaced"`
	// AbsencesSkipped: Feiertagsspalten mit Inhalt (je Werktagsspalte max. 1, nicht × Mitarbeiter).
	AbsencesSkipped int `json:"absences_skipped"`

	WeekNotesUpdated int `json:"week_notes_updated"`
	// PastCellsSkipped: Zellen mit Inhalt, deren Datum vor dem Import-Stichtag liegt (keine DB-Änderung).
	PastCellsSkipped int `json:"past_cells_skipped"`
	// PastWeekNotesSkipped: Anzahl KW-Blöcke, bei denen eine Notiz wegen komplett vergangener Woche nicht gespeichert wurde.
	PastWeekNotesSkipped int `json:"past_week_notes_skipped"`
	// PastTeamMeetingsSkipped: Teamsitzungen (Montag) in vergangenen Wochen mit Inhalt, nicht importiert (Future-Lauf).
	PastTeamMeetingsSkipped int `json:"past_team_meetings_skipped"`

	TeamMeetingsCreated int `json:"team_meetings_created"`

	UnknownNames []string `json:"unknown_names"`
	Errors       []string `json:"errors"`
	Warnings     []string `json:"warnings"`
}

// WeekReport fasst eine erkannte Kalenderwoche zusammen.
type WeekReport struct {
	ISOYear int `json:"iso_year"`
	ISOWk   int `json:"iso_week"`

	NotesWritten bool `json:"notes_written"`

	TimesWritten int `json:"times_written"`
	TimesCleared int `json:"times_cleared"`

	AbsencesCreated  int `json:"absences_created"`
	AbsencesReplaced int `json:"absences_replaced"`

	PastCellsSkipped int `json:"past_cells_skipped"`

	TeamMondaySections []TeamMondaySectionReport `json:"team_monday_sections,omitempty"`
}

// TimeSpan ist ein Uhrzeitfenster (HH:MM).
type TimeSpan struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// TeamMondaySectionReport beschreibt eine „Datum“-Sektion für die Import-Antwort.
type TeamMondaySectionReport struct {
	Monday     string     `json:"monday"`
	RawLine    string     `json:"raw_line,omitempty"`
	NoMeetings bool       `json:"no_meetings"`
	GroupTeam  *TimeSpan  `json:"group_team,omitempty"`
	AllTeam    *TimeSpan  `json:"all_team,omitempty"`
	Employees  []string   `json:"employees,omitempty"`
}
