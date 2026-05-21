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

func employeeResetPasswordRouter(t *testing.T, db *sqlite.DB, auth *authsvc.Service) http.Handler {
	t.Helper()
	leitung := []string{string(model.RoleLeitung), string(model.RoleSuperadmin)}
	eh := &EmployeeHandler{
		Users: sqlite.NewUserStore(db),
		Auth:  auth,
	}
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(apimw.AuthJWT(auth))
			r.Use(apimw.RequireRole(leitung...))
			r.Post("/employees/{id}/reset-password", eh.ResetPassword)
		})
	})
	return r
}

func TestEmployeeResetPassword_LeitungToUser_OK(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-secret-reset-pw-test", 1)
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	hashTarget, _ := auth.HashPassword("old-secret")
	if err := users.Create(ctx, &model.User{
		Username: "emp", PasswordHash: hashTarget, DisplayName: "Employee",
		Role: model.RoleUser, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	hashLeitung, _ := auth.HashPassword("leitung-pw")
	if err := users.Create(ctx, &model.User{
		Username: "boss", PasswordHash: hashLeitung, DisplayName: "Boss",
		Role: model.RoleLeitung, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	leitungUser, err := users.GetByUsername(ctx, "boss")
	if err != nil {
		t.Fatal(err)
	}
	target, err := users.GetByUsername(ctx, "emp")
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueToken(leitungUser.ID, leitungUser.Username, string(leitungUser.Role))
	if err != nil {
		t.Fatal(err)
	}

	router := employeeResetPasswordRouter(t, db, auth)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/employees/"+strconv.Itoa(target.ID)+"/reset-password", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		TemporaryPassword string     `json:"temporary_password"`
		User              model.User `json:"user"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.TemporaryPassword) < 8 {
		t.Fatalf("unexpected temp password: %q", resp.TemporaryPassword)
	}
	if resp.User.ID != target.ID {
		t.Fatalf("user id: %+v", resp.User)
	}

	loginPayload, err := json.Marshal(map[string]string{
		"username": "emp",
		"password": resp.TemporaryPassword,
	})
	if err != nil {
		t.Fatal(err)
	}
	ah := &AuthHandler{Users: users, Auth: auth}
	loginReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(loginPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	ah.Login(loginRR, loginReq)
	if loginRR.Code != http.StatusOK {
		t.Fatalf("login with new password: want 200, got %d: %s", loginRR.Code, loginRR.Body.String())
	}
}

func TestEmployeeResetPassword_LeitungToSuperadmin_Forbidden(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-secret-reset-pw-test-2", 1)
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	hashSA, _ := auth.HashPassword("sa-pw")
	if err := users.Create(ctx, &model.User{
		Username: "root", PasswordHash: hashSA, DisplayName: "Root",
		Role: model.RoleSuperadmin, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	hashLeitung, _ := auth.HashPassword("leitung-pw")
	if err := users.Create(ctx, &model.User{
		Username: "boss2", PasswordHash: hashLeitung, DisplayName: "Boss",
		Role: model.RoleLeitung, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	leitungUser, _ := users.GetByUsername(ctx, "boss2")
	saUser, _ := users.GetByUsername(ctx, "root")
	token, err := auth.IssueToken(leitungUser.ID, leitungUser.Username, string(leitungUser.Role))
	if err != nil {
		t.Fatal(err)
	}

	router := employeeResetPasswordRouter(t, db, auth)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/employees/"+strconv.Itoa(saUser.ID)+"/reset-password", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestEmployeeResetPassword_NoToken_Unauthorized(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-secret-reset-pw-test-3", 1)
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	hash, _ := auth.HashPassword("x")
	if err := users.Create(ctx, &model.User{
		Username: "u", PasswordHash: hash, DisplayName: "U",
		Role: model.RoleUser, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	u, _ := users.GetByUsername(ctx, "u")

	router := employeeResetPasswordRouter(t, db, auth)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/employees/"+strconv.Itoa(u.ID)+"/reset-password", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d: %s", rr.Code, rr.Body.String())
	}
}
