package handler

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"

	apimw "nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/config"
	"nfc-time-tracking-server/internal/model"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	oidcsvc "nfc-time-tracking-server/internal/service/oidc"
	"nfc-time-tracking-server/internal/store/sqlite"
)

// fakeIdP ist ein minimaler OpenID-Connect-Provider für Tests.
type fakeIdP struct {
	srv *httptest.Server
	key *rsa.PrivateKey

	mu        sync.Mutex
	challenge map[string]string // code -> PKCE challenge
	nonce     map[string]string // code -> nonce
	sub       string
	username  string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{key: key, challenge: map[string]string{}, nonce: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                f.srv.URL,
			"authorization_endpoint":                f.srv.URL + "/authorize",
			"token_endpoint":                        f.srv.URL + "/token",
			"jwks_uri":                              f.srv.URL + "/jwks",
			"end_session_endpoint":                  f.srv.URL + "/logout",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "k1",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		code := r.PostForm.Get("code")
		f.mu.Lock()
		ch, nonce := f.challenge[code], f.nonce[code]
		delete(f.challenge, code)
		sub, username := f.sub, f.username
		f.mu.Unlock()
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if ch == "" || base64.RawURLEncoding.EncodeToString(sum[:]) != ch {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss": f.srv.URL, "aud": "nfc-app", "sub": sub, "nonce": nonce,
			"preferred_username": username,
			"iat":                time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix(),
		})
		tok.Header["kid"] = "k1"
		signed, err := tok.SignedString(key)
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "token_type": "Bearer", "expires_in": 300, "id_token": signed,
		})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// authorize simuliert die Anmeldung beim Provider und liefert einen Code.
func (f *fakeIdP) authorize(t *testing.T, authURL, sub, username string) string {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" {
		t.Fatalf("PKCE fehlt: %s", authURL)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	code := "code-" + q.Get("state")[:8]
	f.challenge[code] = q.Get("code_challenge")
	f.nonce[code] = q.Get("nonce")
	f.sub, f.username = sub, username
	return code
}

type oidcEnv struct {
	idp   *fakeIdP
	users *sqlite.UserStore
	auth  *authsvc.Service
	h     *OIDCHandler
}

func newOIDCEnv(t *testing.T, mutate func(*config.OIDCConfig)) *oidcEnv {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	idp := newFakeIdP(t)
	cfg := config.OIDCConfig{
		Enabled: true, Issuer: idp.srv.URL, ClientID: "nfc-app", ClientSecret: "s3cret",
		RedirectURL: "https://zeit.example.de/api/v1/auth/oidc/callback",
	}
	if mutate != nil {
		mutate(&cfg)
	}
	svc, err := oidcsvc.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	users := sqlite.NewUserStore(db)
	auth := authsvc.New("test-secret-key-for-jwt", 1)
	return &oidcEnv{idp: idp, users: users, auth: auth, h: &OIDCHandler{OIDC: svc, Users: users, Auth: auth}}
}

func (e *oidcEnv) createUser(t *testing.T, username string, role model.Role, active bool) *model.User {
	t.Helper()
	u := &model.User{Username: username, DisplayName: username, Role: role, Active: active, PasswordHash: "x"}
	if err := e.users.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	if !active {
		u.Active = false
		if err := e.users.Update(context.Background(), u); err != nil {
			t.Fatal(err)
		}
	}
	return u
}

// login durchläuft Start und Callback und liefert die Weiterleitung des Callbacks.
func (e *oidcEnv) login(t *testing.T, sub, username string) string {
	t.Helper()
	rr := httptest.NewRecorder()
	e.h.Start(rr, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/start", nil))
	if rr.Code != http.StatusFound {
		t.Fatalf("start: %d %s", rr.Code, rr.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == oidcStateCookie {
			cookie = c
		}
	}
	if cookie == nil || !cookie.HttpOnly || !cookie.Secure {
		t.Fatalf("state-Cookie fehlt oder unsicher: %+v", cookie)
	}
	authURL := rr.Header().Get("Location")
	code := e.idp.authorize(t, authURL, sub, username)
	state, _ := url.Parse(authURL)
	cb := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?"+url.Values{
		"state": {state.Query().Get("state")}, "code": {code},
	}.Encode(), nil)
	cb.AddCookie(cookie)
	rr = httptest.NewRecorder()
	e.h.Callback(rr, cb)
	if rr.Code != http.StatusFound {
		t.Fatalf("callback: %d %s", rr.Code, rr.Body.String())
	}
	return rr.Header().Get("Location")
}

func (e *oidcEnv) tokenUser(t *testing.T, loc string) int {
	t.Helper()
	if !strings.HasPrefix(loc, "/login/sso#") {
		t.Fatalf("erwartet Weiterleitung mit Token, bekam %q", loc)
	}
	frag, err := url.ParseQuery(strings.TrimPrefix(loc, "/login/sso#"))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := e.auth.VerifyToken(frag.Get("token"))
	if err != nil {
		t.Fatal(err)
	}
	return claims.UserID
}

func TestOIDC_FirstLoginLinksByUsername(t *testing.T) {
	e := newOIDCEnv(t, nil)
	anna := e.createUser(t, "Anna", model.RoleUser, true)

	// Groß-/Kleinschreibung beim Provider darf abweichen.
	if got := e.tokenUser(t, e.login(t, "sub-anna", "anna")); got != anna.ID {
		t.Fatalf("user %d, want %d", got, anna.ID)
	}
	u, _ := e.users.GetByID(context.Background(), anna.ID)
	if u.SSOSubject != "sub-anna" || !u.SSOLinked {
		t.Fatalf("nicht verknüpft: %+v", u)
	}

	// Danach zählt nur noch die feste SSO-Kennung, nicht mehr der Benutzername.
	if got := e.tokenUser(t, e.login(t, "sub-anna", "anna.neu")); got != anna.ID {
		t.Fatalf("user %d, want %d", got, anna.ID)
	}
}

func TestOIDC_NoMatchingUser(t *testing.T) {
	e := newOIDCEnv(t, nil)
	e.createUser(t, "anna", model.RoleUser, true)
	if loc := e.login(t, "sub-x", "bernd"); loc != "/login?sso_error=no_account" {
		t.Fatalf("got %q", loc)
	}
}

func TestOIDC_AutoLinkDisabled(t *testing.T) {
	off := false
	e := newOIDCEnv(t, func(c *config.OIDCConfig) { c.AutoLink = &off })
	e.createUser(t, "anna", model.RoleUser, true)
	if loc := e.login(t, "sub-anna", "anna"); loc != "/login?sso_error=no_account" {
		t.Fatalf("got %q", loc)
	}
}

func TestOIDC_AlreadyLinkedToOtherAccount(t *testing.T) {
	e := newOIDCEnv(t, nil)
	anna := e.createUser(t, "anna", model.RoleUser, true)
	if err := e.users.SetSSOSubject(context.Background(), anna.ID, "sub-original"); err != nil {
		t.Fatal(err)
	}
	if loc := e.login(t, "sub-fremd", "anna"); loc != "/login?sso_error=conflict" {
		t.Fatalf("got %q", loc)
	}
}

func TestOIDC_InactiveUser(t *testing.T) {
	e := newOIDCEnv(t, nil)
	e.createUser(t, "anna", model.RoleUser, false)
	if loc := e.login(t, "sub-anna", "anna"); loc != "/login?sso_error=inactive" {
		t.Fatalf("got %q", loc)
	}
}

func TestOIDC_CallbackRejectsForeignState(t *testing.T) {
	e := newOIDCEnv(t, nil)
	rr := httptest.NewRecorder()
	e.h.Start(rr, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/start", nil))
	authURL, _ := url.Parse(rr.Header().Get("Location"))
	cb := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?state="+authURL.Query().Get("state")+"&code=x", nil)
	cb.AddCookie(&http.Cookie{Name: oidcStateCookie, Value: "anderer-browser"})
	rr = httptest.NewRecorder()
	e.h.Callback(rr, cb)
	if loc := rr.Header().Get("Location"); loc != "/login?sso_error=failed" {
		t.Fatalf("got %q", loc)
	}
}

func TestOIDC_LogoutUsesEndSessionEndpoint(t *testing.T) {
	e := newOIDCEnv(t, nil)
	rr := httptest.NewRecorder()
	e.h.Logout(rr, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/logout", nil))
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, e.idp.srv.URL+"/logout?") ||
		!strings.Contains(loc, url.QueryEscape("https://zeit.example.de/login")) {
		t.Fatalf("got %q", loc)
	}
}

func TestOIDC_ConfigEndpoint(t *testing.T) {
	rr := httptest.NewRecorder()
	(&OIDCHandler{}).Config(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(rr.Body.String(), `"enabled":false`) {
		t.Fatalf("got %s", rr.Body.String())
	}
	e := newOIDCEnv(t, func(c *config.OIDCConfig) { c.PasswordLogin = "admins" })
	rr = httptest.NewRecorder()
	e.h.Config(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(rr.Body.String(), `"enabled":true`) || !strings.Contains(rr.Body.String(), `"password_login":"admins"`) {
		t.Fatalf("got %s", rr.Body.String())
	}
}

func TestAuth_Login_PasswordLoginAdminsOnly(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	auth := authsvc.New("test-secret-key-for-jwt", 1)
	hash, _ := auth.HashPassword("correcthorse")
	users := sqlite.NewUserStore(db)
	for _, u := range []*model.User{
		{Username: "anna", Role: model.RoleUser},
		{Username: "lea", Role: model.RoleLeitung},
	} {
		u.PasswordHash, u.DisplayName, u.Active = hash, u.Username, true
		if err := users.Create(context.Background(), u); err != nil {
			t.Fatal(err)
		}
	}
	h := &AuthHandler{Users: users, Auth: auth, PasswordLogin: oidcsvc.PasswordLoginAdmins}
	for name, want := range map[string]int{"anna": http.StatusForbidden, "lea": http.StatusOK} {
		rr := httptest.NewRecorder()
		h.Login(rr, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			strings.NewReader(`{"username":"`+name+`","password":"correcthorse"}`)))
		if rr.Code != want {
			t.Fatalf("%s: status %d, want %d", name, rr.Code, want)
		}
	}
}

func TestAuthJWT_RechecksUserOnEveryRequest(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	users := sqlite.NewUserStore(db)
	auth := authsvc.New("test-secret-key-for-jwt", 1)
	auth.SetUserLookup(func(ctx context.Context, id int) (bool, string, error) {
		u, err := users.GetByID(ctx, id)
		if err != nil {
			return false, "", err
		}
		return u.Active, string(u.Role), nil
	})
	u := &model.User{Username: "lea", DisplayName: "Lea", Role: model.RoleLeitung, Active: true, PasswordHash: "x"}
	if err := users.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	token, _ := auth.IssueToken(u.ID, u.Username, string(u.Role))

	r := chi.NewRouter()
	r.Use(apimw.AuthJWT(auth))
	r.Get("/role", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(apimw.Role(r))) })
	call := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/role", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		return rr
	}

	if rr := call(); rr.Code != http.StatusOK || rr.Body.String() != "leitung" {
		t.Fatalf("vorher: %d %s", rr.Code, rr.Body.String())
	}
	u.Role = model.RoleUser
	_ = users.Update(context.Background(), u)
	if rr := call(); rr.Body.String() != "user" {
		t.Fatalf("Rollenänderung greift nicht: %s", rr.Body.String())
	}
	u.Active = false
	_ = users.Update(context.Background(), u)
	if rr := call(); rr.Code != http.StatusUnauthorized {
		t.Fatalf("deaktiviert: %d", rr.Code)
	}
}
