package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	apimw "nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/model"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func entitlementLockRouter(t *testing.T, db *sqlite.DB, auth *authsvc.Service) http.Handler {
	t.Helper()
	leitung := []string{string(model.RoleLeitung), string(model.RoleSuperadmin)}
	eh := &EmployeeHandler{
		Users:       sqlite.NewUserStore(db),
		WeeklyHours: sqlite.NewWeeklyHoursStore(db),
		VacationEnt: sqlite.NewVacationEntitlementStore(db),
	}
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(apimw.AuthJWT(auth))
			r.Use(apimw.RequireRole(leitung...))
			r.Delete("/employees/{id}/weekly-hours/{whId}", eh.DeleteWeeklyHours)
			r.Delete("/employees/{id}/vacation-entitlement/{veId}", eh.DeleteVacationEntitlement)
		})
	})
	return r
}

func seedActorAndTarget(t *testing.T, db *sqlite.DB, auth *authsvc.Service, actorRole model.Role, actorName string) (token string, targetID int) {
	t.Helper()
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	hash, _ := auth.HashPassword("pw")
	if err := users.Create(ctx, &model.User{
		Username: actorName, PasswordHash: hash, DisplayName: actorName,
		Role: actorRole, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(ctx, &model.User{
		Username: "emp", PasswordHash: hash, DisplayName: "Employee",
		Role: model.RoleUser, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	actor, err := users.GetByUsername(ctx, actorName)
	if err != nil {
		t.Fatal(err)
	}
	target, err := users.GetByUsername(ctx, "emp")
	if err != nil {
		t.Fatal(err)
	}
	token, err = auth.IssueToken(actor.ID, actor.Username, string(actor.Role))
	if err != nil {
		t.Fatal(err)
	}
	return token, target.ID
}

func TestDeleteWeeklyHours_LeitungAfter24h(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-entitlement-lock", 1)
	token, targetID := seedActorAndTarget(t, db, auth, model.RoleLeitung, "boss")

	ctx := context.Background()
	whs := sqlite.NewWeeklyHoursStore(db)
	wh := &model.WeeklyHours{UserID: targetID, HoursPerWeek: 40, ValidFrom: "2020-01-01"}
	if err := whs.Set(ctx, wh); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx,
		`UPDATE weekly_hours SET created_at = datetime('now', '-25 hours') WHERE id = ?`, wh.ID); err != nil {
		t.Fatal(err)
	}

	router := entitlementLockRouter(t, db, auth)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/employees/"+strconv.Itoa(targetID)+"/weekly-hours/"+strconv.Itoa(wh.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteWeeklyHours_SuperadminAfter24h(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-entitlement-lock-sa", 1)
	token, targetID := seedActorAndTarget(t, db, auth, model.RoleSuperadmin, "admin")

	ctx := context.Background()
	whs := sqlite.NewWeeklyHoursStore(db)
	wh := &model.WeeklyHours{UserID: targetID, HoursPerWeek: 40, ValidFrom: "2020-01-01"}
	if err := whs.Set(ctx, wh); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx,
		`UPDATE weekly_hours SET created_at = datetime('now', '-25 hours') WHERE id = ?`, wh.ID); err != nil {
		t.Fatal(err)
	}

	router := entitlementLockRouter(t, db, auth)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/employees/"+strconv.Itoa(targetID)+"/weekly-hours/"+strconv.Itoa(wh.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteVacationEntitlement_LeitungWithin24h(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-entitlement-lock-ok", 1)
	token, targetID := seedActorAndTarget(t, db, auth, model.RoleLeitung, "boss2")

	ctx := context.Background()
	ves := sqlite.NewVacationEntitlementStore(db)
	ve := &model.VacationEntitlement{UserID: targetID, DaysPerYear: 28, ValidFrom: time.Now().UTC().Format("2006-01-02")}
	if err := ves.Set(ctx, ve); err != nil {
		t.Fatal(err)
	}

	router := entitlementLockRouter(t, db, auth)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/employees/"+strconv.Itoa(targetID)+"/vacation-entitlement/"+strconv.Itoa(ve.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rr.Code, rr.Body.String())
	}
}
