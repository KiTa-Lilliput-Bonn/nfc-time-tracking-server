package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/folderpick"
	"nfc-time-tracking-server/internal/service/backup"
)

var backupPickFolderMu sync.Mutex

// BackupHandler exposes superadmin backup configuration and operations.
type BackupHandler struct {
	Backup *backup.Service
}

func (h *BackupHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if h.Backup == nil {
		response.Error(w, http.StatusServiceUnavailable, "backup unavailable")
		return
	}
	st, err := h.Backup.ReadStatus(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "read status failed")
		return
	}
	response.JSON(w, http.StatusOK, st)
}

type backupPutConfigBody struct {
	Enabled         bool   `json:"enabled"`
	IntervalMinutes int    `json:"interval_minutes"`
	UseRestic       bool   `json:"use_restic"`
	TargetPath      string `json:"target_path"`
}

func (h *BackupHandler) PutConfig(w http.ResponseWriter, r *http.Request) {
	if h.Backup == nil {
		response.Error(w, http.StatusServiceUnavailable, "backup unavailable")
		return
	}
	var body backupPutConfigBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.Backup.SaveConfig(r.Context(), body.Enabled, body.IntervalMinutes, body.UseRestic, body.TargetPath); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	st, err := h.Backup.ReadStatus(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "read status failed")
		return
	}
	response.JSON(w, http.StatusOK, st)
}

type backupInitBody struct {
	RepoPath string `json:"repo_path"`
}

type backupInitResponse struct {
	Password string `json:"password"`
	Message  string `json:"message"`
}

func (h *BackupHandler) PostInitRestic(w http.ResponseWriter, r *http.Request) {
	if h.Backup == nil {
		response.Error(w, http.StatusServiceUnavailable, "backup unavailable")
		return
	}
	var body backupInitBody
	_ = json.NewDecoder(r.Body).Decode(&body)
	repo := strings.TrimSpace(body.RepoPath)
	if repo == "" {
		var err error
		repo, err = h.Backup.Settings.Get(r.Context(), backup.SettingTargetPath)
		if err != nil || strings.TrimSpace(repo) == "" {
			response.Error(w, http.StatusBadRequest, "repo_path required (or set target_path in config first)")
			return
		}
		repo = strings.TrimSpace(repo)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	pw, err := h.Backup.InitResticRepo(ctx, repo)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Backup.Settings.Set(ctx, backup.SettingResticPassword, pw); err != nil {
		response.Error(w, http.StatusInternalServerError, "save password failed")
		return
	}
	if err := h.Backup.Settings.Set(ctx, backup.SettingResticInitialized, "true"); err != nil {
		response.Error(w, http.StatusInternalServerError, "save init flag failed")
		return
	}
	if err := h.Backup.Settings.Set(ctx, backup.SettingTargetPath, repo); err != nil {
		response.Error(w, http.StatusInternalServerError, "save repo path failed")
		return
	}
	if err := h.Backup.Settings.Set(ctx, backup.SettingUseRestic, "true"); err != nil {
		response.Error(w, http.StatusInternalServerError, "save use_restic failed")
		return
	}
	response.JSON(w, http.StatusOK, backupInitResponse{
		Password: pw,
		Message:  "Repository initialisiert. Passwort sicher aufbewahren — es wird nicht erneut angezeigt.",
	})
}

type backupPickFolderBody struct {
	InitialPath string `json:"initial_path"`
}

type backupPickFolderResponse struct {
	Path      string `json:"path"`
	Cancelled bool   `json:"cancelled"`
}

func (h *BackupHandler) PostPickFolder(w http.ResponseWriter, r *http.Request) {
	if h.Backup == nil {
		response.Error(w, http.StatusServiceUnavailable, "backup unavailable")
		return
	}
	if !folderpick.Available() {
		response.Error(w, http.StatusNotImplemented, "native folder picker unavailable on this server")
		return
	}
	var body backupPickFolderBody
	_ = json.NewDecoder(r.Body).Decode(&body)

	backupPickFolderMu.Lock()
	chosen, err := folderpick.Pick(body.InitialPath)
	backupPickFolderMu.Unlock()

	if errors.Is(err, folderpick.ErrCancelled) {
		response.JSON(w, http.StatusOK, backupPickFolderResponse{Cancelled: true})
		return
	}
	if errors.Is(err, folderpick.ErrUnavailable) {
		response.Error(w, http.StatusNotImplemented, err.Error())
		return
	}
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	response.JSON(w, http.StatusOK, backupPickFolderResponse{Path: chosen})
}

func (h *BackupHandler) GetBrowse(w http.ResponseWriter, r *http.Request) {
	if h.Backup == nil {
		response.Error(w, http.StatusServiceUnavailable, "backup unavailable")
		return
	}
	var result backup.BrowseResult
	var err error
	if r.URL.Query().Get("roots") == "1" {
		result, err = backup.BrowseRoots()
	} else {
		result, err = h.Backup.BrowseDirectories(r.Context(), r.URL.Query().Get("path"))
	}
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	response.JSON(w, http.StatusOK, result)
}

func (h *BackupHandler) PostRunNow(w http.ResponseWriter, r *http.Request) {
	if h.Backup == nil {
		response.Error(w, http.StatusServiceUnavailable, "backup unavailable")
		return
	}
	st0, err := h.Backup.ReadStatus(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "read status failed")
		return
	}
	if !st0.Enabled {
		response.Error(w, http.StatusBadRequest, "backup is disabled")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Minute)
	defer cancel()
	if err := h.Backup.Run(ctx); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	st, err := h.Backup.ReadStatus(ctx)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "read status failed")
		return
	}
	response.JSON(w, http.StatusOK, st)
}
