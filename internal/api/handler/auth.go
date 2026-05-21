package handler

import (
	"encoding/json"
	"net/http"

	"nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store"
	authsvc "nfc-time-tracking-server/internal/service/auth"
)

type AuthHandler struct {
	Users  store.UserStore
	Auth   *authsvc.Service
}

type loginBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userPublic struct {
	ID                 int         `json:"id"`
	Username           string      `json:"username"`
	DisplayName        string      `json:"display_name"`
	Role               model.Role  `json:"role"`
	MustChangePassword bool        `json:"must_change_password"`
}

type loginResponse struct {
	Token   string     `json:"token"`
	User    userPublic `json:"user"`
	Expires int        `json:"expires_in_seconds"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body loginBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	u, err := h.Users.GetByUsername(r.Context(), body.Username)
	if err != nil || !u.Active {
		response.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !h.Auth.CheckPassword(body.Password, u.PasswordHash) {
		response.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := h.Auth.IssueToken(u.ID, u.Username, string(u.Role))
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "token error")
		return
	}
	response.JSON(w, http.StatusOK, loginResponse{
		Token: token,
		User: userPublic{
			ID:                 u.ID,
			Username:           u.Username,
			DisplayName:        u.DisplayName,
			Role:               u.Role,
			MustChangePassword: u.MustChangePassword,
		},
		Expires: h.Auth.ExpirySeconds(),
	})
}

type changePasswordBody struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body changePasswordBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(body.NewPassword) < 8 {
		response.Error(w, http.StatusBadRequest, "new password too short")
		return
	}
	uid := middleware.UserID(r)
	if uid == 0 {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	u, err := h.Users.GetByID(r.Context(), uid)
	if err != nil {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !h.Auth.CheckPassword(body.CurrentPassword, u.PasswordHash) {
		response.Error(w, http.StatusUnauthorized, "invalid current password")
		return
	}
	hash, err := h.Auth.HashPassword(body.NewPassword)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "hash error")
		return
	}
	if err := h.Users.SetPassword(r.Context(), uid, hash, false); err != nil {
		response.Error(w, http.StatusInternalServerError, "update failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	uid := middleware.UserID(r)
	if uid == 0 {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	u, err := h.Users.GetByID(r.Context(), uid)
	if err != nil || !u.Active {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	token, err := h.Auth.IssueToken(u.ID, u.Username, string(u.Role))
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "token error")
		return
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"token":                 token,
		"expires_in_seconds":    h.Auth.ExpirySeconds(),
	})
}
