package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	apimw "nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/model"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestCreateCompensationDayAbsence_UsesOldestOpenClaim(t *testing.T) {
	db, auth, token, employee := setupCompensationDayAPITest(t)
	ctx := context.Background()
	claims := sqlite.NewCompensationDayClaimStore(db)
	if err := claims.EnsureForWorkDate(ctx, employee.ID, "2026-04-05", true); err != nil {
		t.Fatal(err)
	}
	if err := claims.EnsureForWorkDate(ctx, employee.ID, "2026-04-04", true); err != nil {
		t.Fatal(err)
	}

	router := compensationDayEmployeeRouter(db, auth)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, compensationDayAPIRequest(t, token, http.MethodPost, employee.ID, "/absences", map[string]interface{}{
		"absence_date": "2026-04-07",
		"absence_type": "compensation_day",
		"half_day":     false,
	}))
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var absence model.Absence
	if err := json.NewDecoder(rr.Body).Decode(&absence); err != nil {
		t.Fatal(err)
	}
	list, err := claims.ListByUser(ctx, employee.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	var used *model.CompensationDayClaim
	for i := range list {
		if strings.HasPrefix(list[i].WorkDate, "2026-04-04") {
			used = &list[i]
			break
		}
	}
	if used == nil {
		t.Fatalf("oldest claim not found in %+v", list)
	}
	if used.Status != model.CompensationDayClaimUsed {
		t.Fatalf("oldest claim status want used, got %q", used.Status)
	}
	if used.UsedAbsenceID == nil || *used.UsedAbsenceID != absence.ID {
		t.Fatalf("used_absence_id want %d, got %+v", absence.ID, used.UsedAbsenceID)
	}
	open, err := claims.CountOpen(ctx, employee.ID)
	if err != nil {
		t.Fatal(err)
	}
	if open != 1 {
		t.Fatalf("remaining open claims want 1, got %d", open)
	}
}

func TestCreateCompensationDayAbsence_RejectsWithoutOpenClaim(t *testing.T) {
	db, auth, token, employee := setupCompensationDayAPITest(t)
	router := compensationDayEmployeeRouter(db, auth)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, compensationDayAPIRequest(t, token, http.MethodPost, employee.ID, "/absences", map[string]interface{}{
		"absence_date": "2026-04-07",
		"absence_type": "compensation_day",
		"half_day":     false,
	}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("kein offener Ausgleichstag-Anspruch")) {
		t.Fatalf("response should mention missing claim, got %s", rr.Body.String())
	}
}

func TestDeleteCompensationDayAbsence_ReopensUsedClaim(t *testing.T) {
	db, auth, token, employee := setupCompensationDayAPITest(t)
	ctx := context.Background()
	claims := sqlite.NewCompensationDayClaimStore(db)
	if err := claims.EnsureForWorkDate(ctx, employee.ID, "2026-04-04", true); err != nil {
		t.Fatal(err)
	}
	router := compensationDayEmployeeRouter(db, auth)

	createRR := httptest.NewRecorder()
	router.ServeHTTP(createRR, compensationDayAPIRequest(t, token, http.MethodPost, employee.ID, "/absences", map[string]interface{}{
		"absence_date": "2026-04-07",
		"absence_type": "compensation_day",
		"half_day":     false,
	}))
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create want 201, got %d: %s", createRR.Code, createRR.Body.String())
	}
	var absence model.Absence
	if err := json.NewDecoder(createRR.Body).Decode(&absence); err != nil {
		t.Fatal(err)
	}

	deleteRR := httptest.NewRecorder()
	router.ServeHTTP(deleteRR, compensationDayAPIRequest(t, token, http.MethodDelete, employee.ID, fmt.Sprintf("/absences/%d", absence.ID), nil))
	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("delete want 204, got %d: %s", deleteRR.Code, deleteRR.Body.String())
	}
	open, err := claims.CountOpen(ctx, employee.ID)
	if err != nil {
		t.Fatal(err)
	}
	if open != 1 {
		t.Fatalf("open claims after delete want 1, got %d", open)
	}
}

func TestWaiveCompensationDayClaim_MarksOpenClaimWaived(t *testing.T) {
	db, auth, token, employee := setupCompensationDayAPITest(t)
	ctx := context.Background()
	claims := sqlite.NewCompensationDayClaimStore(db)
	if err := claims.EnsureForWorkDate(ctx, employee.ID, "2026-04-04", true); err != nil {
		t.Fatal(err)
	}
	claim, err := claims.GetOldestOpen(ctx, employee.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil {
		t.Fatal("expected open claim")
	}

	router := compensationDayEmployeeRouter(db, auth)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, compensationDayAPIRequest(t, token, http.MethodPost, employee.ID, fmt.Sprintf("/compensation-day-claims/%d/waive", claim.ID), nil))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rr.Code, rr.Body.String())
	}

	list, err := claims.ListByUser(ctx, employee.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one claim, got %+v", list)
	}
	if list[0].Status != model.CompensationDayClaimWaived {
		t.Fatalf("claim status want waived, got %q", list[0].Status)
	}
	open, err := claims.CountOpen(ctx, employee.ID)
	if err != nil {
		t.Fatal(err)
	}
	if open != 0 {
		t.Fatalf("open claims want 0, got %d", open)
	}
}

func setupCompensationDayAPITest(t *testing.T) (*sqlite.DB, *authsvc.Service, string, *model.User) {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	auth := authsvc.New("test-secret-key-for-compensation-day-api", 1)
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	lead := &model.User{
		Username:     "lead",
		PasswordHash: "x",
		DisplayName:  "Lead",
		Role:         model.RoleLeitung,
		Active:       true,
	}
	if err := users.Create(ctx, lead); err != nil {
		t.Fatal(err)
	}
	employee := &model.User{
		Username:     "employee",
		PasswordHash: "x",
		DisplayName:  "Employee",
		Role:         model.RoleUser,
		Active:       true,
	}
	if err := users.Create(ctx, employee); err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueToken(lead.ID, lead.Username, string(lead.Role))
	if err != nil {
		t.Fatal(err)
	}
	return db, auth, token, employee
}

func compensationDayEmployeeRouter(db *sqlite.DB, auth *authsvc.Service) http.Handler {
	eh := &EmployeeHandler{
		Users:                 sqlite.NewUserStore(db),
		Auth:                  auth,
		WorkPeriods:           sqlite.NewWorkPeriodStore(db),
		Corrections:           sqlite.NewCorrectionStore(db),
		Absences:              sqlite.NewAbsenceStore(db),
		CompensationDayClaims: sqlite.NewCompensationDayClaimStore(db),
		Holidays:              sqlite.NewHolidayStore(db),
		ClosureDays:           sqlite.NewClosureDayStore(db),
		WeeklyHours:           sqlite.NewWeeklyHoursStore(db),
		FixedNonWorkWeekdays:  sqlite.NewFixedNonWorkWeekdaysStore(db),
		VacationEnt:           sqlite.NewVacationEntitlementStore(db),
		NFCTags:               sqlite.NewNFCTagStore(db),
	}
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(apimw.AuthJWT(auth))
			r.Use(apimw.RequireRole(string(model.RoleLeitung), string(model.RoleSuperadmin)))
			r.Post("/employees/{id}/absences", eh.CreateAbsence)
			r.Delete("/employees/{id}/absences/{absenceId}", eh.DeleteAbsence)
			r.Post("/employees/{id}/compensation-day-claims/{claimId}/waive", eh.WaiveCompensationDayClaim)
		})
	})
	return r
}

func compensationDayAPIRequest(t *testing.T, token string, method string, employeeID int, suffix string, body interface{}) *http.Request {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, fmt.Sprintf("/api/v1/employees/%d%s", employeeID, suffix), reader)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return req
}
