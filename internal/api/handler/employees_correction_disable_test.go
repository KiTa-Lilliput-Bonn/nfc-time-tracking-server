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
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestEmployeeCreateCorrection_DisableImported(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	users := sqlite.NewUserStore(db)
	emp := &model.User{Username: "emp", PasswordHash: "x", DisplayName: "Emp", Role: model.RoleUser, Active: true}
	lead := &model.User{Username: "lead", PasswordHash: "x", DisplayName: "Lead", Role: model.RoleLeitung, Active: true}
	if err := users.Create(ctx, emp); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(ctx, lead); err != nil {
		t.Fatal(err)
	}

	wps := sqlite.NewWorkPeriodStore(db)
	tIn := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	tOut := time.Date(2026, 5, 1, 16, 0, 0, 0, time.UTC)
	if err := wps.ReplaceForUserDate(ctx, emp.ID, "2026-05-01", []model.WorkPeriod{{PunchIn: tIn, PunchOut: &tOut, IsBreak: false}}); err != nil {
		t.Fatal(err)
	}
	list, err := wps.ListByUserDateRange(ctx, emp.ID, "2026-05-01", "2026-05-01")
	if err != nil || len(list) != 1 {
		t.Fatalf("wps: %v %+v", err, list)
	}
	wpID := list[0].ID

	cs := sqlite.NewCorrectionStore(db)
	eh := &EmployeeHandler{Users: users, WorkPeriods: wps, Corrections: cs}

	body, _ := json.Marshal(map[string]any{
		"work_period_id": wpID,
		"disabled":       true,
		"reason":         "Falscher Stempel",
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

	var resp model.TimeCorrection
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Disabled {
		t.Fatal("expected disabled correction")
	}
	latest, err := cs.GetLatestForPeriod(ctx, wpID)
	if err != nil || latest == nil || !latest.Disabled {
		t.Fatalf("latest disabled: %v %+v", err, latest)
	}
}

func TestEmployeeCreateCorrection_DisableManualRejected(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	users := sqlite.NewUserStore(db)
	emp := &model.User{Username: "emp", PasswordHash: "x", DisplayName: "Emp", Role: model.RoleUser, Active: true}
	lead := &model.User{Username: "lead", PasswordHash: "x", DisplayName: "Lead", Role: model.RoleLeitung, Active: true}
	if err := users.Create(ctx, emp); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(ctx, lead); err != nil {
		t.Fatal(err)
	}

	wps := sqlite.NewWorkPeriodStore(db)
	tIn := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	tOut := time.Date(2026, 5, 1, 16, 0, 0, 0, time.UTC)
	wp := &model.WorkPeriod{UserID: emp.ID, WorkDate: "2026-05-01", PunchIn: tIn, PunchOut: &tOut, IsBreak: false}
	if err := wps.CreateManual(ctx, wp); err != nil {
		t.Fatal(err)
	}

	eh := &EmployeeHandler{
		Users: users, WorkPeriods: wps, Corrections: sqlite.NewCorrectionStore(db),
	}

	body, _ := json.Marshal(map[string]any{
		"work_period_id": wp.ID,
		"disabled":       true,
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
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
}
