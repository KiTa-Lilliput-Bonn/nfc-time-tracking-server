package middleware

import (
	"net/http"
	"strings"

	"nfc-time-tracking-server/internal/service/apipairing"
)

// BearerPairingAuth validates Authorization: Bearer <secret> against paired API clients (not JWT).
func BearerPairingAuth(svc *apipairing.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r.Header.Get("Authorization"))
			if token == "" || !svc.IsAuthorizedBearer(r.Context(), token) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="device-api"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractBearerToken(headerValue string) string {
	v := strings.TrimSpace(headerValue)
	if len(v) < 8 {
		return ""
	}
	if strings.ToLower(v[:7]) != "bearer " {
		return ""
	}
	return strings.TrimSpace(v[7:])
}
