package handler

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/model"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	oidcsvc "nfc-time-tracking-server/internal/service/oidc"
	"nfc-time-tracking-server/internal/store"
)

const oidcStateCookie = "nfc_oidc_state"

// Fehlercodes, die per ?sso_error= an die Login-Seite gehen.
const (
	ssoErrFailed    = "failed"
	ssoErrDenied    = "denied"
	ssoErrNoAccount = "no_account"
	ssoErrInactive  = "inactive"
	ssoErrConflict  = "conflict"
)

// OIDCHandler: Anmeldung über einen OpenID-Connect-Provider (SSO).
type OIDCHandler struct {
	OIDC  *oidcsvc.Service // nil = SSO nicht aktiviert
	Users store.UserStore
	Auth  *authsvc.Service
	Audit *audit.Logger
}

type oidcPublicConfig struct {
	Enabled       bool   `json:"enabled"`
	ButtonLabel   string `json:"button_label,omitempty"`
	PasswordLogin string `json:"password_login"`
}

// Config: öffentlich, damit die Login-Seite den SSO-Button anzeigen kann.
func (h *OIDCHandler) Config(w http.ResponseWriter, r *http.Request) {
	if h.OIDC == nil {
		response.JSON(w, http.StatusOK, oidcPublicConfig{Enabled: false, PasswordLogin: oidcsvc.PasswordLoginAll})
		return
	}
	c := h.OIDC.Config()
	response.JSON(w, http.StatusOK, oidcPublicConfig{Enabled: true, ButtonLabel: c.ButtonLabel, PasswordLogin: c.PasswordLogin})
}

func (h *OIDCHandler) secureCookie() bool {
	return strings.HasPrefix(strings.ToLower(h.OIDC.Config().RedirectURL), "https://")
}

// Start leitet zum Provider weiter.
func (h *OIDCHandler) Start(w http.ResponseWriter, r *http.Request) {
	if h.OIDC == nil {
		http.NotFound(w, r)
		return
	}
	authURL, state, err := h.OIDC.Start(r.Context())
	if err != nil {
		log.Printf("oidc start: %v", err)
		redirectLoginError(w, r, ssoErrFailed)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookie,
		Value:    state,
		Path:     "/api/v1/auth/oidc",
		MaxAge:   int((10 * time.Minute).Seconds()),
		HttpOnly: true,
		Secure:   h.secureCookie(),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback nimmt die Antwort des Providers entgegen, ordnet den App-Benutzer zu
// und übergibt das App-Token per URL-Fragment an die Oberfläche.
func (h *OIDCHandler) Callback(w http.ResponseWriter, r *http.Request) {
	if h.OIDC == nil {
		http.NotFound(w, r)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: oidcStateCookie, Value: "", Path: "/api/v1/auth/oidc", MaxAge: -1,
		HttpOnly: true, Secure: h.secureCookie(), SameSite: http.SameSiteLaxMode,
	})
	q := r.URL.Query()
	state := q.Get("state")
	c, err := r.Cookie(oidcStateCookie)
	if err != nil || state == "" || c.Value != state {
		redirectLoginError(w, r, ssoErrFailed)
		return
	}
	if e := q.Get("error"); e != "" {
		log.Printf("oidc callback: provider error %q", e)
		redirectLoginError(w, r, ssoErrDenied)
		return
	}
	id, err := h.OIDC.Finish(r.Context(), state, q.Get("code"))
	if err != nil {
		log.Printf("oidc callback: %v", err)
		redirectLoginError(w, r, ssoErrFailed)
		return
	}
	u, code := h.resolveUser(r.Context(), id)
	if code != "" {
		redirectLoginError(w, r, code)
		return
	}
	token, err := h.Auth.IssueToken(u.ID, u.Username, string(u.Role))
	if err != nil {
		redirectLoginError(w, r, ssoErrFailed)
		return
	}
	frag := url.Values{}
	frag.Set("token", token)
	http.Redirect(w, r, "/login/sso#"+frag.Encode(), http.StatusFound)
}

// resolveUser sucht den verknüpften Benutzer oder verknüpft beim ersten Login
// über den Benutzernamen. Liefert bei Fehlern einen sso_error-Code.
func (h *OIDCHandler) resolveUser(ctx context.Context, id *oidcsvc.Identity) (*model.User, string) {
	if id.Subject == "" {
		return nil, ssoErrFailed
	}
	if u, err := h.Users.GetBySSOSubject(ctx, id.Subject); err == nil {
		if !u.Active {
			return nil, ssoErrInactive
		}
		return u, ""
	}
	if !h.OIDC.Config().AutoLinkEnabled() || id.Username == "" {
		log.Printf("oidc: kein verknüpfter Benutzer für SSO-Konto %q", id.Username)
		return nil, ssoErrNoAccount
	}
	matches, err := h.Users.FindByUsernameFold(ctx, id.Username)
	if err != nil {
		return nil, ssoErrFailed
	}
	if len(matches) == 0 {
		log.Printf("oidc: kein App-Benutzer %q für automatische Verknüpfung", id.Username)
		return nil, ssoErrNoAccount
	}
	if len(matches) > 1 {
		log.Printf("oidc: Benutzername %q ist nicht eindeutig (Groß-/Kleinschreibung)", id.Username)
		return nil, ssoErrConflict
	}
	u := &matches[0]
	if u.SSOSubject != "" {
		// Bereits mit einem anderen SSO-Konto verknüpft: nicht stillschweigend umhängen.
		log.Printf("oidc: Benutzer %q ist bereits mit einem anderen SSO-Konto verknüpft", u.Username)
		return nil, ssoErrConflict
	}
	if !u.Active {
		return nil, ssoErrInactive
	}
	if err := h.Users.SetSSOSubject(ctx, u.ID, id.Subject); err != nil {
		log.Printf("oidc: verknüpfen fehlgeschlagen: %v", err)
		return nil, ssoErrConflict
	}
	uid := u.ID
	logAudit(h.Audit, ctx, audit.Entry{
		ActorUserID: &uid, ActorRole: string(u.Role),
		Action: audit.ActionUpdate, EntityType: audit.EntityUser, EntityID: auditID(u.ID),
		TargetUserID: auditTarget(u.ID),
		Summary:      audit.JSONSummary(map[string]any{"sso_linked": true, "sso_username": id.Username}),
	})
	u.SSOSubject = id.Subject
	u.SSOLinked = true
	return u, ""
}

// Logout leitet zum Abmelden beim Provider weiter (falls unterstützt), sonst zur Login-Seite.
func (h *OIDCHandler) Logout(w http.ResponseWriter, r *http.Request) {
	target := "/login"
	if h.OIDC != nil {
		post := ""
		if base := h.OIDC.AppBaseURL(); base != "" {
			post = base + "/login"
		}
		if u := h.OIDC.LogoutURL(r.Context(), post); u != "" {
			target = u
		}
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func redirectLoginError(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/login?sso_error="+url.QueryEscape(code), http.StatusFound)
}

// PasswordLoginAllowed prüft die Einstellung auth.oidc.password_login für eine Rolle.
func PasswordLoginAllowed(mode string, role model.Role) bool {
	switch mode {
	case oidcsvc.PasswordLoginNone:
		return false
	case oidcsvc.PasswordLoginAdmins:
		return role == model.RoleLeitung || role == model.RoleSuperadmin
	default:
		return true
	}
}
