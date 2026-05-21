package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nfc-time-tracking-server/internal/model"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestNewRouter_LoginRateLimitedByIP(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("router-rate-test-secret", 1)
	hash, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	users := sqlite.NewUserStore(db)
	if err := users.Create(context.Background(), &model.User{
		Username: "u1", PasswordHash: hash, DisplayName: "U",
		Role: model.RoleUser, Active: true,
	}); err != nil {
		t.Fatal(err)
	}

	h := NewRouter(Deps{UserStore: users, Auth: auth})

	body := `{"username":"nobody","password":"wrong"}`
	const clientIP = "192.0.2.10"
	for i := 0; i < loginRateLimitPerMinute; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = clientIP + ":5555"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d: unexpected 429 before limit", i+1)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = clientIP + ":5556"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 on request over limit, got %d body=%q", rr.Code, rr.Body.String())
	}
}

func TestNewRouter_PairRegisterRateLimitedByIP(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	h := NewRouter(Deps{
		UserStore:          sqlite.NewUserStore(db),
		Auth:               authsvc.New("router-pair-rate-test", 1),
		ApiPairedClients:   sqlite.NewApiPairedClientStore(db),
		ApiPairingSessions: sqlite.NewApiPairingSessionStore(db),
		Settings:           sqlite.NewSettingsStore(db),
	})

	body := `{"host":"10.0.0.1","port":8787}`
	const clientIP = "192.0.2.20"
	for i := 0; i < pairRegisterRateLimitPerMinute; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/device/pair/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer invalid-token")
		req.RemoteAddr = clientIP + ":5555"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d: unexpected 429 before limit", i+1)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/device/pair/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer invalid-token")
	req.RemoteAddr = clientIP + ":5556"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 on request over limit, got %d body=%q", rr.Code, rr.Body.String())
	}
}
