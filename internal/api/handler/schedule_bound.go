package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/entrylock"
)

type scheduleBoundBody struct {
	ScheduleBound bool   `json:"schedule_bound"`
	ValidFrom     string `json:"valid_from"`
}

type scheduleBoundResponse struct {
	model.ScheduleBoundSetting
	Mutable bool `json:"mutable"`
}

func scheduleBoundResponses(list []model.ScheduleBoundSetting) []scheduleBoundResponse {
	now := time.Now().UTC()
	out := make([]scheduleBoundResponse, len(list))
	for i, row := range list {
		out[i] = scheduleBoundResponse{
			ScheduleBoundSetting: row,
			Mutable:              entrylock.IsMutable(row.CreatedAt, now),
		}
	}
	return out
}

func (h *EmployeeHandler) PutScheduleBound(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeIDForWrite(w, r)
	if !ok {
		return
	}
	var body scheduleBoundBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.ValidFrom == "" {
		response.Error(w, http.StatusBadRequest, "valid_from required")
		return
	}
	if h.ScheduleBound == nil {
		response.Error(w, http.StatusInternalServerError, "schedule bound not configured")
		return
	}
	row := &model.ScheduleBoundSetting{
		UserID: uid, ScheduleBound: body.ScheduleBound, ValidFrom: model.NormCalendarDate(body.ValidFrom),
	}
	if err := h.ScheduleBound.Set(r.Context(), row); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityScheduleBound, EntityID: auditID(row.ID),
		TargetUserID: auditTarget(uid),
		Summary: audit.JSONSummary(map[string]any{
			"schedule_bound": body.ScheduleBound, "valid_from": body.ValidFrom,
		}),
	})
	resp := scheduleBoundResponse{
		ScheduleBoundSetting: *row,
		Mutable:              entrylock.IsMutable(row.CreatedAt, time.Now().UTC()),
	}
	response.JSON(w, http.StatusOK, resp)
}

func (h *EmployeeHandler) GetScheduleBound(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeID(w, r)
	if !ok {
		return
	}
	if h.ScheduleBound == nil {
		response.JSON(w, http.StatusOK, map[string]interface{}{"schedule_bound": []scheduleBoundResponse{}})
		return
	}
	list, err := h.ScheduleBound.ListByUser(r.Context(), uid)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"schedule_bound": scheduleBoundResponses(list),
	})
}

func (h *EmployeeHandler) DeleteScheduleBound(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeIDForWrite(w, r)
	if !ok {
		return
	}
	rowID, err := strconv.Atoi(chi.URLParam(r, "sbId"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	if h.ScheduleBound == nil {
		response.Error(w, http.StatusInternalServerError, "schedule bound not configured")
		return
	}
	row, err := h.ScheduleBound.GetByID(r.Context(), uid, rowID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	if row == nil {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	if !h.enforceMutableEntry(w, r, row.CreatedAt) {
		return
	}
	if err := h.ScheduleBound.Delete(r.Context(), uid, rowID); err != nil {
		response.Error(w, http.StatusInternalServerError, "delete failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionDelete, EntityType: audit.EntityScheduleBound, EntityID: auditID(rowID),
		TargetUserID: auditTarget(uid),
	})
	w.WriteHeader(http.StatusNoContent)
}
