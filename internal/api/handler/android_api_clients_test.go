package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	apimw "nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/config"
	"nfc-time-tracking-server/internal/model"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	"nfc-time-tracking-server/internal/service/apipairing"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestAndroidAPI_DeviceStampsBearer(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	cs := sqlite.NewApiPairedClientStore(db)
	id, err := apipairing.NewID()
	if err != nil {
		t.Fatal(err)
	}
	c, err := apipairing.BuildClient(id, "t", "my-shared-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.Insert(ctx, c); err != nil {
		t.Fatal(err)
	}

	svc := apipairing.New(cs)
	dsh := &DeviceStampsHandler{}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(apimw.BearerPairingAuth(svc))
		r.Get("/device/v1/stamps", dsh.Stamps)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/device/v1/stamps", nil)
	req.Header.Set("Authorization", "Bearer my-shared-secret")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var parsed []interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/device/v1/stamps", nil)
	req2.Header.Set("Authorization", "Bearer wrong")
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr2.Code)
	}
}

func TestAndroidAPI_SuperadminClients(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-secret-android-api-test", 1)
	hash, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	if err := users.Create(ctx, &model.User{
		Username: "sa", PasswordHash: hash, DisplayName: "SA",
		Role: model.RoleSuperadmin, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	u, err := users.GetByUsername(ctx, "sa")
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueToken(u.ID, u.Username, string(u.Role))
	if err != nil {
		t.Fatal(err)
	}

	cs := sqlite.NewApiPairedClientStore(db)
	ps := sqlite.NewApiPairingSessionStore(db)
	ah := &AndroidAPIClientsHandler{
		Clients:  cs,
		Sessions: apipairing.NewSessionService(ps),
		Server:   config.ServerConfig{Host: "0.0.0.0", Port: 8080},
	}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(apimw.AuthJWT(auth))
			r.Use(apimw.RequireRole(string(model.RoleSuperadmin)))
			r.Post("/android-api/clients/generate", ah.PostGenerate)
			r.Get("/android-api/clients", ah.List)
			r.Delete("/android-api/clients/{id}", ah.Delete)
		})
	})

	body := strings.NewReader(`{"label":"phone"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/android-api/clients/generate", body)
	req.Host = "192.168.1.10:8080"
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var wrap struct {
		Client         model.ApiPairedClient `json:"client"`
		PairingToken   string                `json:"pairing_token"`
		PairingBaseURL string                `json:"pairing_base_url"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Client.Secret == "" || wrap.Client.Label != "phone" {
		t.Fatalf("unexpected client: %+v", wrap.Client)
	}
	if wrap.PairingToken == "" {
		t.Fatal("expected pairing_token in generate response")
	}
	if wrap.PairingBaseURL != "http://192.168.1.10:8080" {
		t.Fatalf("pairing_base_url: got %q", wrap.PairingBaseURL)
	}
	secret := wrap.Client.Secret

	reqL := httptest.NewRequest(http.MethodGet, "/api/v1/android-api/clients", nil)
	reqL.Header.Set("Authorization", "Bearer "+token)
	rrL := httptest.NewRecorder()
	r.ServeHTTP(rrL, reqL)
	if rrL.Code != http.StatusOK {
		t.Fatalf("want 200 list, got %d", rrL.Code)
	}

	svc := apipairing.New(cs)
	stampsRouter := chi.NewRouter()
	stampsRouter.Route("/api/v1", func(r chi.Router) {
		r.Use(apimw.BearerPairingAuth(svc))
		r.Get("/device/v1/stamps", (&DeviceStampsHandler{}).Stamps)
	})
	reqBefore := httptest.NewRequest(http.MethodGet, "/api/v1/device/v1/stamps", nil)
	reqBefore.Header.Set("Authorization", "Bearer "+secret)
	rrBefore := httptest.NewRecorder()
	stampsRouter.ServeHTTP(rrBefore, reqBefore)
	if rrBefore.Code != http.StatusOK {
		t.Fatalf("before delete want stamps 200, got %d: %s", rrBefore.Code, rrBefore.Body.String())
	}

	route := "/api/v1/android-api/clients/" + wrap.Client.ID
	reqR := httptest.NewRequest(http.MethodDelete, route, nil)
	reqR.Header.Set("Authorization", "Bearer "+token)
	rrR := httptest.NewRecorder()
	r.ServeHTTP(rrR, reqR)
	if rrR.Code != http.StatusNoContent {
		t.Fatalf("want delete 204, got %d: %s", rrR.Code, rrR.Body.String())
	}

	reqS := httptest.NewRequest(http.MethodGet, "/api/v1/device/v1/stamps", nil)
	reqS.Header.Set("Authorization", "Bearer "+secret)
	rrS := httptest.NewRecorder()
	stampsRouter.ServeHTTP(rrS, reqS)
	if rrS.Code != http.StatusUnauthorized {
		t.Fatalf("after delete want 401, got %d", rrS.Code)
	}
}
