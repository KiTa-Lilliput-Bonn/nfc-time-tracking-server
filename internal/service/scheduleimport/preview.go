package scheduleimport

import (
	"context"
	"strings"
)

// PastPreviewReport ist die JSON-Antwort von POST /schedules/preview-excel-import (ohne DB-Schreibungen).
type PastPreviewReport struct {
	PastCellsSkipped        int `json:"past_cells_skipped"`
	PastWeekNotesSkipped    int `json:"past_week_notes_skipped"`
	PastTeamMeetingsSkipped int `json:"past_team_meetings_skipped"`
}

// PreviewPastImport parst die Datei und zählt Inhalte, die beim Import nur ab heute ausgelassen würden.
func PreviewPastImport(ctx context.Context, deps Deps, file []byte, todayLocal string) (PastPreviewReport, error) {
	parsed, err := ParseXLSX(file)
	if err != nil {
		return PastPreviewReport{}, err
	}
	return PreviewPastImportFromParsed(ctx, deps, parsed, todayLocal), nil
}

// PreviewPastImportFromParsed wendet dieselbe Zählung wie PreviewPastImport auf bereits geparste Daten an (u. a. für Tests).
func PreviewPastImportFromParsed(ctx context.Context, deps Deps, parsed *ParsedSheet, todayLocal string) PastPreviewReport {
	cells, weekNotes, teamMeetings := countPastSkippedIfFutureOnly(ctx, deps, parsed, todayLocal)
	return PastPreviewReport{
		PastCellsSkipped:        cells,
		PastWeekNotesSkipped:    weekNotes,
		PastTeamMeetingsSkipped: teamMeetings,
	}
}

func countPastSkippedIfFutureOnly(ctx context.Context, deps Deps, parsed *ParsedSheet, todayLocal string) (cells, weekNotes, teamMeetings int) {
	if parsed == nil {
		return 0, 0, 0
	}
	for _, w := range parsed.Weeks {
		skip := mergeHolidaySkip(ctx, deps.Holidays, w)
		if _, countPast := weekNotesInImportScope(w, todayLocal, ImportScopeFuture); countPast {
			weekNotes++
		}
		for _, row := range w.Rows {
			for col := 0; col < 5; col++ {
				if skip[col] {
					continue
				}
				date := w.Dates[col]
				if date == "" || date >= todayLocal {
					continue
				}
				if strings.TrimSpace(row.Cells[col]) != "" {
					cells++
				}
			}
		}
		monday := w.Dates[0]
		if monday != "" && !skip[0] && monday < todayLocal && weekHasImportableTeamMeetingContent(w) {
			teamMeetings++
		}
	}
	return cells, weekNotes, teamMeetings
}
