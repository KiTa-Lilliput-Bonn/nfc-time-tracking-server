package stampspoll

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/bootstrap"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/apipairing"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestParseLANStampsResponse_roundTrip(t *testing.T) {
	body := `[{"employeeId":"42","timestamp":"2026-03-26T08:00:00Z","timeZoneIana":"Europe/Berlin","utcOffsetMinutes":60}]`
	wires, err := ParseLANStampsResponse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(wires) != 1 {
		t.Fatalf("want 1 wire, got %d", len(wires))
	}
	rp, err := WireToRawPunch(wires[0], "TAGX")
	if err != nil {
		t.Fatal(err)
	}
	if rp.NFCTagUID != "TAGX" || rp.PunchTime.UTC().Hour() != 8 {
		t.Fatalf("unexpected punch %+v", rp)
	}
}

func lanTargetsJSON(host, portStr, targetID, apiClientID string) string {
	port, _ := strconv.Atoi(portStr)
	return fmt.Sprintf(`[{"id":%q,"host":%q,"port":%d,"api_client_id":%q}]`, targetID, host, port, apiClientID)
}

func TestRunPoll_watermarkQuery(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	settings := sqlite.NewSettingsStore(db)
	us := sqlite.NewUserStore(db)
	ns := sqlite.NewNFCTagStore(db)
	ps := sqlite.NewPunchStore(db)
	ws := sqlite.NewWorkPeriodStore(db)
	cs := sqlite.NewCompensationDayClaimStore(db)
	apiStore := sqlite.NewApiPairedClientStore(db)

	u := &model.User{Username: "emp", PasswordHash: "x", DisplayName: "E", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := ns.Assign(ctx, &model.NFCTag{TagUID: "NFC1", UserID: u.ID, AssignedFrom: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	uidStr := strconv.Itoa(u.ID)

	ac, err := apipairing.BuildClient("polltestclient0001", "t", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if err := apiStore.Insert(ctx, ac); err != nil {
		t.Fatal(err)
	}
	tid := "target-wm-1"

	var lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health":
			_, _ = io.WriteString(w, "ok")
		case r.URL.Path == "/v1/stamps":
			if r.Method == http.MethodGet {
				lastPath = r.URL.RawQuery
			}
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`[{"employeeId":"` + uidStr + `","timestamp":"2026-03-26T09:00:00Z","timeZoneIana":"","utcOffsetMinutes":null}]`))
			} else if r.Method == http.MethodPost {
				_, _ = w.Write([]byte(`{"inserted":0,"skipped":0}`))
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")
	host, portStr, ok := strings.Cut(host, ":")
	if !ok {
		t.Fatalf("bad test server url %q", srv.URL)
	}
	_ = settings.Set(ctx, "android_lan_targets", lanTargetsJSON(host, portStr, tid, ac.ID))
	_ = settings.Set(ctx, "stamps_poll_interval_seconds", "300")
	_ = settings.Set(ctx, WatermarkSettingKey(tid), "2026-03-26T08:00:00Z")

	svc := NewService(settings, apiStore, ps, ws, ns, cs, nil, nil)
	svc.SetHTTPClient(srv.Client())

	if err := svc.RunPoll(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(lastPath, "from=") {
		t.Fatalf("expected from= in query, got %q", lastPath)
	}
}

func TestMaskSensitiveSettings_stripsWatermark(t *testing.T) {
	list := []model.Setting{
		{Key: "stamps_poll_watermark_utc_tid", Value: "secret"},
		{Key: "android_lan_targets", Value: "[]"},
	}
	out := bootstrap.MaskSensitiveSettings(list)
	if len(out) != 1 || out[0].Key != "android_lan_targets" {
		t.Fatalf("unexpected %v", out)
	}
}

func TestMaskSensitiveSettings_masksBackupResticPassword(t *testing.T) {
	list := []model.Setting{
		{Key: "backup_restic_password", Value: "secret123"},
	}
	out := bootstrap.MaskSensitiveSettings(list)
	if len(out) != 1 || out[0].Value != bootstrap.MaskedSecretPlaceholder {
		t.Fatalf("got %+v", out)
	}
}

func TestProbeHealth_ok(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	settings := sqlite.NewSettingsStore(db)
	apiStore := sqlite.NewApiPairedClientStore(db)
	ac, err := apipairing.BuildClient("probeclient0000001", "t", "x")
	if err != nil {
		t.Fatal(err)
	}
	if err := apiStore.Insert(ctx, ac); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = io.WriteString(w, "ok")
		}
	}))
	t.Cleanup(srv.Close)
	host, portStr, ok := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://"), ":")
	if !ok {
		t.Fatal(srv.URL)
	}
	tid := "probe-t1"
	_ = settings.Set(ctx, "android_lan_targets", lanTargetsJSON(host, portStr, tid, ac.ID))
	_ = settings.Set(ctx, "stamps_poll_interval_seconds", "60")

	svc := NewService(settings, apiStore, sqlite.NewPunchStore(db), sqlite.NewWorkPeriodStore(db), sqlite.NewNFCTagStore(db), sqlite.NewCompensationDayClaimStore(db), sqlite.NewUserStore(db), nil)
	svc.SetHTTPClient(srv.Client())
	tgt := bootstrap.AndroidLanTarget{ID: tid, Host: host, Port: mustAtoi(t, portStr), APIClientID: ac.ID}
	if err := svc.ProbeHealth(ctx, tgt); err != nil {
		t.Fatal(err)
	}
	st := svc.LanHealth(ctx)
	if st.Mode != "ok" || !st.Reachable {
		t.Fatalf("%+v", st)
	}
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestLanHealth_disabled(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	settings := sqlite.NewSettingsStore(db)
	svc := NewService(settings, nil, sqlite.NewPunchStore(db), sqlite.NewWorkPeriodStore(db), sqlite.NewNFCTagStore(db), sqlite.NewCompensationDayClaimStore(db), sqlite.NewUserStore(db), nil)
	st := svc.LanHealth(ctx)
	if st.Mode != "disabled" {
		t.Fatalf("%+v", st)
	}
}

func TestRunPoll_marksUnreachableOnBadHTTP(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	settings := sqlite.NewSettingsStore(db)
	apiStore := sqlite.NewApiPairedClientStore(db)
	ac, err := apipairing.BuildClient("badhttpclient00001", "t", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if err := apiStore.Insert(ctx, ac); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	host, portStr, ok := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://"), ":")
	if !ok {
		t.Fatal(srv.URL)
	}
	tid := "bad-t1"
	_ = settings.Set(ctx, "android_lan_targets", lanTargetsJSON(host, portStr, tid, ac.ID))
	_ = settings.Set(ctx, "stamps_poll_interval_seconds", "60")

	svc := NewService(settings, apiStore, sqlite.NewPunchStore(db), sqlite.NewWorkPeriodStore(db), sqlite.NewNFCTagStore(db), sqlite.NewCompensationDayClaimStore(db), sqlite.NewUserStore(db), nil)
	svc.SetHTTPClient(srv.Client())
	_ = svc.RunPoll(ctx)
	st := svc.LanHealth(ctx)
	if st.Mode != "down" || st.Reachable {
		t.Fatalf("%+v", st)
	}
	if st.LastCheckUTC == nil || time.Since(*st.LastCheckUTC) > time.Minute {
		t.Fatalf("expected recent last_check")
	}
}

func TestLanHealthPayload_JSON(t *testing.T) {
	p := LanHealthPayload{
		Mode: "ok", Reachable: true,
		Targets: []LanTargetHealth{{ID: "a", Mode: "ok", Reachable: true}},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"targets"`) {
		t.Fatalf("%s", b)
	}
}

func TestRunPoll_pushesTodayStampsLANPOSTBerlin(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip(err)
	}

	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	settings := sqlite.NewSettingsStore(db)
	us := sqlite.NewUserStore(db)
	ns := sqlite.NewNFCTagStore(db)
	ps := sqlite.NewPunchStore(db)
	ws := sqlite.NewWorkPeriodStore(db)
	cs := sqlite.NewCompensationDayClaimStore(db)
	apiStore := sqlite.NewApiPairedClientStore(db)

	u := &model.User{Username: "pushu", PasswordHash: "x", DisplayName: "P", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := ns.Assign(ctx, &model.NFCTag{TagUID: "NFCX", UserID: u.ID, AssignedFrom: "2020-01-01"}); err != nil {
		t.Fatal(err)
	}

	todayUTC := time.Now().UTC()
	utcDate := todayUTC.Format("2006-01-02")
	wantInstant := time.Date(todayUTC.Year(), todayUTC.Month(), todayUTC.Day(), 11, 22, 33, 444000000, time.UTC)
	if _, err := ps.InsertBatch(ctx, []model.RawPunch{{
		PunchTime: wantInstant, NFCTagUID: "NFCX", SourceFile: "test", DeviceName: "d",
	}}); err != nil {
		t.Fatal(err)
	}

	ac, err := apipairing.BuildClient("pushlanclient00001", "t", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if err := apiStore.Insert(ctx, ac); err != nil {
		t.Fatal(err)
	}
	tid := "push-target-1"

	var postBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_, _ = io.WriteString(w, "ok")
		case "/v1/stamps":
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			if r.Method == http.MethodPost {
				postBody, _ = io.ReadAll(io.LimitReader(r.Body, 1<<20))
				_, _ = w.Write([]byte(`{"inserted":1,"skipped":0}`))
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")
	host, portStr, ok := strings.Cut(host, ":")
	if !ok {
		t.Fatalf("bad test server url %q", srv.URL)
	}
	_ = settings.Set(ctx, "android_lan_targets", lanTargetsJSON(host, portStr, tid, ac.ID))
	_ = settings.Set(ctx, "stamps_poll_interval_seconds", "300")

	svc := NewService(settings, apiStore, ps, ws, ns, cs, nil, nil)
	svc.SetHTTPClient(srv.Client())

	if err := svc.RunPoll(ctx); err != nil {
		t.Fatal(err)
	}
	if len(postBody) == 0 {
		t.Fatal("expected POST /v1/stamps body")
	}

	var wrap struct {
		Stamps []struct {
			EmployeeID       string `json:"employeeId"`
			Timestamp        string `json:"timestamp"`
			TimeZoneIana     string `json:"timeZoneIana"`
			UtcOffsetMinutes int    `json:"utcOffsetMinutes"`
		} `json:"stamps"`
	}
	if err := json.Unmarshal(postBody, &wrap); err != nil {
		t.Fatalf("post body: %v\n%s", err, string(postBody))
	}
	if len(wrap.Stamps) != 1 {
		t.Fatalf("want 1 stamp, got %d", len(wrap.Stamps))
	}
	st := wrap.Stamps[0]
	if st.EmployeeID != strconv.Itoa(u.ID) {
		t.Fatalf("employeeId %q", st.EmployeeID)
	}
	if st.TimeZoneIana != "Europe/Berlin" {
		t.Fatalf("timeZoneIana %q", st.TimeZoneIana)
	}
	if strings.HasSuffix(strings.TrimSpace(st.Timestamp), "Z") {
		t.Fatalf("expected Berlin offset in timestamp, got %q", st.Timestamp)
	}
	parsed, err := time.Parse(time.RFC3339Nano, st.Timestamp)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, st.Timestamp)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.UTC().Equal(wantInstant) {
		t.Fatalf("instant: want %v got %v", wantInstant, parsed.UTC())
	}
	_, off := wantInstant.In(berlin).Zone()
	wantOff := off / 60
	if st.UtcOffsetMinutes != wantOff {
		t.Fatalf("utcOffsetMinutes want %d got %d", wantOff, st.UtcOffsetMinutes)
	}
	wantTS := wantInstant.In(berlin).Format(time.RFC3339Nano)
	if st.Timestamp != wantTS {
		t.Fatalf("timestamp string: want %q got %q", wantTS, st.Timestamp)
	}
	_ = utcDate
}

func TestRunManualRangeSync_rejects15InclusiveBerlinDays(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	settings := sqlite.NewSettingsStore(db)
	svc := NewService(settings, nil, sqlite.NewPunchStore(db), sqlite.NewWorkPeriodStore(db), sqlite.NewNFCTagStore(db), sqlite.NewCompensationDayClaimStore(db), sqlite.NewUserStore(db), nil)

	from, err := ParseBerlinYMD("2026-01-01")
	if err != nil {
		t.Fatal(err)
	}
	to, err := ParseBerlinYMD("2026-01-15")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.RunManualRangeSync(ctx, from, to)
	if !errors.Is(err, ErrManualRangeTooLarge) {
		t.Fatalf("want ErrManualRangeTooLarge, got %v", err)
	}
}

func TestRunManualRangeSync_doesNotUpdateWatermark(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	settings := sqlite.NewSettingsStore(db)
	us := sqlite.NewUserStore(db)
	ns := sqlite.NewNFCTagStore(db)
	ps := sqlite.NewPunchStore(db)
	ws := sqlite.NewWorkPeriodStore(db)
	cs := sqlite.NewCompensationDayClaimStore(db)
	apiStore := sqlite.NewApiPairedClientStore(db)

	u := &model.User{Username: "rman", PasswordHash: "x", DisplayName: "R", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := ns.Assign(ctx, &model.NFCTag{TagUID: "NFCM", UserID: u.ID, AssignedFrom: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	uidStr := strconv.Itoa(u.ID)

	ac, err := apipairing.BuildClient("rangesyncclient0001", "t", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if err := apiStore.Insert(ctx, ac); err != nil {
		t.Fatal(err)
	}
	tid := "range-target-1"
	wmKey := WatermarkSettingKey(tid)
	if err := settings.Set(ctx, wmKey, "2026-03-26T08:00:00Z"); err != nil {
		t.Fatal(err)
	}

	var lastGETQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/stamps":
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.Method == http.MethodGet {
				lastGETQuery = r.URL.RawQuery
				_, _ = w.Write([]byte(`[{"employeeId":"` + uidStr + `","timestamp":"2026-03-26T09:00:00Z","timeZoneIana":"","utcOffsetMinutes":null}]`))
				return
			}
			if r.Method == http.MethodPost {
				_, _ = w.Write([]byte(`{"inserted":0,"skipped":0}`))
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")
	host, portStr, ok := strings.Cut(host, ":")
	if !ok {
		t.Fatalf("bad test server url %q", srv.URL)
	}
	if err := settings.Set(ctx, "android_lan_targets", lanTargetsJSON(host, portStr, tid, ac.ID)); err != nil {
		t.Fatal(err)
	}
	if err := settings.Set(ctx, "stamps_poll_interval_seconds", "300"); err != nil {
		t.Fatal(err)
	}

	svc := NewService(settings, apiStore, ps, ws, ns, cs, nil, nil)
	svc.SetHTTPClient(srv.Client())

	from, err := ParseBerlinYMD("2026-03-26")
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.RunManualRangeSync(ctx, from, from)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || len(res.Targets) != 1 {
		t.Fatalf("unexpected result %+v", res)
	}
	if res.Targets[0].PullError != "" {
		t.Fatalf("pull error: %q", res.Targets[0].PullError)
	}
	wmAfter, _ := settings.Get(ctx, wmKey)
	if strings.TrimSpace(wmAfter) != "2026-03-26T08:00:00Z" {
		t.Fatalf("watermark changed to %q", wmAfter)
	}
	if !strings.Contains(lastGETQuery, "from=") || !strings.Contains(lastGETQuery, "to=") {
		t.Fatalf("expected from= and to= in GET query, got %q", lastGETQuery)
	}
}
