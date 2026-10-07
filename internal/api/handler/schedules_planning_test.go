package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestPlanningWeek(t *testing.T) {
	db := openHandlerTestDB(t)
	ctx := context.Background()
	us := sqlite.NewUserStore(db)
	anna := &model.User{Username: "anna", PasswordHash: "x", DisplayName: "Anna", Role: model.RoleUser, Active: true}
	ben := &model.User{Username: "ben", PasswordHash: "x", DisplayName: "Ben", Role: model.RoleUser, Active: true}
	for _, u := range []*model.User{anna, ben} {
		if err := us.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	whs := sqlite.NewWeeklyHoursStore(db)
	// Gültig ab Mittwoch der Vorwoche bzw. erst ab Dienstag der Planwoche: Stand Montag zählt.
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: anna.ID, HoursPerWeek: 30, ValidFrom: "2026-09-30"}); err != nil {
		t.Fatal(err)
	}
	if err := whs.Set(ctx, &model.WeeklyHours{UserID: anna.ID, HoursPerWeek: 35, ValidFrom: "2026-10-06"}); err != nil {
		t.Fatal(err)
	}
	fnw := sqlite.NewFixedNonWorkWeekdaysStore(db)
	if err := fnw.Set(ctx, &model.FixedNonWorkWeekdays{UserID: anna.ID, Weekdays: []int{5}, ValidFrom: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	abs := sqlite.NewAbsenceStore(db)
	for _, a := range []*model.Absence{
		{UserID: ben.ID, AbsenceDate: "2026-10-06", AbsenceType: model.AbsenceSick, CreatedBy: ben.ID},
		{UserID: ben.ID, AbsenceDate: "2026-10-07", AbsenceType: model.AbsenceVacation, HalfDay: true, CreatedBy: ben.ID},
		{UserID: ben.ID, AbsenceDate: "2026-10-12", AbsenceType: model.AbsenceSick, CreatedBy: ben.ID},
	} {
		if err := abs.Create(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	clo := sqlite.NewClosureDayStore(db)
	if err := clo.Create(ctx, &model.ClosureDay{ClosureDate: "2026-10-09", Name: "Konzeptionstag", CreatedBy: anna.ID}); err != nil {
		t.Fatal(err)
	}
	if err := clo.Create(ctx, &model.ClosureDay{ClosureDate: "2026-10-16", Name: "Andere Woche", CreatedBy: anna.ID}); err != nil {
		t.Fatal(err)
	}
	st := sqlite.NewSettingsStore(db)
	if err := st.Set(ctx, "break_rules", `[{"min_work_hours":6,"break_minutes":30}]`); err != nil {
		t.Fatal(err)
	}

	h := &ScheduleHandler{
		Users: us, Absences: abs, Closures: clo, WeeklyHours: whs,
		FixedNonWorkWeekdays: fnw, Settings: st,
	}
	req := httptest.NewRequest(http.MethodGet, "/schedules/planning?year=2026&week=41", nil)
	rr := httptest.NewRecorder()
	h.PlanningWeek(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var out struct {
		From        string             `json:"from"`
		To          string             `json:"to"`
		Users       []planningUser     `json:"users"`
		Absences    []model.Absence    `json:"absences"`
		ClosureDays []model.ClosureDay `json:"closure_days"`
		BreakRules  []model.BreakRule  `json:"break_rules"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.From != "2026-10-05" || out.To != "2026-10-09" {
		t.Fatalf("range %s–%s", out.From, out.To)
	}
	byID := map[int]planningUser{}
	for _, u := range out.Users {
		byID[u.UserID] = u
	}
	if got := byID[anna.ID]; got.HoursPerWeek != 30 || len(got.FixedNonWorkWeekdays) != 1 || got.FixedNonWorkWeekdays[0] != 5 {
		t.Fatalf("anna %+v", got)
	}
	if got := byID[ben.ID]; got.HoursPerWeek != 0 || got.FixedNonWorkWeekdays == nil {
		t.Fatalf("ben %+v", got)
	}
	if len(out.Absences) != 2 {
		t.Fatalf("absences in week: %+v", out.Absences)
	}
	if len(out.ClosureDays) != 1 || out.ClosureDays[0].ClosureDate != "2026-10-09" {
		t.Fatalf("closures %+v", out.ClosureDays)
	}
	if len(out.BreakRules) != 1 || out.BreakRules[0].BreakMinutes != 30 {
		t.Fatalf("break rules %+v", out.BreakRules)
	}
}

func TestPlanningWeek_BadParams(t *testing.T) {
	h := &ScheduleHandler{}
	rr := httptest.NewRecorder()
	h.PlanningWeek(rr, httptest.NewRequest(http.MethodGet, "/schedules/planning?year=2026&week=60", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rr.Code)
	}
}
