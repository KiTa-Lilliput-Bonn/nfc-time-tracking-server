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

func scheduleBoundRouter(t *testing.T, db *sqlite.DB, auth *authsvc.Service) http.Handler {
	t.Helper()
	leitung := []string{string(model.RoleLeitung), string(model.RoleSuperadmin)}
	eh := &EmployeeHandler{
		Users:         sqlite.NewUserStore(db),
		ScheduleBound: sqlite.NewScheduleBoundStore(db),
	}
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(apimw.AuthJWT(auth))
			r.Use(apimw.RequireRole(leitung...))
			r.Get("/employees/{id}/schedule-bound", eh.GetScheduleBound)
			r.Put("/employees/{id}/schedule-bound", eh.PutScheduleBound)
			r.Delete("/employees/{id}/schedule-bound/{sbId}", eh.DeleteScheduleBound)
		})
	})
	return r
}

func TestPutAndGetScheduleBound(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-sb-crud", 1)
	token, targetID := seedActorAndTarget(t, db, auth, model.RoleLeitung, "boss-sb")

	router := scheduleBoundRouter(t, db, auth)
	body, _ := json.Marshal(map[string]interface{}{
		"schedule_bound": false,
		"valid_from":     "2024-06-01",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/employees/"+strconv.Itoa(targetID)+"/schedule-bound", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/employees/"+strconv.Itoa(targetID)+"/schedule-bound", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ScheduleBound []scheduleBoundResponse `json:"schedule_bound"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.ScheduleBound) != 1 {
		t.Fatalf("want 1 row, got %d", len(resp.ScheduleBound))
	}
	if resp.ScheduleBound[0].ScheduleBound {
		t.Fatal("expected schedule_bound false")
	}
}

func TestDeleteScheduleBound_LeitungLockedAfter24h(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-sb-lock", 1)
	token, targetID := seedActorAndTarget(t, db, auth, model.RoleLeitung, "boss-sb-lock")

	ctx := context.Background()
	sb := sqlite.NewScheduleBoundStore(db)
	row := &model.ScheduleBoundSetting{UserID: targetID, ScheduleBound: false, ValidFrom: "2020-01-01"}
	if err := sb.Set(ctx, row); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx,
		`UPDATE user_schedule_bound SET created_at = datetime('now', '-25 hours') WHERE id = ?`, row.ID); err != nil {
		t.Fatal(err)
	}

	router := scheduleBoundRouter(t, db, auth)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/employees/"+strconv.Itoa(targetID)+"/schedule-bound/"+strconv.Itoa(row.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", rr.Code, rr.Body.String())
	}
}
