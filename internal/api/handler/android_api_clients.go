package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/config"
	"nfc-time-tracking-server/internal/serverurl"
	"nfc-time-tracking-server/internal/service/apipairing"
	"nfc-time-tracking-server/internal/store"
)

// AndroidAPIClientsHandler manages paired API clients (superadmin JWT only).
type AndroidAPIClientsHandler struct {
	Clients  store.ApiPairedClientStore
	Sessions *apipairing.SessionService
	Server   config.ServerConfig
	Audit    *audit.Logger
}

type generateAndroidAPIClientBody struct {
	Label string `json:"label"`
}

// PostGenerate creates a new client with a server-generated secret (Klartext in DB) for QR pairing.
func (h *AndroidAPIClientsHandler) PostGenerate(w http.ResponseWriter, r *http.Request) {
	var body generateAndroidAPIClientBody
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	id, err := apipairing.NewID()
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "id generation failed")
		return
	}
	secret, err := apipairing.NewSecret()
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "secret generation failed")
		return
	}
	c, err := apipairing.BuildClient(id, body.Label, secret)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Clients.Insert(r.Context(), c); err != nil {
		response.Error(w, http.StatusInternalServerError, "save failed")
		return
	}
	pairingToken := ""
	if h.Sessions != nil {
		tok, err := h.Sessions.CreatePairingSession(r.Context(), c.ID)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "pairing session failed")
			return
		}
		pairingToken = tok
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionCreate, EntityType: audit.EntityAPIPairedClient, EntityID: c.ID,
		Summary: audit.JSONSummary(map[string]any{"label": c.Label}),
	})
	pairingBaseURL := serverurl.PairingBaseURL(r, h.Server)
	response.JSON(w, http.StatusCreated, map[string]interface{}{
		"client":            c,
		"pairing_token":     pairingToken,
		"pairing_base_url":  pairingBaseURL,
	})
}

func (h *AndroidAPIClientsHandler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.Clients.List(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{"clients": list})
}

func (h *AndroidAPIClientsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.Error(w, http.StatusBadRequest, "id required")
		return
	}
	err := h.Clients.Delete(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.Error(w, http.StatusNotFound, "client not found")
			return
		}
		response.Error(w, http.StatusInternalServerError, "delete failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionDelete, EntityType: audit.EntityAPIPairedClient, EntityID: id,
	})
	w.WriteHeader(http.StatusNoContent)
}
