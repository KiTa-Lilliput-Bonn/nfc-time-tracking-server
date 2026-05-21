package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/bootstrap"
	"nfc-time-tracking-server/internal/lanhost"
	"nfc-time-tracking-server/internal/service/lanemployeesync"
)

// AndroidLanEmployeeSyncHandler ruft die LAN-API der Flutter-App (nfc-time-tracking) auf:
// GET /v1/employees, POST /v1/employee-ids (Upsert), DELETE /v1/employee-ids/<id> — siehe lib/api/lan_api.dart im App-Repo.
type AndroidLanEmployeeSyncHandler struct {
	Sync *lanemployeesync.Service
}

type syncLanEmployeesBody struct {
	TargetID string `json:"target_id"`
}

func findLanTarget(targets []bootstrap.AndroidLanTarget, id string) *bootstrap.AndroidLanTarget {
	id = strings.TrimSpace(id)
	for i := range targets {
		if targets[i].ID == id {
			return &targets[i]
		}
	}
	return nil
}

// PostSync runs employee sync for one LAN target (body.target_id).
func (h *AndroidLanEmployeeSyncHandler) PostSync(w http.ResponseWriter, r *http.Request) {
	var body syncLanEmployeesBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	tid := strings.TrimSpace(body.TargetID)
	if tid == "" {
		response.Error(w, http.StatusBadRequest, "target_id required")
		return
	}

	targets, err := bootstrap.LanTargetsFromSettings(r.Context(), h.Sync.Settings)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "settings read failed")
		return
	}
	t := findLanTarget(targets, tid)
	if t == nil {
		response.Error(w, http.StatusNotFound, "lan target not found")
		return
	}

	c, err := h.Sync.SecretForAPIClient(r.Context(), t.APIClientID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			response.Error(w, http.StatusNotFound, err.Error())
			return
		}
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := lanhost.ValidateAndroidLANHostContext(r.Context(), t.Host); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	base := t.LanBaseURL()
	secret := strings.TrimSpace(c.Secret)

	out, status, msg := h.Sync.Execute(r.Context(), base, secret)
	if status != http.StatusOK {
		response.Error(w, status, msg)
		return
	}
	out["target_id"] = t.ID
	if strings.TrimSpace(t.Label) != "" {
		out["label"] = t.Label
	}
	response.JSON(w, http.StatusOK, out)
}

// PostSyncAll runs employee sync for every configured LAN target.
func (h *AndroidLanEmployeeSyncHandler) PostSyncAll(w http.ResponseWriter, r *http.Request) {
	payload, err := h.Sync.SyncAll(r.Context(), false)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "settings read failed")
		return
	}
	response.JSON(w, http.StatusOK, payload)
}
