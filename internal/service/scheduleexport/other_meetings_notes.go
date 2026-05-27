package scheduleexport

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
)

var weekdayShortDE = []string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}

// mergeOtherMeetingsIntoNotesHTML hängt Sonstiges-Sitzungen (kind=other) an die Wochennotizen für Spalte I an.
func mergeOtherMeetingsIntoNotesHTML(notes string, meetings []model.TeamMeeting) string {
	var others []model.TeamMeeting
	for _, m := range meetings {
		if m.Kind == model.TeamMeetingKindOther {
			others = append(others, m)
		}
	}
	if len(others) == 0 {
		return notes
	}
	sort.Slice(others, func(i, j int) bool {
		if others[i].MeetingDate != others[j].MeetingDate {
			return others[i].MeetingDate < others[j].MeetingDate
		}
		return others[i].TimeStart < others[j].TimeStart
	})

	var lines []string
	for _, m := range others {
		lines = append(lines, formatOtherMeetingNoteLine(m))
	}
	block := otherMeetingsNotesHTML(lines)
	notes = strings.TrimSpace(notes)
	if notes == "" {
		return block
	}
	return notes + `<p><br></p>` + block
}

func formatOtherMeetingNoteLine(m model.TeamMeeting) string {
	label := strings.TrimSpace(m.Label)
	if label == "" {
		label = "Sonstiges"
	}
	day := weekdayShortFromDate(m.MeetingDate)
	return fmt.Sprintf("%s: %s %s–%s",
		day, label,
		formatTeamMeetingClock(m.TimeStart),
		formatTeamMeetingClock(m.TimeEnd))
}

func weekdayShortFromDate(ymd string) string {
	ymd = strings.TrimSpace(ymd)
	if len(ymd) < 10 {
		return "?"
	}
	t, err := time.ParseInLocation("2006-01-02", ymd[:10], time.Local)
	if err != nil {
		return "?"
	}
	return weekdayShortDE[int(t.Weekday())]
}

func otherMeetingsNotesHTML(lines []string) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(line))
		b.WriteString("</p>")
	}
	return b.String()
}
