package middleware

import (
	"context"
	"net/http"
	"strings"

	"nfc-time-tracking-server/internal/api/response"
	authsvc "nfc-time-tracking-server/internal/service/auth"
)

type ctxKey string

const (
	CtxUserID   ctxKey = "userID"
	CtxUsername ctxKey = "username"
	CtxRole     ctxKey = "role"
	// CtxGroupAccountID ist bei Gruppenaccounts gesetzt (nur auf Routen mit AuthJWTAllowGroup).
	CtxGroupAccountID ctxKey = "groupAccountID"
)

// AuthJWT lässt nur Benutzer-Tokens zu; Tokens von Gruppenaccounts werden abgewiesen.
func AuthJWT(svc *authsvc.Service) func(http.Handler) http.Handler {
	return authJWT(svc, false)
}

// AuthJWTAllowGroup lässt zusätzlich Tokens von Gruppenaccounts zu (Anwesenheitsliste, /auth/me).
func AuthJWTAllowGroup(svc *authsvc.Service) func(http.Handler) http.Handler {
	return authJWT(svc, true)
}

func authJWT(svc *authsvc.Service, allowGroup bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if h == "" || !strings.HasPrefix(h, "Bearer ") {
				response.Error(w, http.StatusUnauthorized, "missing bearer token")
				return
			}
			token := strings.TrimPrefix(h, "Bearer ")
			claims, err := svc.VerifyToken(strings.TrimSpace(token))
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "invalid token")
				return
			}
			claims, err = svc.CurrentClaims(r.Context(), claims)
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "invalid token")
				return
			}
			if claims.IsGroupAccount() && !allowGroup {
				response.Error(w, http.StatusForbidden, "forbidden")
				return
			}
			ctx := r.Context()
			if claims.IsGroupAccount() {
				ctx = context.WithValue(ctx, CtxGroupAccountID, claims.GroupAccountID)
			}
			ctx = context.WithValue(ctx, CtxUserID, claims.UserID)
			ctx = context.WithValue(ctx, CtxUsername, claims.Username)
			ctx = context.WithValue(ctx, CtxRole, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserID(r *http.Request) int {
	v := r.Context().Value(CtxUserID)
	if v == nil {
		return 0
	}
	return v.(int)
}

// GroupAccountID liefert die ID des angemeldeten Gruppenaccounts oder 0.
func GroupAccountID(r *http.Request) int {
	v, _ := r.Context().Value(CtxGroupAccountID).(int)
	return v
}

func Role(r *http.Request) string {
	v := r.Context().Value(CtxRole)
	if v == nil {
		return ""
	}
	return v.(string)
}

// RequireRole returns middleware that allows only listed roles.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := Role(r)
			if _, ok := allowed[role]; !ok {
				response.Error(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
