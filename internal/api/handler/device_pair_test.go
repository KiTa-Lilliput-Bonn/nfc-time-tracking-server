package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"nfc-time-tracking-server/internal/bootstrap"
	"nfc-time-tracking-server/internal/service/apipairing"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestDevicePair_PostRegister_happyPath(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	clients := sqlite.NewApiPairedClientStore(db)
	sessions := sqlite.NewApiPairingSessionStore(db)
	settings := sqlite.NewSettingsStore(db)

	id, err := apipairing.NewID()
	if err != nil {
		t.Fatal(err)
	}
	c, err := apipairing.BuildClient(id, "phone", "perm-secret-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if err := clients.Insert(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := settings.Set(ctx, "android_lan_targets", "[]"); err != nil {
		t.Fatal(err)
	}

	sessionSvc := apipairing.NewSessionService(sessions)
	token, err := sessionSvc.CreatePairingSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}

	h := &DevicePairHandler{
		Sessions: sessionSvc,
		Clients:  clients,
		Settings: settings,
	}

	r := chi.NewRouter()
	r.Post("/api/v1/device/pair/register", h.PostRegister)

	body := `{"host":"10.0.0.42","port":8787}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/device/pair/register", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp devicePairRegisterResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ClientID != id || resp.Secret != "perm-secret-xyz" || resp.Label != "phone" {
		t.Fatalf("unexpected resp: %+v", resp)
	}

	targets, err := bootstrap.LanTargetsFromSettings(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Host != "10.0.0.42" || targets[0].Port != 8787 {
		t.Fatalf("targets: %+v", targets)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/device/pair/register", strings.NewReader(body))
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusConflict {
		t.Fatalf("reuse token: want 409, got %d", rr2.Code)
	}
}

func TestDevicePair_PostRegister_invalidHost(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	clients := sqlite.NewApiPairedClientStore(db)
	sessions := sqlite.NewApiPairingSessionStore(db)
	settings := sqlite.NewSettingsStore(db)

	id, _ := apipairing.NewID()
	c, _ := apipairing.BuildClient(id, "", "sec")
	_ = clients.Insert(ctx, c)
	_ = settings.Set(ctx, "android_lan_targets", "[]")

	sessionSvc := apipairing.NewSessionService(sessions)
	token, _ := sessionSvc.CreatePairingSession(ctx, id)

	h := &DevicePairHandler{Sessions: sessionSvc, Clients: clients, Settings: settings}
	r := chi.NewRouter()
	r.Post("/register", h.PostRegister)

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"host":"8.8.8.8","port":8787}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for public IP, got %d: %s", rr.Code, rr.Body.String())
	}
}
