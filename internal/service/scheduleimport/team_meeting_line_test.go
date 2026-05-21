package scheduleimport

import "testing"

func TestParseTeamMeetingLine_NoTeam(t *testing.T) {
	got := ParseTeamMeetingLine("kein Team heute")
	if got.Kind != TeamMeetingLineNoMeetings {
		t.Fatalf("kind: %v", got.Kind)
	}
}

func TestParseTeamMeetingLine_KT(t *testing.T) {
	got := ParseTeamMeetingLine("KT 10:00 - 11:30")
	if got.Kind != TeamMeetingLineScheduled {
		t.Fatalf("kind: %v", got.Kind)
	}
	if got.KTStart != "10:00" || got.KTEnd != "11:30" {
		t.Fatalf("KT: %q–%q", got.KTStart, got.KTEnd)
	}
}

func TestParseTeamMeetingLine_GT(t *testing.T) {
	got := ParseTeamMeetingLine("GT 17:00-19:00")
	if got.Kind != TeamMeetingLineScheduled {
		t.Fatalf("kind: %v", got.Kind)
	}
	if got.GTStart != "17:00" || got.GTEnd != "19:00" {
		t.Fatalf("GT: %q–%q", got.GTStart, got.GTEnd)
	}
}

func TestParseTeamMeetingLine_UnrecognizedGT(t *testing.T) {
	got := ParseTeamMeetingLine("GT irgendwas")
	if got.Kind != TeamMeetingLineUnspecified {
		t.Fatalf("kind: %v", got.Kind)
	}
	if len(got.Warnings) == 0 {
		t.Fatal("expected warning")
	}
}
