package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apimw "nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/model"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestAuth_Login_OK(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("test-secret-key-for-jwt", 1)
	hash, err := auth.HashPassword("correcthorse")
	if err != nil {
		t.Fatal(err)
	}
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	if err := users.Create(ctx, &model.User{
		Username:     "alice",
		PasswordHash: hash,
		DisplayName:  "Alice",
		Role:         model.RoleUser,
		Active:       true,
	}); err != nil {
		t.Fatal(err)
	}

	h := &AuthHandler{Users: users, Auth: auth}
	body := `{"username":"alice","password":"correcthorse"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rr.Code, rr.Body.String())
	}
	var resp loginResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Token == "" || resp.User.Username != "alice" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestAuth_Login_InvalidPassword(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("test-secret-key-for-jwt", 1)
	hash, _ := auth.HashPassword("right")
	users := sqlite.NewUserStore(db)
	_ = users.Create(context.Background(), &model.User{
		Username: "bob", PasswordHash: hash, DisplayName: "B", Role: model.RoleUser, Active: true,
	})

	h := &AuthHandler{Users: users, Auth: auth}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"username":"bob","password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Login(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

func TestAuth_ChangePassword_OK(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("test-secret-key-for-jwt", 1)
	oldHash, _ := auth.HashPassword("oldpass123")
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	u := &model.User{
		Username: "carol", PasswordHash: oldHash, DisplayName: "C",
		Role: model.RoleUser, Active: true, MustChangePassword: true,
	}
	if err := users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueToken(u.ID, u.Username, string(u.Role))
	if err != nil {
		t.Fatal(err)
	}

	h := &AuthHandler{Users: users, Auth: auth}
	payload := map[string]string{
		"current_password": "oldpass123",
		"new_password":     "newpass999",
	}
	buf, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req = req.WithContext(context.WithValue(req.Context(), apimw.CtxUserID, u.ID))

	rr := httptest.NewRecorder()
	h.ChangePassword(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rr.Code, rr.Body.String())
	}

	updated, err := users.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !auth.CheckPassword("newpass999", updated.PasswordHash) {
		t.Fatal("expected new password to be stored")
	}
	if updated.MustChangePassword {
		t.Fatal("expected must_change_password false after change")
	}
}
