package handler

import (
	"net/http"

	"nfc-time-tracking-server/internal/api/response"
)

// DeviceStampsHandler serves GET .../v1/stamps for Bearer-authenticated API clients.
type DeviceStampsHandler struct{}

// Stamps returns stamp events in the shape expected by nfc-time-tracking LAN API (empty until wired to punches).
func (h *DeviceStampsHandler) Stamps(w http.ResponseWriter, r *http.Request) {
	_ = r
	_ = h
	list := []map[string]interface{}{}
	response.JSON(w, http.StatusOK, list)
}
