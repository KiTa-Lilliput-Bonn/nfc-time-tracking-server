package scheduleexport

import (
	"strings"
	"testing"

	"nfc-time-tracking-server/internal/model"
)

func TestMergeOtherMeetingsIntoNotesHTML(t *testing.T) {
	meetings := []model.TeamMeeting{
		{
			Kind:        model.TeamMeetingKindOther,
			Label:       "Fortbildung",
			MeetingDate: "2026-03-18",
			TimeStart:   "09:00",
			TimeEnd:     "10:00",
		},
	}
	got := mergeOtherMeetingsIntoNotesHTML("<p>Bestehend</p>", meetings)
	if !strings.Contains(got, "Bestehend") {
		t.Fatalf("missing existing notes: %q", got)
	}
	if !strings.Contains(got, "Mi: Fortbildung 09:00–10:00") {
		t.Fatalf("missing other line: %q", got)
	}
}

func TestFormatOtherMeetingNoteLine(t *testing.T) {
	line := formatOtherMeetingNoteLine(model.TeamMeeting{
		Kind:        model.TeamMeetingKindOther,
		Label:       "Workshop",
		MeetingDate: "2026-03-16",
		TimeStart:   "14:00",
		TimeEnd:     "16:00",
	})
	if line != "Mo: Workshop 14:00–16:00" {
		t.Fatalf("got %q", line)
	}
}
