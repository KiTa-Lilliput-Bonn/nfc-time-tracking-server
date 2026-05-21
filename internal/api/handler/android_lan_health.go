package handler

import (
	"net/http"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/service/stampspoll"
)

// AndroidLanHealthHandler exposes LAN device reachability for Leitung UI.
type AndroidLanHealthHandler struct {
	Stamps *stampspoll.Service
}

func (h *AndroidLanHealthHandler) Get(w http.ResponseWriter, r *http.Request) {
	if h.Stamps == nil {
		response.JSON(w, http.StatusOK, stampspoll.LanHealthPayload{
			Mode: "disabled", Reachable: true, Targets: []stampspoll.LanTargetHealth{},
		})
		return
	}
	response.JSON(w, http.StatusOK, h.Stamps.LanHealth(r.Context()))
}
