package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	apimw "nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/model"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestClosureDayCreate_SyncsVacationExceptFixedFree(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("test-secret-closure-sync", 1)
	hash, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	us := sqlite.NewUserStore(db)
	ctx := context.Background()

	if err := us.Create(ctx, &model.User{
		Username: "lead", PasswordHash: hash, DisplayName: "L",
		Role: model.RoleLeitung, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	lead, err := us.GetByUsername(ctx, "lead")
	if err != nil {
		t.Fatal(err)
	}

	if err := us.Create(ctx, &model.User{
		Username: "work", PasswordHash: hash, DisplayName: "W",
		Role: model.RoleUser, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	userWork, err := us.GetByUsername(ctx, "work")
	if err != nil {
		t.Fatal(err)
	}

	if err := us.Create(ctx, &model.User{
		Username: "wedfree", PasswordHash: hash, DisplayName: "WF",
		Role: model.RoleUser, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	userFixedWed, err := us.GetByUsername(ctx, "wedfree")
	if err != nil {
		t.Fatal(err)
	}
	fnw := sqlite.NewFixedNonWorkWeekdaysStore(db)
	if err := fnw.Set(ctx, &model.FixedNonWorkWeekdays{
		UserID: userFixedWed.ID, Weekdays: []int{int(time.Wednesday)}, ValidFrom: "2000-01-01",
	}); err != nil {
		t.Fatal(err)
	}

	token, err := auth.IssueToken(lead.ID, lead.Username, string(lead.Role))
	if err != nil {
		t.Fatal(err)
	}

	ch := &ClosureHandler{
		Closures:             sqlite.NewClosureDayStore(db),
		Holidays:             sqlite.NewHolidayStore(db),
		Users:                us,
		FixedNonWorkWeekdays: fnw,
		Absences:             sqlite.NewAbsenceStore(db),
	}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(apimw.AuthJWT(auth))
			r.Use(apimw.RequireRole(string(model.RoleLeitung), string(model.RoleSuperadmin)))
			r.Post("/closure-days", ch.Create)
		})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/closure-days",
		strings.NewReader(`{"closure_date":"2026-04-08","name":"Betrieb"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}

	as := sqlite.NewAbsenceStore(db)
	aW, err := as.GetForUserDate(ctx, userWork.ID, "2026-04-08")
	if err != nil || aW == nil || aW.AbsenceType != model.AbsenceVacation {
		t.Fatalf("expected vacation for normal worker: %+v err=%v", aW, err)
	}
	aF, err := as.GetForUserDate(ctx, userFixedWed.ID, "2026-04-08")
	if err != nil {
		t.Fatal(err)
	}
	if aF != nil {
		t.Fatalf("expected no absence for fixed-Wednesday user, got %+v", aF)
	}
}

func TestClosureDaysList_AllowsAuthenticatedUser(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("test-secret-closure-list", 1)
	hash, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	us := sqlite.NewUserStore(db)
	ctx := context.Background()
	if err := us.Create(ctx, &model.User{
		Username: "emp", PasswordHash: hash, DisplayName: "E",
		Role: model.RoleUser, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	u, err := us.GetByUsername(ctx, "emp")
	if err != nil {
		t.Fatal(err)
	}
	cs := sqlite.NewClosureDayStore(db)
	if err := cs.Create(ctx, &model.ClosureDay{
		ClosureDate: "2026-05-11", Name: "Test", CreatedBy: u.ID,
	}); err != nil {
		t.Fatal(err)
	}

	token, err := auth.IssueToken(u.ID, u.Username, string(u.Role))
	if err != nil {
		t.Fatal(err)
	}

	ch := &ClosureHandler{Closures: cs, Holidays: sqlite.NewHolidayStore(db), Users: us, Absences: sqlite.NewAbsenceStore(db)}
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(apimw.AuthJWT(auth))
			r.Get("/closure-days", ch.List)
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/closure-days", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var env struct {
		ClosureDays []model.ClosureDay `json:"closure_days"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if len(env.ClosureDays) != 1 || env.ClosureDays[0].Name != "Test" {
		t.Fatalf("unexpected payload: %+v", env.ClosureDays)
	}
}
