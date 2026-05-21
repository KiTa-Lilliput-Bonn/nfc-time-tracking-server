package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	apimw "nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/model"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	"nfc-time-tracking-server/internal/service/stampspoll"
	"nfc-time-tracking-server/internal/store/sqlite"
)

// Lan health response shape (subset for test decode).
type lanHealthTestBody struct {
	Mode      string                      `json:"mode"`
	Reachable bool                        `json:"reachable"`
	Targets   []stampspoll.LanTargetHealth `json:"targets"`
}

func TestAndroidLanHealth_LeitungGetsJSON(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-lan-health-test", 1)
	hash, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	if err := users.Create(ctx, &model.User{
		Username: "chef", PasswordHash: hash, DisplayName: "Chef",
		Role: model.RoleLeitung, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	u, err := users.GetByUsername(ctx, "chef")
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueToken(u.ID, u.Username, string(u.Role))
	if err != nil {
		t.Fatal(err)
	}

	settings := sqlite.NewSettingsStore(db)
	svc := stampspoll.NewService(settings, sqlite.NewApiPairedClientStore(db), sqlite.NewPunchStore(db), sqlite.NewWorkPeriodStore(db), sqlite.NewNFCTagStore(db), sqlite.NewCompensationDayClaimStore(db), sqlite.NewUserStore(db), nil)
	h := &AndroidLanHealthHandler{Stamps: svc}

	leitung := []string{string(model.RoleLeitung), string(model.RoleSuperadmin)}
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(apimw.AuthJWT(auth))
			r.Use(apimw.RequireRole(leitung...))
			r.Get("/android-lan/health-status", h.Get)
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/android-lan/health-status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", rr.Code, rr.Body.String())
	}
	var body lanHealthTestBody
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Mode != "disabled" {
		t.Fatalf("want disabled without LAN config, got %+v", body)
	}
	if body.Targets == nil {
		t.Fatal("expected targets array in JSON")
	}
}
