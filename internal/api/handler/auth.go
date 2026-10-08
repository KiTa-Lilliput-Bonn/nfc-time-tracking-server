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
	// GroupAccounts/Groups: Anmeldung der Gruppenaccounts (Anwesenheitsliste); nil = keine.
	GroupAccounts store.AttendanceStore
	Groups        store.GroupStore
	// PasswordLogin: auth.oidc.password_login ("" bzw. "all" = alle dürfen mit Passwort anmelden).
	PasswordLogin string
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
	// GroupID nur bei Gruppenaccounts: die Gruppe, deren Anwesenheitsliste das Konto sieht.
	GroupID *int `json:"group_id,omitempty"`
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
	if err != nil && h.GroupAccounts != nil {
		if ga, gerr := h.GroupAccounts.GetGroupAccountByUsername(r.Context(), body.Username); gerr == nil {
			h.loginGroupAccount(w, r, ga, body.Password)
			return
		}
	}
	if err != nil || !u.Active {
		response.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !h.Auth.CheckPassword(body.Password, u.PasswordHash) {
		response.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !PasswordLoginAllowed(h.PasswordLogin, u.Role) {
		response.Error(w, http.StatusForbidden, "password login disabled")
		return
	}
	token, err := h.Auth.IssueToken(u.ID, u.Username, string(u.Role))
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "token error")
		return
	}
	response.JSON(w, http.StatusOK, loginResponse{
		Token:   token,
		User:    toUserPublic(u),
		Expires: h.Auth.ExpirySeconds(),
	})
}

func toUserPublic(u *model.User) userPublic {
	return userPublic{
		ID:                 u.ID,
		Username:           u.Username,
		DisplayName:        u.DisplayName,
		Role:               u.Role,
		MustChangePassword: u.MustChangePassword,
	}
}

// loginGroupAccount meldet ein Gruppengerät an. Gruppenaccounts haben kein SSO, daher gilt
// auth.oidc.password_login für sie nicht.
func (h *AuthHandler) loginGroupAccount(w http.ResponseWriter, r *http.Request, ga *model.GroupAccount, password string) {
	if !ga.Active || !h.Auth.CheckPassword(password, ga.PasswordHash) {
		response.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	pub, err := h.groupAccountPublic(r, ga)
	if err != nil {
		response.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	token, err := h.Auth.IssueGroupToken(ga.ID, ga.Username, ga.SessionVersion)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "token error")
		return
	}
	response.JSON(w, http.StatusOK, loginResponse{
		Token:   token,
		User:    pub,
		Expires: int(authsvc.GroupTokenExpiry.Seconds()),
	})
}

func (h *AuthHandler) groupAccountPublic(r *http.Request, ga *model.GroupAccount) (userPublic, error) {
	g, err := h.Groups.GetByID(r.Context(), ga.GroupID)
	if err != nil {
		return userPublic{}, err
	}
	gid := ga.GroupID
	return userPublic{
		ID:          ga.ID,
		Username:    ga.Username,
		DisplayName: "Gruppe " + g.Name,
		Role:        model.Role(model.RoleGroupAccount),
		GroupID:     &gid,
	}, nil
}

// Me liefert den angemeldeten Benutzer (z. B. nach SSO-Login, wenn nur das Token bekannt ist).
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	if gaID := middleware.GroupAccountID(r); gaID != 0 && h.GroupAccounts != nil {
		ga, err := h.GroupAccounts.GetGroupAccount(r.Context(), gaID)
		if err != nil {
			response.Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		pub, err := h.groupAccountPublic(r, ga)
		if err != nil {
			response.Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		response.JSON(w, http.StatusOK, pub)
		return
	}
	uid := middleware.UserID(r)
	u, err := h.Users.GetByID(r.Context(), uid)
	if uid == 0 || err != nil || !u.Active {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	response.JSON(w, http.StatusOK, toUserPublic(u))
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
	if middleware.GroupAccountID(r) != 0 {
		response.Error(w, http.StatusForbidden, "forbidden")
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
	if gaID := middleware.GroupAccountID(r); gaID != 0 && h.GroupAccounts != nil {
		ga, err := h.GroupAccounts.GetGroupAccount(r.Context(), gaID)
		if err != nil || !ga.Active {
			response.Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		token, err := h.Auth.IssueGroupToken(ga.ID, ga.Username, ga.SessionVersion)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "token error")
			return
		}
		response.JSON(w, http.StatusOK, map[string]interface{}{
			"token":              token,
			"expires_in_seconds": int(authsvc.GroupTokenExpiry.Seconds()),
		})
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
