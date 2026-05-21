package handler

import (
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/store/sqlite"
)

// AuditHandler exposes superadmin audit log read and verify APIs.
type AuditHandler struct {
	Store *sqlite.AuditStore
}

func (h *AuditHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		response.Error(w, http.StatusInternalServerError, "audit not configured")
		return
	}
	f := audit.ListFilter{}
	if s := r.URL.Query().Get("from"); s != "" {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid from")
			return
		}
		f.From = &t
	}
	if s := r.URL.Query().Get("to"); s != "" {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid to")
			return
		}
		end := t.Add(24*time.Hour - time.Nanosecond)
		f.To = &end
	}
	if s := r.URL.Query().Get("entity_type"); s != "" {
		f.EntityType = s
	}
	if s := r.URL.Query().Get("actor_user_id"); s != "" {
		id, err := strconv.Atoi(s)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid actor_user_id")
			return
		}
		f.ActorUserID = &id
	}
	if s := r.URL.Query().Get("target_user_id"); s != "" {
		id, err := strconv.Atoi(s)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "invalid target_user_id")
			return
		}
		f.TargetUserID = &id
	}
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			response.Error(w, http.StatusBadRequest, "invalid limit")
			return
		}
		f.Limit = n
	}
	if s := r.URL.Query().Get("offset"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			response.Error(w, http.StatusBadRequest, "invalid offset")
			return
		}
		f.Offset = n
	}
	events, err := h.Store.List(r.Context(), f)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	if events == nil {
		events = []audit.Event{}
	}
	type eventJSON struct {
		ID            int64   `json:"id"`
		CreatedAt     string  `json:"created_at"`
		ActorUserID   *int    `json:"actor_user_id,omitempty"`
		ActorRole     string  `json:"actor_role"`
		Action        string  `json:"action"`
		EntityType    string  `json:"entity_type"`
		EntityID      string  `json:"entity_id"`
		TargetUserID  *int    `json:"target_user_id,omitempty"`
		Summary       string  `json:"summary"`
		PrevHash      string  `json:"prev_hash"`
		EventHash     string  `json:"event_hash"`
	}
	out := make([]eventJSON, len(events))
	for i, e := range events {
		out[i] = eventJSON{
			ID: e.ID, CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339Nano),
			ActorUserID: e.ActorUserID, ActorRole: e.ActorRole, Action: e.Action,
			EntityType: e.EntityType, EntityID: e.EntityID, TargetUserID: e.TargetUserID,
			Summary: e.Summary, PrevHash: hex.EncodeToString(e.PrevHash),
			EventHash: hex.EncodeToString(e.EventHash),
		}
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{"events": out})
}

func (h *AuditHandler) Verify(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		response.Error(w, http.StatusInternalServerError, "audit not configured")
		return
	}
	res := h.Store.Verify(r.Context())
	response.JSON(w, http.StatusOK, res)
}

func (h *AuditHandler) ListAnchors(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		response.Error(w, http.StatusInternalServerError, "audit not configured")
		return
	}
	anchors, err := h.Store.ListAnchors(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	if anchors == nil {
		anchors = []audit.Anchor{}
	}
	type anchorJSON struct {
		ID            int64  `json:"id"`
		AnchoredAt    string `json:"anchored_at"`
		LastDeletedID int64  `json:"last_deleted_id"`
		LastEventHash string `json:"last_event_hash"`
	}
	out := make([]anchorJSON, len(anchors))
	for i, a := range anchors {
		out[i] = anchorJSON{
			ID: a.ID, AnchoredAt: a.AnchoredAt.UTC().Format(time.RFC3339Nano),
			LastDeletedID: a.LastDeletedID, LastEventHash: hex.EncodeToString(a.LastEventHash),
		}
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{"anchors": out})
}
