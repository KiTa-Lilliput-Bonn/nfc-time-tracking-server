package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/bootstrap"
	"nfc-time-tracking-server/internal/lanhost"
	"nfc-time-tracking-server/internal/service/apipairing"
	"nfc-time-tracking-server/internal/store"
)

// DevicePairHandler handles app registration via short-lived pairing tokens.
type DevicePairHandler struct {
	Sessions *apipairing.SessionService
	Clients  store.ApiPairedClientStore
	Settings store.SettingsStore
}

type devicePairRegisterBody struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type devicePairRegisterResponse struct {
	ClientID string `json:"client_id"`
	Secret   string `json:"secret"`
	Label    string `json:"label"`
}

// PostRegister consumes a pairing token and records the app's LAN endpoint.
func (h *DevicePairHandler) PostRegister(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r.Header.Get("Authorization"))
	if token == "" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="device-pair"`)
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	clientID, err := h.Sessions.ConsumePairingToken(r.Context(), token)
	if err != nil {
		switch {
		case errors.Is(err, apipairing.ErrPairingTokenUsed):
			response.Error(w, http.StatusConflict, "pairing token already used")
		case errors.Is(err, apipairing.ErrPairingTokenExpired):
			response.Error(w, http.StatusUnauthorized, "pairing token expired")
		default:
			w.Header().Set("WWW-Authenticate", `Bearer realm="device-pair"`)
			response.Error(w, http.StatusUnauthorized, "unauthorized")
		}
		return
	}

	var body devicePairRegisterBody
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	host := strings.TrimSpace(body.Host)
	if host == "" {
		response.Error(w, http.StatusBadRequest, "host required")
		return
	}
	port := body.Port
	if port == 0 {
		port = 8787
	}
	if port < 1 || port > 65535 {
		response.Error(w, http.StatusBadRequest, "port out of range")
		return
	}
	if err := lanhost.ValidateAndroidLANHostContext(r.Context(), host); err != nil {
		response.Error(w, http.StatusBadRequest, fmt.Sprintf("host: %v", err))
		return
	}

	c, err := h.Clients.GetByID(r.Context(), clientID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		response.Error(w, http.StatusInternalServerError, "client lookup failed")
		return
	}
	if c.RevokedAtUTC != nil && strings.TrimSpace(*c.RevokedAtUTC) != "" {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	raw, err := h.Settings.Get(r.Context(), "android_lan_targets")
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "settings read failed")
		return
	}
	targets, err := bootstrap.ParseAndroidLanTargetsJSON(raw)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "settings parse failed")
		return
	}

	entry := bootstrap.AndroidLanTarget{
		ID:          clientID,
		Host:        host,
		Port:        port,
		APIClientID: clientID,
		Label:       strings.TrimSpace(c.Label),
	}
	targets, err = bootstrap.UpsertAndroidLanTarget(targets, entry)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	value, err := bootstrap.MarshalAndroidLanTargetsJSON(targets)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "settings marshal failed")
		return
	}
	if err := h.Settings.Set(r.Context(), "android_lan_targets", value); err != nil {
		response.Error(w, http.StatusInternalServerError, "settings save failed")
		return
	}

	response.JSON(w, http.StatusOK, devicePairRegisterResponse{
		ClientID: c.ID,
		Secret:   c.Secret,
		Label:    c.Label,
	})
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
