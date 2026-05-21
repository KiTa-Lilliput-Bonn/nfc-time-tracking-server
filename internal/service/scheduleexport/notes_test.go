package scheduleexport

import (
	"strings"
	"testing"

	"nfc-time-tracking-server/internal/model"
)

func TestHtmlToRichRuns_paragraphs(t *testing.T) {
	runs := htmlToRichRuns("<p>Zeile eins</p><p>Zeile zwei</p>")
	if len(runs) == 0 {
		t.Fatal("no runs")
	}
	text := runs[0].Text
	if !strings.Contains(text, "Zeile eins") || !strings.Contains(text, "Zeile zwei") {
		t.Fatalf("text: %q", text)
	}
	if !strings.Contains(text, "\n") {
		t.Fatalf("expected newline between paragraphs, got %q", text)
	}
}

func TestHtmlToRichRuns_colorResetsAfterSpanClose(t *testing.T) {
	runs := htmlToRichRuns(`<p>normal <span style="color: rgb(230, 0, 0);">rot</span> wieder normal</p>`)
	for _, r := range runs {
		if r.Font == nil {
			continue
		}
		if strings.Contains(r.Text, "wieder") && r.Font.Color != "" {
			t.Fatalf("text after red span must not be colored, got color %q in %q", r.Font.Color, r.Text)
		}
		if strings.Contains(r.Text, "rot") && !strings.EqualFold(r.Font.Color, "E60000") {
			t.Fatalf("red span: color %q", r.Font.Color)
		}
	}
}

func TestHtmlToRichRuns_colorAndUnderline(t *testing.T) {
	runs := htmlToRichRuns(`<p><u>unter</u> <span style="color: rgb(230, 0, 0);">rot</span></p>`)
	if len(runs) < 2 {
		t.Fatalf("runs: %d", len(runs))
	}
	var foundUnderline, foundColor bool
	for _, r := range runs {
		if r.Font == nil {
			continue
		}
		if strings.Contains(r.Text, "unter") && r.Font.Underline == "single" {
			foundUnderline = true
		}
		if strings.Contains(r.Text, "rot") && strings.EqualFold(r.Font.Color, "E60000") {
			foundColor = true
		}
	}
	if !foundUnderline {
		t.Fatal("expected underline on 'unter'")
	}
	if !foundColor {
		t.Fatal("expected red color on 'rot'")
	}
}

func TestHtmlToRichRuns_brAndEmptyParagraph(t *testing.T) {
	runs := htmlToRichRuns("<p>A</p><p><br></p><p>B</p>")
	text := runs[0].Text
	if strings.Count(text, "\n") < 2 {
		t.Fatalf("expected multiple line breaks, got %q", text)
	}
}

func TestFormatTeamMeetingLine_gtOnEverySection(t *testing.T) {
	meetings := []model.TeamMeeting{
		{Kind: model.TeamMeetingKindKT, SectionIndex: 0, TimeStart: "15:30", TimeEnd: "17:00"},
		{Kind: model.TeamMeetingKindGT, SectionIndex: 0, TimeStart: "17:00", TimeEnd: "19:00"},
	}
	line0 := formatTeamMeetingLine(meetings, 0)
	if !strings.Contains(line0, "KT") || !strings.Contains(line0, "GT") {
		t.Fatalf("section 0: %q", line0)
	}
	if !strings.Contains(line0, " + ") {
		t.Fatalf("expected KT and GT separated by plus, got %q", line0)
	}
	line1 := formatTeamMeetingLine(meetings, 1)
	if strings.Contains(line1, "KT") {
		t.Fatalf("section 1 should not have KT: %q", line1)
	}
	if !strings.Contains(line1, "GT") {
		t.Fatalf("section 1 should have GT: %q", line1)
	}
}
