package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"

	apimw "nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/model"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func weeklyHoursPutRouter(t *testing.T, db *sqlite.DB, auth *authsvc.Service) http.Handler {
	t.Helper()
	leitung := []string{string(model.RoleLeitung), string(model.RoleSuperadmin)}
	eh := &EmployeeHandler{
		Users:       sqlite.NewUserStore(db),
		WeeklyHours: sqlite.NewWeeklyHoursStore(db),
	}
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(apimw.AuthJWT(auth))
			r.Use(apimw.RequireRole(leitung...))
			r.Put("/employees/{id}/weekly-hours", eh.PutWeeklyHours)
		})
	})
	return r
}

func TestPutWeeklyHours_AcceptsZeroHoursPerWeek(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-wh-zero", 1)
	token, targetID := seedActorAndTarget(t, db, auth, model.RoleLeitung, "boss-wh")
	backdateCreatedAt(t, db, targetID, "2025-12-01 08:00:00")

	body, _ := json.Marshal(map[string]interface{}{
		"hours_per_week": 0,
		"valid_from":     "2026-01-01",
	})
	router := weeklyHoursPutRouter(t, db, auth)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/employees/"+strconv.Itoa(targetID)+"/weekly-hours", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	ctx := context.Background()
	wh, err := sqlite.NewWeeklyHoursStore(db).GetForDate(ctx, targetID, "2026-06-01")
	if err != nil {
		t.Fatal(err)
	}
	if wh == nil || wh.HoursPerWeek != 0 {
		t.Fatalf("got %+v want 0 hours_per_week", wh)
	}
}

func backdateCreatedAt(t *testing.T, db *sqlite.DB, userID int, ts string) {
	t.Helper()
	if _, err := db.DB.ExecContext(context.Background(), `UPDATE users SET created_at = ? WHERE id = ?`, ts, userID); err != nil {
		t.Fatal(err)
	}
}

// Wochenstunden und Urlaubsanspruch dürfen nicht vor dem Anlagedatum beginnen; die Zeit davor steckt im Startsaldo.
func TestValidFromBeforeCreationRejected(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-wh-created", 1)
	token, targetID := seedActorAndTarget(t, db, auth, model.RoleLeitung, "boss-created")
	backdateCreatedAt(t, db, targetID, "2026-03-15 08:00:00")

	eh := &EmployeeHandler{
		Users:       sqlite.NewUserStore(db),
		WeeklyHours: sqlite.NewWeeklyHoursStore(db),
		VacationEnt: sqlite.NewVacationEntitlementStore(db),
	}
	r := chi.NewRouter()
	r.Use(apimw.AuthJWT(auth))
	r.Put("/employees/{id}/weekly-hours", eh.PutWeeklyHours)
	r.Put("/employees/{id}/vacation-entitlement", eh.PutVacationEntitlement)

	put := func(path string, body map[string]interface{}) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, "/employees/"+strconv.Itoa(targetID)+path, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	if rr := put("/weekly-hours", map[string]interface{}{"hours_per_week": 30, "valid_from": "2026-03-14"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("weekly hours before creation: want 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr := put("/weekly-hours", map[string]interface{}{"hours_per_week": 30, "valid_from": "2026-03-15"}); rr.Code != http.StatusOK {
		t.Fatalf("weekly hours on creation day: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr := put("/vacation-entitlement", map[string]interface{}{"days_per_year": 30, "valid_from": "2026-01-01"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("vacation before creation: want 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr := put("/vacation-entitlement", map[string]interface{}{"days_per_year": 30, "valid_from": "2026-04-01"}); rr.Code != http.StatusOK {
		t.Fatalf("vacation after creation: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}
