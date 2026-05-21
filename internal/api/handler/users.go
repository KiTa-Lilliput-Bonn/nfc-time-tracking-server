package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/model"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	"nfc-time-tracking-server/internal/store"
)

// UsersHandler is superadmin user management (GET/POST/PATCH /users).
type UsersHandler struct {
	Users store.UserStore
	Auth  *authsvc.Service
	Audit *audit.Logger
}

func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.Users.List(r.Context(), false)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{"users": users})
}

type createUserBody struct {
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	Role        model.Role `json:"role"`
}

func (h *UsersHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body createUserBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Username == "" || body.DisplayName == "" || body.Role == "" {
		response.Error(w, http.StatusBadRequest, "username, display_name, role required")
		return
	}
	pw := authsvc.GenerateRandomPassword(14)
	hash, err := h.Auth.HashPassword(pw)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "hash error")
		return
	}
	u := &model.User{
		Username: body.Username, DisplayName: body.DisplayName, Role: body.Role,
		PasswordHash: hash, Active: true, MustChangePassword: true,
	}
	if err := h.Users.Create(r.Context(), u); err != nil {
		response.Error(w, http.StatusBadRequest, "create failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionCreate, EntityType: audit.EntityUser, EntityID: auditID(u.ID),
		TargetUserID: auditTarget(u.ID),
		Summary:      audit.JSONSummary(map[string]any{"username": u.Username, "role": u.Role}),
	})
	response.JSON(w, http.StatusCreated, map[string]interface{}{"user": u, "temporary_password": pw})
}

type patchUserBody struct {
	DisplayName *string     `json:"display_name"`
	Role        *model.Role `json:"role"`
	Active      *bool       `json:"active"`
}

func (h *UsersHandler) Patch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body patchUserBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	u, err := h.Users.GetByID(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	if body.DisplayName != nil {
		u.DisplayName = *body.DisplayName
	}
	if body.Active != nil {
		u.Active = *body.Active
	}
	if body.Role != nil {
		u.Role = *body.Role
	}
	if err := h.Users.Update(r.Context(), u); err != nil {
		response.Error(w, http.StatusInternalServerError, "update failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityUser, EntityID: auditID(u.ID),
		TargetUserID: auditTarget(u.ID),
		Summary:      audit.JSONSummary(map[string]any{"username": u.Username, "role": u.Role, "active": u.Active}),
	})
	response.JSON(w, http.StatusOK, u)
}
