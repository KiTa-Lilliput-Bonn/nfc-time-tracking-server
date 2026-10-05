// Package oidc kapselt die Anmeldung über einen OpenID-Connect-Provider
// (Authorization Code Flow mit PKCE). Die App stellt nach erfolgreichem Login
// ihr eigenes JWT aus; der Provider ersetzt nur die Passwortprüfung.
package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"nfc-time-tracking-server/internal/config"
)

// Password-Login-Modi (auth.oidc.password_login).
const (
	PasswordLoginAll    = "all"
	PasswordLoginAdmins = "admins"
	PasswordLoginNone   = "none"
)

// pendingTTL: so lange darf zwischen Start und Callback vergehen.
const pendingTTL = 10 * time.Minute

// maxPending begrenzt offene Anmeldevorgänge im Speicher.
const maxPending = 5000

// Identity ist das Ergebnis eines erfolgreichen Logins beim Provider.
type Identity struct {
	Subject  string
	Username string
	IDToken  string
}

type pending struct {
	nonce    string
	verifier string
	expires  time.Time
}

// Service hält die Provider-Konfiguration. Die Discovery passiert beim ersten
// Login (nicht beim Serverstart), damit die App auch startet, wenn der Provider
// gerade nicht erreichbar ist.
type Service struct {
	cfg config.OIDCConfig

	mu         sync.Mutex
	provider   *gooidc.Provider
	verifier   *gooidc.IDTokenVerifier
	endSession string
	pending    map[string]pending
	now        func() time.Time
}

// New prüft die Pflichtfelder und liefert nil, wenn OIDC nicht aktiviert ist.
func New(cfg config.OIDCConfig) (*Service, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	var missing []string
	if cfg.Issuer == "" {
		missing = append(missing, "issuer")
	}
	if cfg.ClientID == "" {
		missing = append(missing, "client_id")
	}
	if cfg.RedirectURL == "" {
		missing = append(missing, "redirect_url")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("auth.oidc: fehlende Angaben: %s", strings.Join(missing, ", "))
	}
	switch cfg.PasswordLogin {
	case "":
		cfg.PasswordLogin = PasswordLoginAll
	case PasswordLoginAll, PasswordLoginAdmins, PasswordLoginNone:
	default:
		return nil, fmt.Errorf("auth.oidc.password_login: %q ungültig (all, admins, none)", cfg.PasswordLogin)
	}
	if cfg.UsernameClaim == "" {
		cfg.UsernameClaim = "preferred_username"
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{gooidc.ScopeOpenID, "profile", "email"}
	}
	if cfg.ButtonLabel == "" {
		cfg.ButtonLabel = "Mit SSO anmelden"
	}
	return &Service{cfg: cfg, pending: map[string]pending{}, now: time.Now}, nil
}

// Config liefert die (mit Standardwerten ergänzte) Konfiguration.
func (s *Service) Config() config.OIDCConfig { return s.cfg }

func (s *Service) discover(ctx context.Context) (*gooidc.Provider, *gooidc.IDTokenVerifier, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.provider != nil {
		return s.provider, s.verifier, nil
	}
	p, err := gooidc.NewProvider(ctx, s.cfg.Issuer)
	if err != nil {
		return nil, nil, fmt.Errorf("oidc discovery: %w", err)
	}
	var extra struct {
		EndSession string `json:"end_session_endpoint"`
	}
	_ = p.Claims(&extra)
	s.provider = p
	s.verifier = p.Verifier(&gooidc.Config{ClientID: s.cfg.ClientID})
	s.endSession = extra.EndSession
	return s.provider, s.verifier, nil
}

func (s *Service) oauth2Config(p *gooidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     s.cfg.ClientID,
		ClientSecret: s.cfg.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  s.cfg.RedirectURL,
		Scopes:       s.cfg.Scopes,
	}
}

func randomString() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Start erzeugt state, nonce und PKCE-Verifier und liefert die URL zum Provider.
// state muss vom Aufrufer an den Browser gebunden werden (Cookie).
func (s *Service) Start(ctx context.Context) (authURL, state string, err error) {
	p, _, err := s.discover(ctx)
	if err != nil {
		return "", "", err
	}
	state, err = randomString()
	if err != nil {
		return "", "", err
	}
	nonce, err := randomString()
	if err != nil {
		return "", "", err
	}
	verifier := oauth2.GenerateVerifier()

	s.mu.Lock()
	now := s.now()
	for k, v := range s.pending {
		if now.After(v.expires) {
			delete(s.pending, k)
		}
	}
	if len(s.pending) >= maxPending {
		s.mu.Unlock()
		return "", "", errors.New("oidc: zu viele offene Anmeldungen")
	}
	s.pending[state] = pending{nonce: nonce, verifier: verifier, expires: now.Add(pendingTTL)}
	s.mu.Unlock()

	authURL = s.oauth2Config(p).AuthCodeURL(state, gooidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
	return authURL, state, nil
}

// ErrUnknownState: Callback ohne passenden Start (abgelaufen, schon benutzt oder fremd).
var ErrUnknownState = errors.New("oidc: unbekannter oder abgelaufener state")

// Finish tauscht den Code gegen Tokens, prüft das ID-Token und liefert die Identität.
func (s *Service) Finish(ctx context.Context, state, code string) (*Identity, error) {
	s.mu.Lock()
	pd, ok := s.pending[state]
	delete(s.pending, state)
	s.mu.Unlock()
	if !ok || s.now().After(pd.expires) {
		return nil, ErrUnknownState
	}

	p, verifier, err := s.discover(ctx)
	if err != nil {
		return nil, err
	}
	tok, err := s.oauth2Config(p).Exchange(ctx, code, oauth2.VerifierOption(pd.verifier))
	if err != nil {
		return nil, fmt.Errorf("oidc token exchange: %w", err)
	}
	rawID, _ := tok.Extra("id_token").(string)
	if rawID == "" {
		return nil, errors.New("oidc: kein id_token in der Antwort")
	}
	idt, err := verifier.Verify(ctx, rawID)
	if err != nil {
		return nil, fmt.Errorf("oidc id_token: %w", err)
	}
	if idt.Nonce != pd.nonce {
		return nil, errors.New("oidc: nonce stimmt nicht")
	}
	claims := map[string]any{}
	if err := idt.Claims(&claims); err != nil {
		return nil, fmt.Errorf("oidc claims: %w", err)
	}
	username, _ := claims[s.cfg.UsernameClaim].(string)
	if username == "" {
		// Manche Provider liefern Profil-Claims nur über den Userinfo-Endpunkt.
		if ui, err := p.UserInfo(ctx, oauth2.StaticTokenSource(tok)); err == nil {
			uiClaims := map[string]any{}
			if err := ui.Claims(&uiClaims); err == nil {
				if sub, _ := uiClaims["sub"].(string); sub == idt.Subject {
					username, _ = uiClaims[s.cfg.UsernameClaim].(string)
				}
			}
		}
	}
	return &Identity{Subject: idt.Subject, Username: strings.TrimSpace(username), IDToken: rawID}, nil
}

// LogoutURL liefert die Abmelde-Adresse des Providers (RP-Initiated Logout) oder "",
// wenn der Provider keinen end_session_endpoint anbietet.
func (s *Service) LogoutURL(ctx context.Context, postLogoutRedirect string) string {
	if _, _, err := s.discover(ctx); err != nil {
		return ""
	}
	s.mu.Lock()
	end := s.endSession
	s.mu.Unlock()
	if end == "" {
		return ""
	}
	u, err := url.Parse(end)
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Set("client_id", s.cfg.ClientID)
	if postLogoutRedirect != "" {
		q.Set("post_logout_redirect_uri", postLogoutRedirect)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// AppBaseURL leitet die öffentliche Basisadresse der App aus der Redirect-URL ab.
func (s *Service) AppBaseURL() string {
	u, err := url.Parse(s.cfg.RedirectURL)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
