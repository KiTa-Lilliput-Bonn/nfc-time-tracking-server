package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	apimw "nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/model"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func teamOverviewRouter(t *testing.T, db *sqlite.DB, auth *authsvc.Service) http.Handler {
	t.Helper()
	leitung := []string{string(model.RoleLeitung), string(model.RoleSuperadmin)}

	dh := &DashboardHandler{
		Users:       sqlite.NewUserStore(db),
		WorkPeriods: sqlite.NewWorkPeriodStore(db),
		Corrections: sqlite.NewCorrectionStore(db),
		Absences:    sqlite.NewAbsenceStore(db),
		Holidays:    sqlite.NewHolidayStore(db),
		Closures:    sqlite.NewClosureDayStore(db),
		WeeklyHours: sqlite.NewWeeklyHoursStore(db),
		Settings:    sqlite.NewSettingsStore(db),
		VacationEnt: sqlite.NewVacationEntitlementStore(db),
		Schedules:   sqlite.NewScheduleStore(db),
	}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(apimw.AuthJWT(auth))
			r.Use(apimw.RequireRole(leitung...))
			r.Get("/dashboard/team-overview", dh.TeamOverview)
		})
	})
	return r
}

func TestTeamOverview_RequiresLeitung(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("test-secret-key-for-jwt-team-overview", 1)
	hash, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	if err := users.Create(ctx, &model.User{
		Username: "plainuser", PasswordHash: hash, DisplayName: "Plain",
		Role: model.RoleUser, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	u, err := users.GetByUsername(ctx, "plainuser")
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueToken(u.ID, u.Username, string(u.Role))
	if err != nil {
		t.Fatal(err)
	}

	router := teamOverviewRouter(t, db, auth)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/team-overview", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403 for non-leitung user, got %d: %s", rr.Code, rr.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/team-overview", nil)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without token, got %d: %s", rr2.Code, rr2.Body.String())
	}
}

func TestTeamOverview_ResponseShape(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("test-secret-key-for-jwt-team-overview", 1)
	hash, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	if err := users.Create(ctx, &model.User{
		Username: "lead", PasswordHash: hash, DisplayName: "Lead Person",
		Role: model.RoleLeitung, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(ctx, &model.User{
		Username: "emp", PasswordHash: hash, DisplayName: "Employee One",
		Role: model.RoleUser, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	lead, err := users.GetByUsername(ctx, "lead")
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueToken(lead.ID, lead.Username, string(lead.Role))
	if err != nil {
		t.Fatal(err)
	}

	router := teamOverviewRouter(t, db, auth)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/team-overview?vacation_year=2026", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var envelope struct {
		AsOf         string                   `json:"as_of"`
		VacationYear int                      `json:"vacation_year"`
		Rows         []map[string]interface{} `json:"rows"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if _, err := time.Parse("2006-01-02", envelope.AsOf); err != nil {
		t.Fatalf("as_of: invalid date %q", envelope.AsOf)
	}
	if envelope.VacationYear != 2026 {
		t.Fatalf("vacation_year: want 2026, got %d", envelope.VacationYear)
	}
	if len(envelope.Rows) < 1 {
		t.Fatalf("want at least one row, got %d", len(envelope.Rows))
	}
	row := envelope.Rows[0]
	for _, k := range []string{
		"id", "display_name", "hours_balance", "vacation_planned", "vacation_free",
		"vacation_remaining_total", "vacation_carryover", "vacation_entitlement", "vacation_taken",
		"vacation_opening_days", "compensation_day_claims_open",
	} {
		if _, ok := row[k]; !ok {
			t.Fatalf("row missing key %q", k)
		}
	}
}

func TestTeamOverview_InvalidVacationYear(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("test-secret-key-for-jwt-team-overview", 1)
	hash, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	if err := users.Create(ctx, &model.User{
		Username: "lead", PasswordHash: hash, DisplayName: "Lead",
		Role: model.RoleLeitung, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	lead, err := users.GetByUsername(ctx, "lead")
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueToken(lead.ID, lead.Username, string(lead.Role))
	if err != nil {
		t.Fatal(err)
	}

	router := teamOverviewRouter(t, db, auth)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/team-overview?vacation_year=10000", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid vacation_year, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestTeamOverview_SuperadminOK(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("test-secret-key-for-jwt-team-overview", 1)
	hash, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	if err := users.Create(ctx, &model.User{
		Username: "admin", PasswordHash: hash, DisplayName: "Super",
		Role: model.RoleSuperadmin, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(ctx, &model.User{
		Username: "emp", PasswordHash: hash, DisplayName: "Employee",
		Role: model.RoleUser, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	sa, err := users.GetByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueToken(sa.ID, sa.Username, string(sa.Role))
	if err != nil {
		t.Fatal(err)
	}

	router := teamOverviewRouter(t, db, auth)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/team-overview?vacation_year=2026", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 for superadmin, got %d: %s", rr.Code, rr.Body.String())
	}
}
