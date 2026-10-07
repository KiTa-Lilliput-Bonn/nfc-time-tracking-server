package kibizcalc

import (
	"testing"

	"nfc-time-tracking-server/internal/model"
)

func intp(v int) *int { return &v }

var week = []string{"2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08", "2026-10-09"}

func baseInput() Input {
	return Input{
		Dates:  week,
		Groups: []model.Group{{ID: 1, Name: "Raupen"}},
		Users: []model.User{
			{ID: 10, Role: model.RoleUser, GroupID: intp(1)},
			{ID: 11, Role: model.RoleUser, GroupID: intp(1)},
			{ID: 12, Role: model.RoleLeitung, GroupID: intp(1)},
			{ID: 13, Role: model.RoleUser, GroupID: intp(1)},
			{ID: 14, Role: model.RoleLeitung, GroupID: intp(1)},
			{ID: 15, Role: model.RoleUser, GroupID: intp(1)},
		},
		Qualifications: map[int]model.Qualification{
			10: model.QualificationFachkraft,
			11: model.QualificationErgaenzungskraft,
			12: model.QualificationFachkraft,
			15: model.QualificationHauswirtschaft,
		},
		Rates: []model.KibizRate{
			{GroupForm: model.GroupFormIII, CareHours: 35, Children: 20, LeitungHours: 7, TotalHours: 75, FachkraftMinHours: 50},
		},
		Patterns: []model.ChildPattern{
			{GroupID: 1, ValidFrom: "2026-01-01", Counts: []model.ChildCount{{GroupForm: model.GroupFormIII, CareHours: 35, Count: 20}}},
		},
		Closed:       map[string]bool{},
		FixedNonWork: map[int][]int{},
		BreakRules:   []model.BreakRule{{MinWorkHours: 6, BreakMinutes: 30}},
	}
}

func TestNeedPerChild(t *testing.T) {
	in := baseInput()
	// 18 statt 20 Kinder am Mittwoch
	in.ChildDays = []model.ChildCountDay{{GroupID: 1, Date: "2026-10-07", Counts: []model.ChildCount{{GroupForm: model.GroupFormIII, CareHours: 35, Count: 18}}}}
	g := Compute(in).Groups[0]
	if !g.HasPattern || g.PatternTotal != 20 {
		t.Fatalf("pattern: %+v", g)
	}
	// Normaltag: 50 h / 5 = 600 min Fachkraft, 75 h / 5 = 900 min insgesamt
	if g.Days[0].NeedFachkraft != 600 || g.Days[0].NeedTotal != 900 {
		t.Fatalf("day0 need: %+v", g.Days[0].Totals)
	}
	// 18 Kinder: 18 × 50/20/5 h = 9 h
	if g.Days[2].NeedFachkraft != 540 || !g.Days[2].Adjusted || g.Days[2].ChildrenTotal != 18 {
		t.Fatalf("day2: %+v", g.Days[2])
	}
	if g.Week.NeedFachkraft != 4*600+540 {
		t.Fatalf("week need: %d", g.Week.NeedFachkraft)
	}
}

func TestClosedDayNeedsNothing(t *testing.T) {
	in := baseInput()
	in.Closed["2026-10-06"] = true
	in.Schedules = []model.Schedule{{UserID: 10, ScheduleDate: "2026-10-06", ShiftStart: "08:00", ShiftEnd: "14:00"}}
	g := Compute(in).Groups[0]
	if g.Days[1].Open || g.Days[1].NeedFachkraft != 0 || g.Days[1].PlannedFachkraft != 0 {
		t.Fatalf("closed day: %+v", g.Days[1])
	}
	if g.Week.NeedFachkraft != 2400 {
		t.Fatalf("week: %d", g.Week.NeedFachkraft)
	}
}

func TestPlannedByQualification(t *testing.T) {
	in := baseInput()
	in.Schedules = []model.Schedule{
		{UserID: 10, ScheduleDate: "2026-10-05", ShiftStart: "07:00", ShiftEnd: "15:00"}, // 8 h − 30 = 450
		{UserID: 11, ScheduleDate: "2026-10-05", ShiftStart: "08:00", ShiftEnd: "12:00"}, // 240
		{UserID: 12, ScheduleDate: "2026-10-05", ShiftStart: "08:00", ShiftEnd: "12:00"}, // Leitungskonto als Fachkraft: zählt
		{UserID: 14, ScheduleDate: "2026-10-05", ShiftStart: "08:00", ShiftEnd: "09:00"}, // Leitungskonto ohne Eintrag: Leitung
		{UserID: 15, ScheduleDate: "2026-10-05", ShiftStart: "08:00", ShiftEnd: "11:00"}, // Hauswirtschaft: zählt nicht
		{UserID: 13, ScheduleDate: "2026-10-05", ShiftStart: "08:00", ShiftEnd: "10:00"}, // ohne Qualifikation
		{UserID: 10, ScheduleDate: "2026-10-06", ShiftStart: "07:00", ShiftEnd: "15:00"}, // krank
	}
	in.Absences = []model.Absence{{UserID: 10, AbsenceDate: "2026-10-06", AbsenceType: model.AbsenceSick}}
	in.TeamMeetings = []model.TeamMeeting{{MeetingDate: "2026-10-05", TimeStart: "14:00", TimeEnd: "16:00", UserIDs: []int{10}}}
	res := Compute(in)
	g := res.Groups[0]
	// Teamsitzung 14–15 Uhr in der Schicht: 450 − 60
	if g.Days[0].PlannedFachkraft != 390+240 || g.Days[0].PlannedErgaenzung != 240 || g.Days[0].PlannedOther != 120+60+180 {
		t.Fatalf("day0 planned: %+v", g.Days[0].Totals)
	}
	if g.Days[1].PlannedFachkraft != 0 {
		t.Fatalf("sick day counted: %+v", g.Days[1].Totals)
	}
	if len(res.Unqualified) != 1 || res.Unqualified[0] != 13 {
		t.Fatalf("unqualified: %v", res.Unqualified)
	}

	in.Options = Options{CountTeamMeetings: true}
	g = Compute(in).Groups[0]
	if g.Days[0].PlannedFachkraft != 450+240 {
		t.Fatalf("with options: %+v", g.Days[0].Totals)
	}
}

func TestMissingRateAndFixedFreeDay(t *testing.T) {
	in := baseInput()
	in.Patterns[0].Counts = append(in.Patterns[0].Counts, model.ChildCount{GroupForm: model.GroupFormII, CareHours: 45, Count: 2})
	in.FixedNonWork[10] = []int{1}
	in.Schedules = []model.Schedule{{UserID: 10, ScheduleDate: "2026-10-05", ShiftStart: "08:00", ShiftEnd: "12:00"}}
	g := Compute(in).Groups[0]
	if len(g.MissingRates) != 1 || g.MissingRates[0] != "II/45" {
		t.Fatalf("missing: %v", g.MissingRates)
	}
	if g.Days[0].PlannedFachkraft != 0 {
		t.Fatalf("fixed free day counted")
	}
}
