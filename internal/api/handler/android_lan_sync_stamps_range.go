package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/service/stampspoll"
)

// AndroidLanSyncStampsRangeHandler triggers manual LAN stamps pull/push for a date range (Leitung).
type AndroidLanSyncStampsRangeHandler struct {
	Stamps *stampspoll.Service
	Audit  *audit.Logger
}

type androidLanSyncStampsRangeBody struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (h *AndroidLanSyncStampsRangeHandler) Post(w http.ResponseWriter, r *http.Request) {
	if h.Stamps == nil {
		response.Error(w, http.StatusServiceUnavailable, "stamps sync unavailable")
		return
	}
	var body androidLanSyncStampsRangeBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	fromS := strings.TrimSpace(body.From)
	toS := strings.TrimSpace(body.To)
	if fromS == "" || toS == "" {
		response.Error(w, http.StatusBadRequest, "from and to are required (YYYY-MM-DD)")
		return
	}
	fromT, err := stampspoll.ParseBerlinYMD(fromS)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid from date")
		return
	}
	toT, err := stampspoll.ParseBerlinYMD(toS)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid to date")
		return
	}
	res, err := h.Stamps.RunManualRangeSync(r.Context(), fromT, toT)
	if err != nil {
		switch {
		case errors.Is(err, stampspoll.ErrManualRangeInverted):
			response.Error(w, http.StatusBadRequest, "from must be on or before to")
		case errors.Is(err, stampspoll.ErrManualRangeTooLarge):
			response.Error(w, http.StatusBadRequest, "range must be at most 14 inclusive calendar days")
		default:
			response.Error(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityLanStampsSync, EntityID: fromS + ":" + toS,
		Summary: audit.JSONSummary(map[string]any{"from": fromS, "to": toS, "result": res}),
	})
	response.JSON(w, http.StatusOK, res)
}
