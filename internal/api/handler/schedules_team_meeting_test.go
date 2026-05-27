package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func openHandlerTestDB(t *testing.T) *sqlite.DB {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestPostTeamMeeting_Other(t *testing.T) {
	db := openHandlerTestDB(t)
	ctx := context.Background()
	us := sqlite.NewUserStore(db)
	u := &model.User{Username: "tm", PasswordHash: "x", DisplayName: "TM", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	tms := sqlite.NewTeamMeetingStore(db)
	h := &ScheduleHandler{TeamMeetings: tms}

	body, _ := json.Marshal(map[string]any{
		"year":         2026,
		"week":         12,
		"kind":         "other",
		"meeting_date": "2026-03-18",
		"label":        "Fortbildung",
		"time_start":   "09:00",
		"time_end":     "10:00",
		"user_ids":     []int{u.ID},
	})
	req := httptest.NewRequest(http.MethodPost, "/team-meetings", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.PostTeamMeeting(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var out model.TeamMeeting
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Kind != model.TeamMeetingKindOther || out.Label != "Fortbildung" {
		t.Fatalf("got %+v", out)
	}
	if out.MeetingDate != "2026-03-18" {
		t.Fatalf("meeting_date %q", out.MeetingDate)
	}
}

func TestPostTeamMeeting_OtherRequiresLabel(t *testing.T) {
	db := openHandlerTestDB(t)
	ctx := context.Background()
	us := sqlite.NewUserStore(db)
	u := &model.User{Username: "tm2", PasswordHash: "x", DisplayName: "TM2", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	h := &ScheduleHandler{TeamMeetings: sqlite.NewTeamMeetingStore(db)}

	body, _ := json.Marshal(map[string]any{
		"year":         2026,
		"week":         12,
		"kind":         "other",
		"meeting_date": "2026-03-18",
		"time_start":   "09:00",
		"time_end":     "10:00",
		"user_ids":     []int{u.ID},
	})
	req := httptest.NewRequest(http.MethodPost, "/team-meetings", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.PostTeamMeeting(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rr.Code)
	}
}

func TestPostTeamMeeting_KTStillMonday(t *testing.T) {
	db := openHandlerTestDB(t)
	ctx := context.Background()
	us := sqlite.NewUserStore(db)
	u := &model.User{Username: "tm3", PasswordHash: "x", DisplayName: "TM3", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	h := &ScheduleHandler{TeamMeetings: sqlite.NewTeamMeetingStore(db)}

	body, _ := json.Marshal(map[string]any{
		"year":       2026,
		"week":       12,
		"kind":       "kt",
		"time_start": "09:00",
		"time_end":   "10:00",
		"user_ids":   []int{u.ID},
	})
	req := httptest.NewRequest(http.MethodPost, "/team-meetings", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.PostTeamMeeting(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var out model.TeamMeeting
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.MeetingDate != "2026-03-16" {
		t.Fatalf("expected monday 2026-03-16, got %q", out.MeetingDate)
	}
}
