package scheduleimport

import (
	"context"
	"fmt"
	"sort"

	"nfc-time-tracking-server/internal/model"
)

func allUserIDsFromIndex(index map[string]int) []int {
	seen := map[int]struct{}{}
	for _, uid := range index {
		if uid > 0 {
			seen[uid] = struct{}{}
		}
	}
	out := make([]int, 0, len(seen))
	for uid := range seen {
		out = append(out, uid)
	}
	sort.Ints(out)
	return out
}

func filterDefaultTeamMeetingParticipants(uids []int, optOut map[int]struct{}) []int {
	if len(optOut) == 0 || len(uids) == 0 {
		return uids
	}
	out := make([]int, 0, len(uids))
	for _, id := range uids {
		if _, skip := optOut[id]; skip {
			continue
		}
		out = append(out, id)
	}
	return out
}

func resolveSectionUserIDs(rawNames []string, index map[string]int) []int {
	seen := map[int]struct{}{}
	var out []int
	for _, n := range rawNames {
		uid, ok := index[normalizeName(n)]
		if !ok || uid <= 0 {
			continue
		}
		if _, dup := seen[uid]; dup {
			continue
		}
		seen[uid] = struct{}{}
		out = append(out, uid)
	}
	sort.Ints(out)
	return out
}

func buildTeamMondaySectionsReport(w ParsedWeek) []TeamMondaySectionReport {
	monday := w.Dates[0]
	out := make([]TeamMondaySectionReport, 0, len(w.TeamMondaySections))
	for _, sec := range w.TeamMondaySections {
		tr := TeamMondaySectionReport{
			Monday:    monday,
			RawLine:   sec.Line.Raw,
			Employees: append([]string(nil), sec.EmployeeRawNames...),
		}
		switch sec.Line.Kind {
		case TeamMeetingLineNoMeetings:
			tr.NoMeetings = true
		case TeamMeetingLineScheduled:
			if sec.Line.KTStart != "" {
				tr.GroupTeam = &TimeSpan{Start: sec.Line.KTStart, End: sec.Line.KTEnd}
			}
			if sec.Line.GTStart != "" {
				tr.AllTeam = &TimeSpan{Start: sec.Line.GTStart, End: sec.Line.GTEnd}
			}
		}
		out = append(out, tr)
	}
	return out
}

func weekHasImportableTeamMeetingContent(w ParsedWeek) bool {
	for _, sec := range w.TeamMondaySections {
		switch sec.Line.Kind {
		case TeamMeetingLineNoMeetings, TeamMeetingLineUnspecified:
			continue
		case TeamMeetingLineScheduled:
			if sec.Line.KTStart != "" && sec.Line.KTEnd != "" {
				return true
			}
			if sec.Line.GTStart != "" && sec.Line.GTEnd != "" {
				return true
			}
		}
	}
	return false
}

func applyTeamMeetingsForWeek(
	ctx context.Context,
	deps Deps,
	w ParsedWeek,
	index map[string]int,
	teamMeetingOptOut map[int]struct{},
	skip [5]bool,
	todayLocal string,
	scope ImportScope,
	rep *Report,
	wr *WeekReport,
) {
	wr.TeamMondaySections = buildTeamMondaySectionsReport(w)
	for _, tw := range w.TeamMeetingParseWarnings {
		rep.Warnings = append(rep.Warnings, tw)
	}
	if deps.TeamMeetings == nil {
		return
	}
	monday := w.Dates[0]
	if monday == "" {
		return
	}
	if skip[0] {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf(
			"KW %d/%d: Teamsitzungen (Montag) nicht importiert (Feiertag/Freitag-Spalte).", w.ISOWk, w.ISOYear))
		return
	}
	if !teamMeetingsInImportScope(monday, todayLocal, scope) {
		if scope == ImportScopeFuture && monday < todayLocal && weekHasImportableTeamMeetingContent(w) {
			rep.PastTeamMeetingsSkipped++
			rep.Warnings = append(rep.Warnings, fmt.Sprintf(
				"KW %d/%d: Teamsitzungen (Montag) nicht importiert (Datum vor %s).", w.ISOWk, w.ISOYear, todayLocal))
		}
		return
	}

	if err := deps.TeamMeetings.DeleteByWeekAndSource(ctx, w.ISOYear, w.ISOWk, "excel"); err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("KW %d/%d Teamsitzungen löschen: %v", w.ISOWk, w.ISOYear, err))
		return
	}

	for i, sec := range w.TeamMondaySections {
		line := sec.Line
		switch line.Kind {
		case TeamMeetingLineNoMeetings, TeamMeetingLineUnspecified:
			continue
		}
		if line.KTStart != "" && line.KTEnd != "" {
			uids := filterDefaultTeamMeetingParticipants(
				resolveSectionUserIDs(sec.EmployeeRawNames, index), teamMeetingOptOut)
			if len(uids) == 0 {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf(
					"KW %d/%d: KT %s–%s ohne zuordenbare Mitarbeiter (Sektion %d).",
					w.ISOWk, w.ISOYear, line.KTStart, line.KTEnd, i))
				continue
			}
			m := &model.TeamMeeting{
				ISOWeekYear: w.ISOYear, ISOWeek: w.ISOWk, MeetingDate: monday,
				Kind: model.TeamMeetingKindKT, TimeStart: line.KTStart, TimeEnd: line.KTEnd,
				Source: "excel", SectionIndex: i, UserIDs: uids,
			}
			if err := deps.TeamMeetings.CreateWithUsers(ctx, m); err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("KW %d/%d KT-Sitzung: %v", w.ISOWk, w.ISOYear, err))
				continue
			}
			rep.TeamMeetingsCreated++
		}
		if line.GTStart != "" && line.GTEnd != "" {
			uids := filterDefaultTeamMeetingParticipants(allUserIDsFromIndex(index), teamMeetingOptOut)
			if len(uids) == 0 {
				rep.Warnings = append(rep.Warnings, fmt.Sprintf(
					"KW %d/%d: GT %s–%s ohne Mitarbeiter-Index.", w.ISOWk, w.ISOYear, line.GTStart, line.GTEnd))
				continue
			}
			m := &model.TeamMeeting{
				ISOWeekYear: w.ISOYear, ISOWeek: w.ISOWk, MeetingDate: monday,
				Kind: model.TeamMeetingKindGT, TimeStart: line.GTStart, TimeEnd: line.GTEnd,
				Source: "excel", SectionIndex: i, UserIDs: uids,
			}
			if err := deps.TeamMeetings.CreateWithUsers(ctx, m); err != nil {
				rep.Errors = append(rep.Errors, fmt.Sprintf("KW %d/%d GT-Sitzung: %v", w.ISOWk, w.ISOYear, err))
				continue
			}
			rep.TeamMeetingsCreated++
		}
	}
}
