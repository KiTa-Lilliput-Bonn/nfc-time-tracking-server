package handler

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	apimw "nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/apipairing"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	"nfc-time-tracking-server/internal/service/lanemployeesync"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func androidLanTargetsSetting(host string, port int, targetID, apiClientID string) string {
	type row struct {
		ID          string `json:"id"`
		Host        string `json:"host"`
		Port        int    `json:"port"`
		APIClientID string `json:"api_client_id"`
	}
	b, err := json.Marshal([]row{{ID: targetID, Host: host, Port: port, APIClientID: apiClientID}})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// mockLanEmployeesServer simuliert die Flutter-LAN-API (GET /v1/employees, POST Upsert /v1/employee-ids).
type mockLanEmployeesServer struct {
	t       *testing.T
	rows    []mockEmpRow
	secret  string
	handler http.HandlerFunc
}

type mockEmpRow struct {
	ID       string
	Name     string
	NfcTagID string
}

func newMockLanServer(t *testing.T, secret string) *mockLanEmployeesServer {
	m := &mockLanEmployeesServer{t: t, secret: secret}
	m.handler = func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, "Bearer ") || strings.TrimPrefix(authz, "Bearer ") != secret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/v1/employees" && r.Method == http.MethodGet:
			out := make([]map[string]string, 0, len(m.rows))
			for _, e := range m.rows {
				out = append(out, map[string]string{"id": e.ID, "name": e.Name, "nfcTagId": e.NfcTagID})
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(out); err != nil {
				t.Errorf("encode employees: %v", err)
			}
		case r.URL.Path == "/v1/employee-ids" && r.Method == http.MethodPost:
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			id := strings.TrimSpace(body["id"])
			name := strings.TrimSpace(body["name"])
			tag := strings.TrimSpace(body["nfcTagId"])
			for i, e := range m.rows {
				if e.ID == id {
					if e.Name == name && e.NfcTagID == tag {
						w.Header().Set("Content-Type", "text/plain")
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte("unchanged"))
						return
					}
					m.rows[i] = mockEmpRow{ID: id, Name: name, NfcTagID: tag}
					w.Header().Set("Content-Type", "text/plain")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("ok"))
					return
				}
			}
			m.rows = append(m.rows, mockEmpRow{ID: id, Name: name, NfcTagID: tag})
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("ok"))
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/employee-ids/"):
			rest := strings.TrimPrefix(r.URL.Path, "/v1/employee-ids/")
			delID, err := url.PathUnescape(rest)
			if err != nil {
				delID = rest
			}
			delID = strings.TrimSpace(delID)
			if delID == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			next := m.rows[:0]
			for _, e := range m.rows {
				if e.ID != delID {
					next = append(next, e)
				}
			}
			m.rows = next
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}
	return m
}

func TestAndroidLanEmployeeSync_CreatesMissingEmployees(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-secret-lan-sync-test", 1)
	hash, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	if err := users.Create(ctx, &model.User{
		Username: "sa", PasswordHash: hash, DisplayName: "SA",
		Role: model.RoleSuperadmin, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(ctx, &model.User{
		Username: "emp", PasswordHash: hash, DisplayName: "Erika",
		Role: model.RoleUser, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	emp, err := users.GetByUsername(ctx, "emp")
	if err != nil {
		t.Fatal(err)
	}

	ns := sqlite.NewNFCTagStore(db)
	if err := ns.Assign(ctx, &model.NFCTag{
		TagUID:       "TAGAPP",
		UserID:       emp.ID,
		AssignedFrom: "2026-01-01",
	}); err != nil {
		t.Fatal(err)
	}

	cs := sqlite.NewApiPairedClientStore(db)
	cid, err := apipairing.NewID()
	if err != nil {
		t.Fatal(err)
	}
	ac, err := apipairing.BuildClient(cid, "phone", "testsecret")
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.Insert(ctx, ac); err != nil {
		t.Fatal(err)
	}

	mock := newMockLanServer(t, "testsecret")
	api := httptest.NewServer(mock.handler)
	t.Cleanup(api.Close)

	u, _ := url.Parse(api.URL)
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	settings := sqlite.NewSettingsStore(db)
	tid := "sync-target-1"
	if err := settings.Set(ctx, "android_lan_targets", androidLanTargetsSetting(host, port, tid, ac.ID)); err != nil {
		t.Fatal(err)
	}

	sa, err := users.GetByUsername(ctx, "sa")
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueToken(sa.ID, sa.Username, string(sa.Role))
	if err != nil {
		t.Fatal(err)
	}

	h := &AndroidLanEmployeeSyncHandler{Sync: lanemployeesync.NewService(users, ns, settings, cs)}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(apimw.AuthJWT(auth))
		r.Use(apimw.RequireRole(string(model.RoleSuperadmin)))
		r.Post("/android-lan/sync-employee-ids", h.PostSync)
	})

	body := strings.NewReader(`{"target_id":"` + tid + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/android-lan/sync-employee-ids", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var out struct {
		Created []struct {
			UserID int    `json:"user_id"`
			IDStr  string `json:"employee_id"`
		} `json:"created"`
		Updated []interface{} `json:"updated"`
		Skipped []int         `json:"skipped_already_in_app"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Created) != 1 || out.Created[0].UserID != emp.ID || out.Created[0].IDStr != strconv.Itoa(emp.ID) {
		t.Fatalf("unexpected created: %+v", out.Created)
	}
	if len(out.Updated) != 0 {
		t.Fatalf("expected no updates: %+v", out.Updated)
	}
	if len(out.Skipped) != 0 {
		t.Fatalf("expected no skips, got %v", out.Skipped)
	}

	body2 := strings.NewReader(`{"target_id":"` + tid + `"}`)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/android-lan/sync-employee-ids", body2)
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("want 200 second run, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var out2 struct {
		Created []interface{} `json:"created"`
		Updated []interface{} `json:"updated"`
		Skipped []int         `json:"skipped_already_in_app"`
	}
	if err := json.Unmarshal(rr2.Body.Bytes(), &out2); err != nil {
		t.Fatal(err)
	}
	if len(out2.Created) != 0 || len(out2.Updated) != 0 || len(out2.Skipped) != 1 || out2.Skipped[0] != emp.ID {
		t.Fatalf("second run: want skip only, got created=%v updated=%v skipped=%v", out2.Created, out2.Updated, out2.Skipped)
	}
}

func TestAndroidLanEmployeeSync_UpdatesStaleTag(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-secret-lan-sync-test2", 1)
	hash, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	if err := users.Create(ctx, &model.User{
		Username: "sa2", PasswordHash: hash, DisplayName: "SA",
		Role: model.RoleSuperadmin, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(ctx, &model.User{
		Username: "emp2", PasswordHash: hash, DisplayName: "Max",
		Role: model.RoleUser, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	emp, err := users.GetByUsername(ctx, "emp2")
	if err != nil {
		t.Fatal(err)
	}
	idStr := strconv.Itoa(emp.ID)

	ns := sqlite.NewNFCTagStore(db)
	if err := ns.Assign(ctx, &model.NFCTag{
		TagUID:       "SERVER_TAG",
		UserID:       emp.ID,
		AssignedFrom: "2026-01-01",
	}); err != nil {
		t.Fatal(err)
	}

	cs := sqlite.NewApiPairedClientStore(db)
	cid, err := apipairing.NewID()
	if err != nil {
		t.Fatal(err)
	}
	ac, err := apipairing.BuildClient(cid, "t", "sec2")
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.Insert(ctx, ac); err != nil {
		t.Fatal(err)
	}

	mock := newMockLanServer(t, "sec2")
	mock.rows = []mockEmpRow{{ID: idStr, Name: "Max", NfcTagID: "OLD_TAG"}}

	api := httptest.NewServer(mock.handler)
	t.Cleanup(api.Close)

	u, _ := url.Parse(api.URL)
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	settings := sqlite.NewSettingsStore(db)
	tid2 := "sync-target-2"
	_ = settings.Set(ctx, "android_lan_targets", androidLanTargetsSetting(host, port, tid2, ac.ID))

	sa2, _ := users.GetByUsername(ctx, "sa2")
	token, _ := auth.IssueToken(sa2.ID, sa2.Username, string(sa2.Role))

	h := &AndroidLanEmployeeSyncHandler{Sync: lanemployeesync.NewService(users, ns, settings, cs)}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(apimw.AuthJWT(auth))
		r.Use(apimw.RequireRole(string(model.RoleSuperadmin)))
		r.Post("/android-lan/sync-employee-ids", h.PostSync)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/android-lan/sync-employee-ids",
		strings.NewReader(`{"target_id":"`+tid2+`"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var out struct {
		Created []interface{} `json:"created"`
		Updated []struct {
			UserID int `json:"user_id"`
		} `json:"updated"`
		Skipped []int `json:"skipped_already_in_app"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Created) != 0 || len(out.Skipped) != 0 {
		t.Fatalf("want update only: created=%v skipped=%v", out.Created, out.Skipped)
	}
	if len(out.Updated) != 1 || out.Updated[0].UserID != emp.ID {
		t.Fatalf("want one updated row: %+v", out.Updated)
	}
	if mock.rows[0].NfcTagID != "SERVER_TAG" {
		t.Fatalf("mock app should have new tag, got %q", mock.rows[0].NfcTagID)
	}
}

func TestAndroidLanEmployeeSync_RejectsDisallowedLANHost(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-secret-lan-host-test", 1)
	hash, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	if err := users.Create(ctx, &model.User{
		Username: "sa3", PasswordHash: hash, DisplayName: "SA",
		Role: model.RoleSuperadmin, Active: true,
	}); err != nil {
		t.Fatal(err)
	}

	cs := sqlite.NewApiPairedClientStore(db)
	cid, err := apipairing.NewID()
	if err != nil {
		t.Fatal(err)
	}
	ac, err := apipairing.BuildClient(cid, "p", "s3")
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.Insert(ctx, ac); err != nil {
		t.Fatal(err)
	}

	settings := sqlite.NewSettingsStore(db)
	tid3 := "sync-target-3"
	if err := settings.Set(ctx, "android_lan_targets", androidLanTargetsSetting("8.8.8.8", 8787, tid3, ac.ID)); err != nil {
		t.Fatal(err)
	}

	sa, err := users.GetByUsername(ctx, "sa3")
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueToken(sa.ID, sa.Username, string(sa.Role))
	if err != nil {
		t.Fatal(err)
	}

	h := &AndroidLanEmployeeSyncHandler{Sync: lanemployeesync.NewService(users, sqlite.NewNFCTagStore(db), settings, cs)}
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(apimw.AuthJWT(auth))
		r.Use(apimw.RequireRole(string(model.RoleSuperadmin)))
		r.Post("/android-lan/sync-employee-ids", h.PostSync)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/android-lan/sync-employee-ids",
		strings.NewReader(`{"target_id":"`+tid3+`"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for public IP host, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAndroidLanEmployeeSync_RemovesInactiveEmployeeFromApp(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	auth := authsvc.New("jwt-secret-lan-sync-inactive", 1)
	hash, err := auth.HashPassword("pw")
	if err != nil {
		t.Fatal(err)
	}
	users := sqlite.NewUserStore(db)
	ctx := context.Background()
	if err := users.Create(ctx, &model.User{
		Username: "sa4", PasswordHash: hash, DisplayName: "SA",
		Role: model.RoleSuperadmin, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(ctx, &model.User{
		Username: "gone", PasswordHash: hash, DisplayName: "Ex MA",
		Role: model.RoleUser, Active: false,
	}); err != nil {
		t.Fatal(err)
	}
	inactive, err := users.GetByUsername(ctx, "gone")
	if err != nil {
		t.Fatal(err)
	}
	idStr := strconv.Itoa(inactive.ID)

	cs := sqlite.NewApiPairedClientStore(db)
	cid, err := apipairing.NewID()
	if err != nil {
		t.Fatal(err)
	}
	ac, err := apipairing.BuildClient(cid, "phone", "secinactive")
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.Insert(ctx, ac); err != nil {
		t.Fatal(err)
	}

	mock := newMockLanServer(t, "secinactive")
	mock.rows = []mockEmpRow{{ID: idStr, Name: "Ex MA", NfcTagID: "OLD"}}

	api := httptest.NewServer(mock.handler)
	t.Cleanup(api.Close)

	u, _ := url.Parse(api.URL)
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	settings := sqlite.NewSettingsStore(db)
	tid := "sync-target-inactive"
	if err := settings.Set(ctx, "android_lan_targets", androidLanTargetsSetting(host, port, tid, ac.ID)); err != nil {
		t.Fatal(err)
	}

	sa, err := users.GetByUsername(ctx, "sa4")
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueToken(sa.ID, sa.Username, string(sa.Role))
	if err != nil {
		t.Fatal(err)
	}

	h := &AndroidLanEmployeeSyncHandler{Sync: lanemployeesync.NewService(users, sqlite.NewNFCTagStore(db), settings, cs)}

	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(apimw.AuthJWT(auth))
		r.Use(apimw.RequireRole(string(model.RoleSuperadmin)))
		r.Post("/android-lan/sync-employee-ids", h.PostSync)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/android-lan/sync-employee-ids",
		strings.NewReader(`{"target_id":"`+tid+`"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var out struct {
		Removed []struct {
			UserID int    `json:"user_id"`
			IDStr  string `json:"employee_id"`
		} `json:"removed_from_app"`
		Failures []map[string]string `json:"failures"`
		AppIDs   []string            `json:"app_employee_ids_after"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Failures) != 0 {
		t.Fatalf("unexpected failures: %+v", out.Failures)
	}
	if len(out.Removed) != 1 || out.Removed[0].UserID != inactive.ID || out.Removed[0].IDStr != idStr {
		t.Fatalf("removed_from_app: want one row for inactive user, got %+v", out.Removed)
	}
	if len(out.AppIDs) != 0 {
		t.Fatalf("app_employee_ids_after: want empty, got %v", out.AppIDs)
	}
	if len(mock.rows) != 0 {
		t.Fatalf("mock app should have no employees left, got %+v", mock.rows)
	}
}
