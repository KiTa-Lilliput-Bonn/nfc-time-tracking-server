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
