package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/bootstrap"
	"nfc-time-tracking-server/internal/lanhost"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store"
)

// allowedSettingKeys limits writable keys (plan: validate known keys).
var allowedSettingKeys = map[string]struct{}{
	"rounding_minutes":             {},
	"break_rules":                  {},
	"android_lan_targets":        {},
	"stamps_poll_interval_seconds": {},
}

// SettingsHandler is superadmin settings API.
type SettingsHandler struct {
	Settings store.SettingsStore
	Audit    *audit.Logger
}

func (h *SettingsHandler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.Settings.GetAll(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{"settings": bootstrap.MaskSensitiveSettings(list)})
}

type settingPutBody struct {
	Value string `json:"value"`
}

func (h *SettingsHandler) Put(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		response.Error(w, http.StatusBadRequest, "key required")
		return
	}
	if _, ok := allowedSettingKeys[key]; !ok {
		response.Error(w, http.StatusBadRequest, "unknown or read-only setting key")
		return
	}
	var body settingPutBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if key == "android_lan_targets" {
		targets, err := bootstrap.ParseAndroidLanTargetsJSON(body.Value)
		if err != nil {
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		for _, t := range targets {
			if err := lanhost.ValidateAndroidLANHostContext(r.Context(), t.Host); err != nil {
				response.Error(w, http.StatusBadRequest, fmt.Sprintf("target %q: %v", t.ID, err))
				return
			}
		}
	}
	if err := h.Settings.Set(r.Context(), key, body.Value); err != nil {
		response.Error(w, http.StatusInternalServerError, "save failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntitySetting, EntityID: key,
		Summary: audit.JSONSummary(map[string]any{
			"key": key, "value": audit.RedactSettingValue(key, body.Value),
		}),
	})
	response.JSON(w, http.StatusOK, model.Setting{Key: key, Value: body.Value})
}
