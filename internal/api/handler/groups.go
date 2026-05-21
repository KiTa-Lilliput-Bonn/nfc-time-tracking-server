package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store"
)

type GroupHandler struct {
	Groups store.GroupStore
	Audit  *audit.Logger
}

func (h *GroupHandler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.Groups.List(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	if list == nil {
		list = []model.Group{}
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{"groups": list})
}

type createGroupBody struct {
	Name string `json:"name"`
}

func (h *GroupHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createGroupBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Name == "" {
		response.Error(w, http.StatusBadRequest, "name required")
		return
	}
	g := &model.Group{Name: body.Name}
	if err := h.Groups.Create(r.Context(), g); err != nil {
		response.Error(w, http.StatusBadRequest, "create failed (duplicate name?)")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionCreate, EntityType: audit.EntityGroup, EntityID: auditID(g.ID),
		Summary: audit.JSONSummary(map[string]any{"name": g.Name}),
	})
	response.JSON(w, http.StatusCreated, g)
}

type patchGroupBody struct {
	Name *string `json:"name"`
}

func (h *GroupHandler) Patch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body patchGroupBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Name == nil {
		response.Error(w, http.StatusBadRequest, "name required")
		return
	}
	if *body.Name == "" {
		response.Error(w, http.StatusBadRequest, "name must not be empty")
		return
	}
	g, err := h.Groups.GetByID(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	g.Name = *body.Name
	if err := h.Groups.Update(r.Context(), g); err != nil {
		response.Error(w, http.StatusBadRequest, "update failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityGroup, EntityID: auditID(g.ID),
		Summary: audit.JSONSummary(map[string]any{"name": g.Name}),
	})
	response.JSON(w, http.StatusOK, g)
}

func (h *GroupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.Groups.Delete(r.Context(), id); err != nil {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionDelete, EntityType: audit.EntityGroup, EntityID: auditID(id),
	})
	w.WriteHeader(http.StatusNoContent)
}

type putGroupOrderBody struct {
	IDs []int `json:"ids"`
}

func (h *GroupHandler) PutOrder(w http.ResponseWriter, r *http.Request) {
	var body putGroupOrderBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.Groups.Reorder(r.Context(), body.IDs); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid order")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityGroup, EntityID: "order",
		Summary: audit.JSONSummary(map[string]any{"ids": body.IDs}),
	})
	w.WriteHeader(http.StatusNoContent)
}
