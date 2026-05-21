package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	apimw "nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestEmployeeCreateCorrection_WritesAuditEvent(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	users := sqlite.NewUserStore(db)
	leitungHash := "x"
	emp := &model.User{Username: "emp", PasswordHash: "x", DisplayName: "Emp", Role: model.RoleUser, Active: true}
	lead := &model.User{Username: "lead", PasswordHash: leitungHash, DisplayName: "Lead", Role: model.RoleLeitung, Active: true}
	if err := users.Create(ctx, emp); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(ctx, lead); err != nil {
		t.Fatal(err)
	}

	wps := sqlite.NewWorkPeriodStore(db)
	tIn := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	tOut := time.Date(2026, 5, 1, 17, 0, 0, 0, time.UTC)
	wp := &model.WorkPeriod{UserID: emp.ID, WorkDate: "2026-05-01", PunchIn: tIn, PunchOut: &tOut, Source: "manual"}
	if err := wps.CreateManual(ctx, wp); err != nil {
		t.Fatal(err)
	}

	auditStore := sqlite.NewAuditStore(db)
	eh := &EmployeeHandler{
		Users: users, WorkPeriods: wps, Corrections: sqlite.NewCorrectionStore(db),
		Audit: &audit.Logger{Store: auditStore},
	}

	body, _ := json.Marshal(map[string]any{
		"work_period_id": wp.ID,
		"corrected_in":   tIn.Format(time.RFC3339),
		"corrected_out":  tOut.Format(time.RFC3339),
		"reason":         "test",
	})
	req := httptest.NewRequest(http.MethodPost, "/employees/1/corrections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.Itoa(emp.ID))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(context.WithValue(req.Context(), apimw.CtxUserID, lead.ID))
	req = req.WithContext(context.WithValue(req.Context(), apimw.CtxRole, string(model.RoleLeitung)))

	rr := httptest.NewRecorder()
	eh.CreateCorrection(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}

	events, err := auditStore.List(ctx, audit.ListFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events: %d", len(events))
	}
	if events[0].EntityType != audit.EntityTimeCorrection {
		t.Fatalf("entity_type %q", events[0].EntityType)
	}
	if events[0].ActorUserID == nil || *events[0].ActorUserID != lead.ID {
		t.Fatalf("actor %+v", events[0].ActorUserID)
	}
	res := auditStore.Verify(ctx)
	if !res.OK {
		t.Fatalf("verify: %+v", res)
	}
}
