package handler

import (
	"encoding/json"
	"net/http"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/service/shiftalerts"
	"nfc-time-tracking-server/internal/store"
)

// ShiftAlertConfigHandler serves Leitung/Superadmin shift alert threshold settings.
type ShiftAlertConfigHandler struct {
	Settings store.SettingsStore
	Audit    *audit.Logger
}

// Get returns current shift alert thresholds (GET /shift-alert-config).
func (h *ShiftAlertConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg, err := shiftalerts.LoadConfig(r.Context(), h.Settings)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "settings failed")
		return
	}
	response.JSON(w, http.StatusOK, cfg)
}

// Put updates shift alert thresholds (PUT /shift-alert-config).
func (h *ShiftAlertConfigHandler) Put(w http.ResponseWriter, r *http.Request) {
	var body shiftalerts.Config
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := shiftalerts.SaveConfig(r.Context(), h.Settings, body); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntitySetting,
		EntityID: shiftalerts.SettingMaxHours,
		Summary: audit.JSONSummary(map[string]any{
			"shift_alert_max_hours": body.MaxHours,
			"shift_alert_late_end":  body.LateEnd,
		}),
	})
	response.JSON(w, http.StatusOK, body)
}
