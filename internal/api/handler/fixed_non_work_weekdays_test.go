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

func fnwRouter(t *testing.T, db *sqlite.DB, auth *authsvc.Service) http.Handler {
	t.Helper()
	leitung := []string{string(model.RoleLeitung), string(model.RoleSuperadmin)}
	eh := &EmployeeHandler{
		Users:                sqlite.NewUserStore(db),
		FixedNonWorkWeekdays: sqlite.NewFixedNonWorkWeekdaysStore(db),
	}
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(apimw.AuthJWT(auth))
			r.Use(apimw.RequireRole(leitung...))
			r.Get("/employees/{id}/fixed-non-work-weekdays", eh.GetFixedNonWorkWeekdays)
			r.Put("/employees/{id}/fixed-non-work-weekdays", eh.PutFixedNonWorkWeekdays)
			r.Delete("/employees/{id}/fixed-non-work-weekdays/{fnwId}", eh.DeleteFixedNonWorkWeekdays)
		})
	})
	return r
}

func TestPutAndGetFixedNonWorkWeekdays(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-fnw-crud", 1)
	token, targetID := seedActorAndTarget(t, db, auth, model.RoleLeitung, "boss-fnw")

	router := fnwRouter(t, db, auth)
	body, _ := json.Marshal(map[string]interface{}{
		"weekdays":   []int{5},
		"valid_from": "2024-06-01",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/employees/"+strconv.Itoa(targetID)+"/fixed-non-work-weekdays", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/employees/"+strconv.Itoa(targetID)+"/fixed-non-work-weekdays", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		FixedNonWorkWeekdays []fixedNonWorkWeekdaysResponse `json:"fixed_non_work_weekdays"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.FixedNonWorkWeekdays) != 1 {
		t.Fatalf("want 1 row, got %d", len(resp.FixedNonWorkWeekdays))
	}
	if len(resp.FixedNonWorkWeekdays[0].Weekdays) != 1 || resp.FixedNonWorkWeekdays[0].Weekdays[0] != 5 {
		t.Fatalf("unexpected weekdays: %v", resp.FixedNonWorkWeekdays[0].Weekdays)
	}
}

func TestDeleteFixedNonWorkWeekdays_LeitungLockedAfter24h(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-fnw-lock", 1)
	token, targetID := seedActorAndTarget(t, db, auth, model.RoleLeitung, "boss-fnw-lock")

	ctx := context.Background()
	fnw := sqlite.NewFixedNonWorkWeekdaysStore(db)
	row := &model.FixedNonWorkWeekdays{UserID: targetID, Weekdays: []int{5}, ValidFrom: "2020-01-01"}
	if err := fnw.Set(ctx, row); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx,
		`UPDATE fixed_non_work_weekdays SET created_at = datetime('now', '-25 hours') WHERE id = ?`, row.ID); err != nil {
		t.Fatal(err)
	}

	router := fnwRouter(t, db, auth)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/employees/"+strconv.Itoa(targetID)+"/fixed-non-work-weekdays/"+strconv.Itoa(row.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteFixedNonWorkWeekdays_SuperadminAfter24h(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-fnw-lock-sa", 1)
	token, targetID := seedActorAndTarget(t, db, auth, model.RoleSuperadmin, "admin-fnw")

	ctx := context.Background()
	fnw := sqlite.NewFixedNonWorkWeekdaysStore(db)
	row := &model.FixedNonWorkWeekdays{UserID: targetID, Weekdays: []int{5}, ValidFrom: "2020-01-01"}
	if err := fnw.Set(ctx, row); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(ctx,
		`UPDATE fixed_non_work_weekdays SET created_at = datetime('now', '-25 hours') WHERE id = ?`, row.ID); err != nil {
		t.Fatal(err)
	}

	router := fnwRouter(t, db, auth)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/employees/"+strconv.Itoa(targetID)+"/fixed-non-work-weekdays/"+strconv.Itoa(row.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rr.Code, rr.Body.String())
	}
}
