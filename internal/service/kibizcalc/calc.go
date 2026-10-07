// Package kibizcalc rechnet für eine Dienstplan-Woche je Gruppe aus, wie viele Fachkraft- und
// Ergänzungskraftstunden nach KiBiz für die betreuten Kinder nötig sind und wie viele geplant sind.
//
// Nötig: Jedes Kind zählt einzeln mit dem Anteil seiner Gruppenform und Betreuungszeit
// (Stunden der KiBiz-Tabelle ÷ Kinderzahl), verteilt auf die fünf Wochentage. Das ergibt die
// Personalstunden insgesamt und davon mindestens Fachkraftstunden; den Rest dürfen Ergänzungskräfte
// abdecken. Leitungsstunden der Tabelle zählen nicht dazu. An Feiertagen und Schließtagen ist nichts nötig.
// Geplant: Schichten der Stammgruppe nach Kraft, nach Pflichtpause, ohne ganztägig abwesende
// Personen, fix freie Tage, Feier- und Schließtage. Es zählen nur Fach- und Ergänzungskräfte, unabhängig
// von der Kontorolle; Leitungskonten ohne Eintrag gelten als Leitung. Teamsitzungen zählen nur per Schalter.
package kibizcalc

import (
	"fmt"
	"math"
	"sort"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/timecalc"
)

// Options sind die Schalter der Leitung.
type Options struct {
	// CountTeamMeetings: Teamsitzungen innerhalb einer Schicht zählen als Betreuung.
	CountTeamMeetings bool `json:"count_team_meetings"`
}

type Input struct {
	// Dates: Mo–Fr der Woche (YYYY-MM-DD).
	Dates          []string
	Groups         []model.Group
	Users          []model.User
	Qualifications map[int]model.Qualification
	Rates          []model.KibizRate
	Patterns       []model.ChildPattern
	ChildDays      []model.ChildCountDay
	Schedules      []model.Schedule
	Absences       []model.Absence
	// Closed: Feiertage und Schließtage.
	Closed map[string]bool
	// FixedNonWork: fix freie Wochentage je Person (1 = Mo … 5 = Fr).
	FixedNonWork map[int][]int
	TeamMeetings []model.TeamMeeting
	BreakRules   []model.BreakRule
	Options      Options
}

// Totals sind Minuten.
type Totals struct {
	// NeedTotal: Personalstunden insgesamt (Fach- und Ergänzungskräfte).
	NeedTotal int `json:"need_total_min"`
	// NeedFachkraft: davon mindestens durch Fachkräfte.
	NeedFachkraft     int `json:"need_fachkraft_min"`
	PlannedFachkraft  int `json:"planned_fachkraft_min"`
	PlannedErgaenzung int `json:"planned_ergaenzung_min"`
	// PlannedOther: Leitung, Hauswirtschaft, Sonstige oder noch keine Kraft hinterlegt; zählt nicht.
	PlannedOther int `json:"planned_other_min"`
}

type Day struct {
	Date string `json:"date"`
	Open bool   `json:"open"`
	// Children: Kinder an diesem Tag (Abweichung, sonst Muster).
	Children      []model.ChildCount `json:"children"`
	ChildrenTotal int                `json:"children_total"`
	// Adjusted: für diesen Tag weicht die Kinderzahl vom Muster ab.
	Adjusted bool `json:"adjusted"`
	Totals
}

type Group struct {
	GroupID int    `json:"group_id"`
	Name    string `json:"name"`
	// HasPattern: für die Woche ist ein Kinder-Wochenmuster hinterlegt.
	HasPattern bool `json:"has_pattern"`
	// Pattern: das am Montag gültige Muster.
	Pattern      []model.ChildCount `json:"pattern"`
	PatternTotal int                `json:"pattern_total"`
	Days         []Day              `json:"days"`
	Week         Totals             `json:"week"`
	// MissingRates: Kombinationen ohne Zeile in der KiBiz-Tabelle (zählen nicht).
	MissingRates []string `json:"missing_rates"`
}

type Result struct {
	Groups []Group `json:"groups"`
	// Unqualified: Personen mit geplanter Schicht in einer Gruppe, aber ohne hinterlegte Qualifikation.
	Unqualified []int `json:"unqualified_user_ids"`
}

func rateKey(f model.GroupForm, h int) string { return fmt.Sprintf("%s/%d", f, h) }

func clockMinutes(s string) (int, bool) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}

func weekday(date string) int {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0
	}
	return int(t.Weekday())
}

func overlap(a1, a2, b1, b2 int) int {
	lo, hi := max(a1, b1), min(a2, b2)
	if hi <= lo {
		return 0
	}
	return hi - lo
}

// ShiftCareMinutes sind die als Betreuung zählenden Minuten einer Schicht: brutto − Pflichtpause,
// ohne Teamsitzungen (meetings sind die Zeitfenster der Sitzungen dieser Person an diesem Tag).
func ShiftCareMinutes(start, end string, rules []model.BreakRule, meetings [][2]int) int {
	a, ok1 := clockMinutes(start)
	b, ok2 := clockMinutes(end)
	if !ok1 || !ok2 || b <= a {
		return 0
	}
	gross := b - a
	net := gross - timecalc.RequiredBreakMinutes(time.Duration(gross)*time.Minute, rules)
	for _, m := range meetings {
		net -= overlap(a, b, m[0], m[1])
	}
	return max(0, net)
}

func Compute(in Input) Result {
	rates := map[string]model.KibizRate{}
	for _, r := range in.Rates {
		rates[rateKey(r.GroupForm, r.CareHours)] = r
	}
	overrides := map[string][]model.ChildCount{}
	for _, d := range in.ChildDays {
		overrides[fmt.Sprintf("%d_%s", d.GroupID, d.Date)] = d.Counts
	}
	fullDayAbsent := map[string]bool{}
	for _, a := range in.Absences {
		if !a.HalfDay {
			fullDayAbsent[fmt.Sprintf("%d_%s", a.UserID, model.NormCalendarDate(a.AbsenceDate))] = true
		}
	}
	userByID := map[int]model.User{}
	for _, u := range in.Users {
		userByID[u.ID] = u
	}
	meetingsByUserDate := map[string][][2]int{}
	if !in.Options.CountTeamMeetings {
		for _, m := range in.TeamMeetings {
			a, ok1 := clockMinutes(m.TimeStart)
			b, ok2 := clockMinutes(m.TimeEnd)
			if !ok1 || !ok2 || b <= a {
				continue
			}
			for _, uid := range m.UserIDs {
				k := fmt.Sprintf("%d_%s", uid, m.MeetingDate)
				meetingsByUserDate[k] = append(meetingsByUserDate[k], [2]int{a, b})
			}
		}
	}

	type acc struct{ needFK, needTotal float64 }
	groupIdx := map[int]int{}
	out := Result{Groups: make([]Group, 0, len(in.Groups)), Unqualified: []int{}}
	needs := make([][]acc, len(in.Groups))
	for gi, g := range in.Groups {
		groupIdx[g.ID] = gi
		gr := Group{GroupID: g.ID, Name: g.Name, Pattern: []model.ChildCount{}, MissingRates: []string{}}
		if len(in.Dates) > 0 {
			if p := model.ChildPatternForDate(in.Patterns, g.ID, in.Dates[0]); p != nil {
				gr.Pattern = p.Counts
				gr.HasPattern = true
				for _, c := range p.Counts {
					gr.PatternTotal += c.Count
				}
			}
		}
		missing := map[string]bool{}
		needs[gi] = make([]acc, len(in.Dates))
		for di, date := range in.Dates {
			day := Day{Date: date, Open: !in.Closed[date], Children: []model.ChildCount{}}
			counts, adjusted := overrides[fmt.Sprintf("%d_%s", g.ID, date)]
			if !adjusted {
				if p := model.ChildPatternForDate(in.Patterns, g.ID, date); p != nil {
					counts = p.Counts
					gr.HasPattern = true
				}
			}
			if counts != nil {
				day.Children = counts
			}
			day.Adjusted = adjusted
			for _, c := range day.Children {
				day.ChildrenTotal += c.Count
			}
			if day.Open {
				for _, c := range day.Children {
					if c.Count <= 0 {
						continue
					}
					r, ok := rates[rateKey(c.GroupForm, c.CareHours)]
					if !ok || r.Children <= 0 {
						missing[rateKey(c.GroupForm, c.CareHours)] = true
						continue
					}
					// Anteil je Kind und Woche, auf fünf Tage verteilt, in Minuten.
					needs[gi][di].needFK += float64(c.Count) * r.FachkraftMinHours / r.Children / 5 * 60
					needs[gi][di].needTotal += float64(c.Count) * r.TotalHours / r.Children / 5 * 60
				}
			}
			day.NeedFachkraft = int(math.Round(needs[gi][di].needFK))
			day.NeedTotal = int(math.Round(needs[gi][di].needTotal))
			gr.Days = append(gr.Days, day)
		}
		var wFK, wTotal float64
		for _, a := range needs[gi] {
			wFK += a.needFK
			wTotal += a.needTotal
		}
		gr.Week.NeedFachkraft = int(math.Round(wFK))
		gr.Week.NeedTotal = int(math.Round(wTotal))
		for k := range missing {
			gr.MissingRates = append(gr.MissingRates, k)
		}
		sort.Strings(gr.MissingRates)
		out.Groups = append(out.Groups, gr)
	}

	dateIdx := map[string]int{}
	for i, d := range in.Dates {
		dateIdx[d] = i
	}
	unqualified := map[int]bool{}
	for _, s := range in.Schedules {
		u, ok := userByID[s.UserID]
		if !ok || u.GroupID == nil || u.Role == model.RoleSuperadmin {
			continue
		}
		gi, ok := groupIdx[*u.GroupID]
		if !ok {
			continue
		}
		date := model.NormCalendarDate(s.ScheduleDate)
		di, ok := dateIdx[date]
		if !ok || in.Closed[date] || fullDayAbsent[fmt.Sprintf("%d_%s", u.ID, date)] {
			continue
		}
		wd := weekday(date)
		fixedFree := false
		for _, f := range in.FixedNonWork[u.ID] {
			if f == wd {
				fixedFree = true
			}
		}
		if fixedFree {
			continue
		}
		mins := ShiftCareMinutes(s.ShiftStart, s.ShiftEnd, in.BreakRules, meetingsByUserDate[fmt.Sprintf("%d_%s", u.ID, date)])
		if mins <= 0 {
			continue
		}
		day := &out.Groups[gi].Days[di]
		week := &out.Groups[gi].Week
		q, hasQ := in.Qualifications[u.ID]
		if !hasQ && u.Role == model.RoleLeitung {
			q, hasQ = model.QualificationLeitung, true
		}
		switch q {
		case model.QualificationFachkraft:
			day.PlannedFachkraft += mins
			week.PlannedFachkraft += mins
		case model.QualificationErgaenzungskraft:
			day.PlannedErgaenzung += mins
			week.PlannedErgaenzung += mins
		default:
			day.PlannedOther += mins
			week.PlannedOther += mins
			if !hasQ {
				unqualified[u.ID] = true
			}
		}
	}
	for uid := range unqualified {
		out.Unqualified = append(out.Unqualified, uid)
	}
	sort.Ints(out.Unqualified)
	return out
}
