package scheduleimport

import (
	"regexp"
	"strings"
)

// TeamMeetingLineKind klassifiziert die Zeile über einer „Datum“-Zeile.
type TeamMeetingLineKind int

const (
	TeamMeetingLineUnspecified TeamMeetingLineKind = iota
	TeamMeetingLineNoMeetings
	TeamMeetingLineScheduled
)

// ParsedTeamMeetingLine ist das aus Spalte B gelesene Teamsitzungs-Motto pro Sektion.
type ParsedTeamMeetingLine struct {
	Kind TeamMeetingLineKind
	Raw  string

	KTStart, KTEnd string
	GTStart, GTEnd string

	Warnings []string
}

// ParsedTeamMondaySection gehört zu einem „Datum“-Block (Montag = Dates[0]).
type ParsedTeamMondaySection struct {
	Line             ParsedTeamMeetingLine
	EmployeeRawNames []string
}

var (
	ktSlotRE = regexp.MustCompile(`(?i)\bKT\s*(\d{1,2}[.:]\d{2}\s*-\s*\d{1,2}[.:]\d{2})`)
	gtSlotRE = regexp.MustCompile(`(?i)\bGT\s*(\d{1,2}[.:]\d{2}\s*-\s*\d{1,2}[.:]\d{2})`)
)

// ParseTeamMeetingLine wertet die Zelle direkt über „Datum“ aus (Spalte B).
func ParseTeamMeetingLine(prev string) ParsedTeamMeetingLine {
	s := trimCell(prev)
	if s == "" {
		return ParsedTeamMeetingLine{Kind: TeamMeetingLineUnspecified}
	}
	if weekHeaderRE.MatchString(s) || strings.EqualFold(s, "Datum") {
		return ParsedTeamMeetingLine{Kind: TeamMeetingLineUnspecified}
	}
	low := strings.ToLower(s)
	if strings.Contains(low, "kein team") {
		return ParsedTeamMeetingLine{Kind: TeamMeetingLineNoMeetings, Raw: s}
	}

	out := ParsedTeamMeetingLine{Kind: TeamMeetingLineScheduled, Raw: s}
	if sm := ktSlotRE.FindStringSubmatch(s); len(sm) > 1 {
		if a, b, ok := ParseClockRange(sm[1]); ok {
			out.KTStart, out.KTEnd = a, b
		} else {
			out.Warnings = append(out.Warnings, "KT-Zeitspanne nicht erkannt")
		}
	}
	if sm := gtSlotRE.FindStringSubmatch(s); len(sm) > 1 {
		if a, b, ok := ParseClockRange(sm[1]); ok {
			out.GTStart, out.GTEnd = a, b
		} else {
			out.Warnings = append(out.Warnings, "GT-Zeitspanne nicht erkannt")
		}
	}

	hasKT := strings.Contains(low, "kt")
	hasGT := strings.Contains(low, "gt")
	if (hasKT && out.KTStart == "") || (hasGT && out.GTStart == "") {
		if len(out.Warnings) == 0 {
			out.Warnings = append(out.Warnings, "Teamsitzungszeile enthält KT/GT ohne lesbare Uhrzeit")
		}
	}
	if !hasKT && !hasGT && !strings.Contains(low, "kein team") {
		// z. B. "GT irgendwas"
		out.Kind = TeamMeetingLineUnspecified
		out.Warnings = append(out.Warnings, "Teamsitzungszeile nicht erkannt (erwartet KT/GT mit Uhrzeit oder „kein team“)")
	}
	if out.Kind == TeamMeetingLineScheduled && out.KTStart == "" && out.GTStart == "" {
		out.Kind = TeamMeetingLineUnspecified
	}
	return out
}
