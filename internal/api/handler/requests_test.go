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

type requestsFixture struct {
	h     *ChangeRequestHandler
	emp   *model.User
	lead  *model.User
	lead2 *model.User
	wpID  int
	db    *sqlite.DB
}

func newRequestsFixture(t *testing.T) *requestsFixture {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	users := sqlite.NewUserStore(db)
	emp := &model.User{Username: "emp", PasswordHash: "x", DisplayName: "Anna", Role: model.RoleUser, Active: true}
	lead := &model.User{Username: "lead", PasswordHash: "x", DisplayName: "Leitung", Role: model.RoleLeitung, Active: true}
	lead2 := &model.User{Username: "lead2", PasswordHash: "x", DisplayName: "Leitung 2", Role: model.RoleLeitung, Active: true}
	for _, u := range []*model.User{emp, lead, lead2} {
		if err := users.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	wps := sqlite.NewWorkPeriodStore(db)
	tIn := time.Date(2026, 5, 4, 8, 0, 0, 0, time.Local)
	tOut := time.Date(2026, 5, 4, 15, 0, 0, 0, time.Local)
	if err := wps.ReplaceForUserDate(ctx, emp.ID, "2026-05-04", []model.WorkPeriod{{PunchIn: tIn, PunchOut: &tOut}}); err != nil {
		t.Fatal(err)
	}
	list, err := wps.ListByUserDateRange(ctx, emp.ID, "2026-05-04", "2026-05-04")
	if err != nil || len(list) != 1 {
		t.Fatalf("wps: %v %+v", err, list)
	}
	h := &ChangeRequestHandler{
		Requests:              sqlite.NewChangeRequestStore(db),
		Users:                 users,
		WorkPeriods:           wps,
		Corrections:           sqlite.NewCorrectionStore(db),
		Absences:              sqlite.NewAbsenceStore(db),
		CompensationDayClaims: sqlite.NewCompensationDayClaimStore(db),
		FixedNonWorkWeekdays:  sqlite.NewFixedNonWorkWeekdaysStore(db),
		Holidays:              sqlite.NewHolidayStore(db),
		ClosureDays:           sqlite.NewClosureDayStore(db),
	}
	return &requestsFixture{h: h, emp: emp, lead: lead, lead2: lead2, wpID: list[0].ID, db: db}
}

func call(t *testing.T, fn http.HandlerFunc, actor *model.User, id int, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	rctx := chi.NewRouteContext()
	if id != 0 {
		rctx.URLParams.Add("id", strconv.Itoa(id))
	}
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, apimw.CtxUserID, actor.ID)
	ctx = context.WithValue(ctx, apimw.CtxRole, string(actor.Role))
	rr := httptest.NewRecorder()
	fn(rr, req.WithContext(ctx))
	return rr
}

func decodeRequest(t *testing.T, rr *httptest.ResponseRecorder) changeRequestResponse {
	t.Helper()
	var out changeRequestResponse
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func localTime(h, m int) time.Time {
	return time.Date(2026, 5, 4, h, m, 0, 0, time.Local)
}

func TestChangeRequest_TimeCorrectionOnlyAppliesAfterApproval(t *testing.T) {
	f := newRequestsFixture(t)
	ctx := context.Background()
	in, out := localTime(7, 30), localTime(15, 0)
	rr := call(t, f.h.CreateMine, f.emp, 0, map[string]any{
		"kind": "time_correction", "work_period_id": f.wpID, "punch_in": in, "punch_out": out, "reason": "Zu spät gestempelt",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	req := decodeRequest(t, rr)
	if req.Status != model.RequestPending || req.OriginalIn == nil || !req.OriginalIn.Equal(localTime(8, 0)) {
		t.Fatalf("unexpected request %+v", req)
	}
	if c, _ := f.h.Corrections.GetLatestForPeriod(ctx, f.wpID); c != nil {
		t.Fatal("correction must not exist before approval")
	}

	// A second open request for the same entry is refused.
	rr = call(t, f.h.CreateMine, f.emp, 0, map[string]any{
		"kind": "time_correction", "work_period_id": f.wpID, "punch_in": in, "punch_out": out, "reason": "nochmal",
	})
	if rr.Code != http.StatusConflict {
		t.Fatalf("duplicate: %d", rr.Code)
	}

	// The employee cannot approve.
	rr = call(t, f.h.Approve, f.emp, req.ID, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("self approve: %d", rr.Code)
	}

	rr = call(t, f.h.Approve, f.lead, req.ID, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rr.Code, rr.Body.String())
	}
	if got := decodeRequest(t, rr); got.Status != model.RequestApproved || got.DecidedByName != "Leitung" {
		t.Fatalf("approved: %+v", got)
	}
	c, err := f.h.Corrections.GetLatestForPeriod(ctx, f.wpID)
	if err != nil || c == nil || !c.CorrectedIn.Equal(in) || c.CorrectedBy != f.lead.ID {
		t.Fatalf("correction: %v %+v", err, c)
	}

	// Already decided.
	rr = call(t, f.h.Approve, f.lead, req.ID, nil)
	if rr.Code != http.StatusConflict {
		t.Fatalf("second approve: %d", rr.Code)
	}
}

func TestChangeRequest_RejectRequiresComment(t *testing.T) {
	f := newRequestsFixture(t)
	rr := call(t, f.h.CreateMine, f.emp, 0, map[string]any{
		"kind": "time_correction", "work_period_id": f.wpID, "punch_in": localTime(7, 0), "punch_out": localTime(15, 0), "reason": "x",
	})
	req := decodeRequest(t, rr)

	rr = call(t, f.h.Reject, f.lead, req.ID, map[string]any{"comment": "  "})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("reject without comment: %d", rr.Code)
	}
	rr = call(t, f.h.Reject, f.lead, req.ID, map[string]any{"comment": "Laut Dienstplan Beginn 8:00"})
	if rr.Code != http.StatusOK {
		t.Fatalf("reject: %d %s", rr.Code, rr.Body.String())
	}
	got := decodeRequest(t, rr)
	if got.Status != model.RequestRejected || got.DecisionComment != "Laut Dienstplan Beginn 8:00" {
		t.Fatalf("rejected: %+v", got)
	}
	if c, _ := f.h.Corrections.GetLatestForPeriod(context.Background(), f.wpID); c != nil {
		t.Fatal("rejected request must not create a correction")
	}
}

func TestChangeRequest_ApprovalFailsWhenEntryChangedMeanwhile(t *testing.T) {
	f := newRequestsFixture(t)
	ctx := context.Background()
	rr := call(t, f.h.CreateMine, f.emp, 0, map[string]any{
		"kind": "time_correction", "work_period_id": f.wpID, "punch_in": localTime(7, 0), "punch_out": localTime(15, 0), "reason": "x",
	})
	req := decodeRequest(t, rr)
	if err := f.h.Corrections.Create(ctx, &model.TimeCorrection{
		WorkPeriodID: f.wpID, CorrectedIn: localTime(9, 0), CorrectedOut: localTime(15, 0), Reason: "Leitung", CorrectedBy: f.lead.ID,
	}); err != nil {
		t.Fatal(err)
	}
	rr = call(t, f.h.Approve, f.lead, req.ID, nil)
	if rr.Code != http.StatusConflict {
		t.Fatalf("approve stale: %d %s", rr.Code, rr.Body.String())
	}
	still, _ := f.h.Requests.GetByID(ctx, req.ID)
	if still.Status != model.RequestPending {
		t.Fatalf("request should stay pending, got %s", still.Status)
	}
}

func TestChangeRequest_LeitungCannotDecideOwnRequest(t *testing.T) {
	f := newRequestsFixture(t)
	rr := call(t, f.h.CreateMine, f.lead, 0, map[string]any{
		"kind": "time_entry", "work_date": "2026-05-04", "punch_in": localTime(8, 0), "punch_out": localTime(12, 0), "reason": "vergessen",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	req := decodeRequest(t, rr)
	if rr := call(t, f.h.Approve, f.lead, req.ID, nil); rr.Code != http.StatusForbidden {
		t.Fatalf("own approve: %d", rr.Code)
	}
	if rr := call(t, f.h.Approve, f.lead2, req.ID, nil); rr.Code != http.StatusOK {
		t.Fatalf("other lead approve: %d %s", rr.Code, rr.Body.String())
	}
	list, _ := f.h.WorkPeriods.ListByUserDateRange(context.Background(), f.lead.ID, "2026-05-04", "2026-05-04")
	if len(list) != 1 || list[0].Source != "manual" {
		t.Fatalf("manual entry: %+v", list)
	}
}

func TestChangeRequest_TimeEntryOverlapRejected(t *testing.T) {
	f := newRequestsFixture(t)
	rr := call(t, f.h.CreateMine, f.emp, 0, map[string]any{
		"kind": "time_entry", "work_date": "2026-05-04", "punch_in": localTime(14, 0), "punch_out": localTime(16, 0), "reason": "x",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("overlap: %d %s", rr.Code, rr.Body.String())
	}
}

func TestChangeRequest_VacationRange(t *testing.T) {
	f := newRequestsFixture(t)
	ctx := context.Background()
	// Mo 11.05.–So 17.05.2026, Do 14.05. Christi Himmelfahrt.
	if err := f.h.Holidays.Create(ctx, &model.Holiday{HolidayDate: "2026-05-14", Name: "Christi Himmelfahrt"}); err != nil {
		t.Fatal(err)
	}
	rr := call(t, f.h.CreateMine, f.emp, 0, map[string]any{"kind": "vacation", "date_from": "2026-05-11", "date_to": "2026-05-17"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	req := decodeRequest(t, rr)
	if req.VacationDays != 4 {
		t.Fatalf("vacation days %v (%v)", req.VacationDays, req.VacationDates)
	}
	abs, _ := f.h.Absences.ListByUserDateRange(ctx, f.emp.ID, "2026-05-11", "2026-05-17")
	if len(abs) != 0 {
		t.Fatal("no absences before approval")
	}

	// Overlapping pending request is refused.
	rr = call(t, f.h.CreateMine, f.emp, 0, map[string]any{"kind": "vacation", "date_from": "2026-05-15", "date_to": "2026-05-15"})
	if rr.Code != http.StatusConflict {
		t.Fatalf("overlap: %d", rr.Code)
	}

	rr = call(t, f.h.Approve, f.lead, req.ID, map[string]any{"comment": "Schönen Urlaub"})
	if rr.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rr.Code, rr.Body.String())
	}
	abs, _ = f.h.Absences.ListByUserDateRange(ctx, f.emp.ID, "2026-05-11", "2026-05-17")
	if len(abs) != 4 {
		t.Fatalf("absences: %+v", abs)
	}
	for _, a := range abs {
		if a.AbsenceType != model.AbsenceVacation || a.CreatedBy != f.lead.ID {
			t.Fatalf("absence %+v", a)
		}
	}
}

func TestChangeRequest_VacationValidation(t *testing.T) {
	f := newRequestsFixture(t)
	cases := []map[string]any{
		{"kind": "vacation", "date_from": "2026-05-16", "date_to": "2026-05-17"},                   // nur Wochenende
		{"kind": "vacation", "date_from": "2026-05-12", "date_to": "2026-05-11"},                   // Ende vor Start
		{"kind": "vacation", "date_from": "2026-05-11", "date_to": "2026-05-12", "half_day": true}, // halber Tag über mehrere Tage
	}
	for i, body := range cases {
		if rr := call(t, f.h.CreateMine, f.emp, 0, body); rr.Code != http.StatusBadRequest {
			t.Fatalf("case %d: %d %s", i, rr.Code, rr.Body.String())
		}
	}
}

func TestChangeRequest_Withdraw(t *testing.T) {
	f := newRequestsFixture(t)
	rr := call(t, f.h.CreateMine, f.emp, 0, map[string]any{"kind": "vacation", "date_from": "2026-05-11", "date_to": "2026-05-11", "half_day": true})
	req := decodeRequest(t, rr)
	if req.VacationDays != 0.5 {
		t.Fatalf("half day: %v", req.VacationDays)
	}
	if rr := call(t, f.h.WithdrawMine, f.lead, req.ID, nil); rr.Code != http.StatusConflict {
		t.Fatalf("foreign withdraw: %d", rr.Code)
	}
	if rr := call(t, f.h.WithdrawMine, f.emp, req.ID, nil); rr.Code != http.StatusNoContent {
		t.Fatalf("withdraw: %d", rr.Code)
	}
	if rr := call(t, f.h.Approve, f.lead, req.ID, nil); rr.Code != http.StatusConflict {
		t.Fatalf("approve withdrawn: %d", rr.Code)
	}
}
