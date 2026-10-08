package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	apimw "nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/model"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	"nfc-time-tracking-server/internal/store/sqlite"
)

type attFixture struct {
	router  http.Handler
	db      *sqlite.DB
	store   *sqlite.AttendanceStore
	mice    *model.Group
	bears   *model.Group
	lead    *model.User
	fach    *model.User
	noQual  *model.User
	kitchen *model.User
	usersBy map[int]*model.User
	ga      *model.GroupAccount
	anna    *model.Child
	ben     *model.Child
	carl    *model.Child
}

const attToday = "2026-04-15"

func newAttFixture(t *testing.T) *attFixture {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	users := sqlite.NewUserStore(db)
	groups := sqlite.NewGroupStore(db)
	kibiz := sqlite.NewKibizStore(db)
	f := &attFixture{db: db, store: sqlite.NewAttendanceStore(db), usersBy: map[int]*model.User{}}
	f.lead = &model.User{Username: "ld", PasswordHash: "x", DisplayName: "Leitung", Role: model.RoleLeitung, Active: true}
	f.fach = &model.User{Username: "fk", PasswordHash: "x", DisplayName: "Fachkraft", Role: model.RoleUser, Active: true}
	f.noQual = &model.User{Username: "nq", PasswordHash: "x", DisplayName: "Ohne Kraft", Role: model.RoleUser, Active: true}
	f.kitchen = &model.User{Username: "hw", PasswordHash: "x", DisplayName: "Küche", Role: model.RoleUser, Active: true}
	for _, u := range []*model.User{f.lead, f.fach, f.noQual, f.kitchen} {
		if err := users.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
		f.usersBy[u.ID] = u
	}
	_ = kibiz.SetQualification(ctx, f.fach.ID, model.QualificationFachkraft)
	_ = kibiz.SetQualification(ctx, f.kitchen.ID, model.QualificationHauswirtschaft)
	f.mice = &model.Group{Name: "Mäuse"}
	f.bears = &model.Group{Name: "Bären"}
	for _, g := range []*model.Group{f.mice, f.bears} {
		if err := groups.Create(ctx, g); err != nil {
			t.Fatal(err)
		}
	}
	f.anna = &model.Child{GroupID: f.mice.ID, FirstName: "Anna", LastName: "A", Active: true}
	f.ben = &model.Child{GroupID: f.mice.ID, FirstName: "Ben", Active: true}
	f.carl = &model.Child{GroupID: f.bears.ID, FirstName: "Carl", Active: true}
	for _, c := range []*model.Child{f.anna, f.ben, f.carl} {
		if err := f.store.CreateChild(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	f.ga = &model.GroupAccount{GroupID: f.mice.ID, Username: "maeuse", PasswordHash: "x", Active: true}
	if err := f.store.CreateGroupAccount(ctx, f.ga); err != nil {
		t.Fatal(err)
	}
	h := &AttendanceHandler{
		Attendance: f.store, Groups: groups, Users: users, Kibiz: kibiz, Auth: authsvc.New("s", 8),
		Now: func() time.Time { return time.Date(2026, 4, 15, 10, 0, 0, 0, time.Local) },
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := req.Context()
			if g := atoiOr0(req.Header.Get("X-Group")); g != 0 {
				ctx = context.WithValue(ctx, apimw.CtxGroupAccountID, g)
				ctx = context.WithValue(ctx, apimw.CtxUserID, 0)
				ctx = context.WithValue(ctx, apimw.CtxRole, model.RoleGroupAccount)
			} else {
				u := f.usersBy[atoiOr0(req.Header.Get("X-User"))]
				ctx = context.WithValue(ctx, apimw.CtxUserID, u.ID)
				ctx = context.WithValue(ctx, apimw.CtxRole, string(u.Role))
			}
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Get("/attendance/access", h.Access)
	r.Get("/attendance", h.List)
	r.Get("/attendance/evacuation", h.Evacuation)
	r.Put("/attendance/children/{id}/days/{date}", h.PutDay)
	r.Get("/attendance/children/{id}/notices", h.ListNotices)
	r.Post("/attendance/children/{id}/notices", h.CreateNotice)
	r.Put("/attendance/notices/{noticeId}", h.UpdateNotice)
	r.Delete("/attendance/notices/{noticeId}", h.DeleteNotice)
	f.router = r
	return f
}

func (f *attFixture) req(t *testing.T, header, id, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set(header, id)
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)
	return rr
}

func (f *attFixture) asUser(t *testing.T, u *model.User, method, path string, body any) *httptest.ResponseRecorder {
	return f.req(t, "X-User", itoa(u.ID), method, path, body)
}

func (f *attFixture) asGroup(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	return f.req(t, "X-Group", itoa(f.ga.ID), method, path, body)
}

type attListResp struct {
	Date   string `json:"date"`
	Groups []struct {
		ID       int `json:"id"`
		Children []struct {
			ID        int                `json:"id"`
			FirstName string             `json:"first_name"`
			ArrivedAt *string            `json:"arrived_at"`
			LeftAt    *string            `json:"left_at"`
			Notice    *model.ChildNotice `json:"notice"`
			Upcoming  int                `json:"upcoming"`
		} `json:"children"`
	} `json:"groups"`
}

func decodeAtt(t *testing.T, rr *httptest.ResponseRecorder) attListResp {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var out attListResp
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAttendance_Access(t *testing.T) {
	f := newAttFixture(t)
	for _, u := range []*model.User{f.lead, f.fach, f.noQual} {
		out := decodeAtt(t, f.asUser(t, u, http.MethodGet, "/attendance", nil))
		if len(out.Groups) != 2 {
			t.Fatalf("%s: want all groups, got %d", u.Username, len(out.Groups))
		}
	}
	if rr := f.asUser(t, f.kitchen, http.MethodGet, "/attendance", nil); rr.Code != http.StatusForbidden {
		t.Fatalf("Hauswirtschaft: %d", rr.Code)
	}

	// Gruppenaccounts sehen und bearbeiten alle Gruppen; ihre eigene ist nur die Startansicht.
	out := decodeAtt(t, f.asGroup(t, http.MethodGet, "/attendance", nil))
	if len(out.Groups) != 2 {
		t.Fatalf("group account sees: %+v", out.Groups)
	}
	if rr := f.asGroup(t, http.MethodPut, "/attendance/children/"+itoa(f.carl.ID)+"/days/"+attToday, map[string]any{"arrived_at": "08:00"}); rr.Code != http.StatusOK {
		t.Fatalf("other group child: %d", rr.Code)
	}
	rr := f.asGroup(t, http.MethodGet, "/attendance/access", nil)
	if !strings.Contains(rr.Body.String(), `"default_group_id":`+itoa(f.mice.ID)) {
		t.Fatalf("default group: %s", rr.Body.String())
	}
	if rr := f.asUser(t, f.lead, http.MethodGet, "/attendance/access", nil); !strings.Contains(rr.Body.String(), `"default_group_id":0`) {
		t.Fatalf("lead without group: %s", rr.Body.String())
	}

	// Gesperrter Gruppenaccount verliert den Zugriff sofort.
	f.ga.Active = false
	_ = f.store.UpdateGroupAccount(context.Background(), f.ga)
	if rr := f.asGroup(t, http.MethodGet, "/attendance", nil); rr.Code != http.StatusUnauthorized {
		t.Fatalf("inactive group account: %d", rr.Code)
	}
}

func TestAttendance_ArriveLeave(t *testing.T) {
	f := newAttFixture(t)
	day := "/attendance/children/" + itoa(f.anna.ID) + "/days/"
	if rr := f.asGroup(t, http.MethodPut, day+attToday, map[string]any{"left_at": "12:00"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("leave without arrive: %d", rr.Code)
	}
	if rr := f.asGroup(t, http.MethodPut, day+attToday, map[string]any{"arrived_at": "8:5"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("bad time: %d", rr.Code)
	}
	if rr := f.asGroup(t, http.MethodPut, day+"2026-04-16", map[string]any{"arrived_at": "08:00"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("future day: %d", rr.Code)
	}
	if rr := f.asGroup(t, http.MethodPut, day+attToday, map[string]any{"arrived_at": "08:05"}); rr.Code != http.StatusOK {
		t.Fatalf("arrive: %d %s", rr.Code, rr.Body.String())
	}
	if rr := f.asGroup(t, http.MethodPut, day+attToday, map[string]any{"arrived_at": "08:05", "left_at": "07:00"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("leave before arrive: %d", rr.Code)
	}
	if rr := f.asUser(t, f.fach, http.MethodPut, day+attToday, map[string]any{"arrived_at": "08:05", "left_at": "12:30"}); rr.Code != http.StatusOK {
		t.Fatalf("leave: %d %s", rr.Code, rr.Body.String())
	}
	out := decodeAtt(t, f.asGroup(t, http.MethodGet, "/attendance?date="+attToday, nil))
	var got string
	for _, c := range out.Groups[0].Children {
		if c.ID == f.anna.ID && c.ArrivedAt != nil && c.LeftAt != nil {
			got = *c.ArrivedAt + "-" + *c.LeftAt
		}
	}
	if got != "08:05-12:30" {
		t.Fatalf("got %q", got)
	}
	// Beides leeren entfernt den Eintrag.
	if rr := f.asGroup(t, http.MethodPut, day+attToday, map[string]any{"arrived_at": nil, "left_at": nil}); rr.Code != http.StatusOK {
		t.Fatalf("clear: %d", rr.Code)
	}
	att, _ := f.store.ListAttendance(context.Background(), attToday)
	if _, ok := att[f.anna.ID]; ok {
		t.Fatal("entry should be removed")
	}
}

func TestAttendance_Notices(t *testing.T) {
	f := newAttFixture(t)
	base := "/attendance/children/" + itoa(f.ben.ID) + "/notices"
	if rr := f.asGroup(t, http.MethodPost, base, map[string]any{"date_from": "2026-04-20", "date_to": "2026-04-18", "reason": "vacation"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("reversed range: %d", rr.Code)
	}
	if rr := f.asGroup(t, http.MethodPost, base, map[string]any{"date_from": "2026-04-20", "reason": "party"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("bad reason: %d", rr.Code)
	}
	rr := f.asGroup(t, http.MethodPost, base, map[string]any{"date_from": "2026-04-20", "date_to": "2026-04-24", "reason": "vacation"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	var vac model.ChildNotice
	_ = json.Unmarshal(rr.Body.Bytes(), &vac)
	// Heute später gebracht (Arzttermin).
	if rr := f.asUser(t, f.fach, http.MethodPost, base, map[string]any{"date_from": attToday, "reason": "other", "arrive_from": "10:30", "note": "Arzt"}); rr.Code != http.StatusCreated {
		t.Fatalf("partial: %d %s", rr.Code, rr.Body.String())
	}

	out := decodeAtt(t, f.asGroup(t, http.MethodGet, "/attendance", nil))
	for _, c := range out.Groups[0].Children {
		if c.ID != f.ben.ID {
			continue
		}
		if c.Notice == nil || c.Notice.ArriveFrom == nil || *c.Notice.ArriveFrom != "10:30" {
			t.Fatalf("today notice: %+v", c.Notice)
		}
		if c.Upcoming != 1 {
			t.Fatalf("upcoming: %d", c.Upcoming)
		}
	}
	out = decodeAtt(t, f.asGroup(t, http.MethodGet, "/attendance?date=2026-04-22", nil))
	for _, c := range out.Groups[0].Children {
		if c.ID == f.ben.ID && (c.Notice == nil || !c.Notice.FullDay() || c.Notice.Reason != model.ChildNoticeVacation) {
			t.Fatalf("vacation day: %+v", c.Notice)
		}
	}

	// Ändern und löschen; andere Gruppe sieht die Meldung nicht.
	path := "/attendance/notices/" + itoa(vac.ID)
	if rr := f.asGroup(t, http.MethodPut, path, map[string]any{"date_from": "2026-04-21", "date_to": "2026-04-22", "reason": "sick"}); rr.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rr.Code, rr.Body.String())
	}
	if rr := f.asUser(t, f.kitchen, http.MethodDelete, path, nil); rr.Code != http.StatusForbidden {
		t.Fatalf("kitchen delete: %d", rr.Code)
	}
	if rr := f.asGroup(t, http.MethodDelete, path, nil); rr.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rr.Code)
	}
	rr = f.asGroup(t, http.MethodGet, base, nil)
	if rr.Code != http.StatusOK || strings.Count(rr.Body.String(), `"id"`) != 1 {
		t.Fatalf("list after delete: %d %s", rr.Code, rr.Body.String())
	}
}

func TestAttendance_Evacuation(t *testing.T) {
	f := newAttFixture(t)
	ctx := context.Background()
	in, out := "08:00", "09:00"
	_ = f.store.PutAttendance(ctx, model.ChildAttendance{ChildID: f.anna.ID, Date: attToday, ArrivedAt: &in})
	_ = f.store.PutAttendance(ctx, model.ChildAttendance{ChildID: f.ben.ID, Date: attToday, ArrivedAt: &in, LeftAt: &out})
	_ = f.store.PutAttendance(ctx, model.ChildAttendance{ChildID: f.carl.ID, Date: attToday, ArrivedAt: &in})

	wps := sqlite.NewWorkPeriodStore(f.db)
	t0 := time.Date(2026, 4, 15, 7, 0, 0, 0, time.Local)
	t1 := t0.Add(2 * time.Hour)
	// Fachkraft eingestempelt, Küche aus- und wieder eingestempelt, Leitung ausgestempelt, ohne Kraft gestern offen.
	_ = wps.ReplaceForUserDate(ctx, f.fach.ID, attToday, []model.WorkPeriod{{UserID: f.fach.ID, WorkDate: attToday, PunchIn: t0}})
	_ = wps.ReplaceForUserDate(ctx, f.kitchen.ID, attToday, []model.WorkPeriod{
		{UserID: f.kitchen.ID, WorkDate: attToday, PunchIn: t0, PunchOut: &t1},
		{UserID: f.kitchen.ID, WorkDate: attToday, PunchIn: t1.Add(time.Hour)},
	})
	_ = wps.ReplaceForUserDate(ctx, f.lead.ID, attToday, []model.WorkPeriod{{UserID: f.lead.ID, WorkDate: attToday, PunchIn: t0, PunchOut: &t1}})
	_ = wps.ReplaceForUserDate(ctx, f.noQual.ID, "2026-04-14", []model.WorkPeriod{{UserID: f.noQual.ID, WorkDate: "2026-04-14", PunchIn: t0.AddDate(0, 0, -1)}})

	rr := f.asGroup(t, http.MethodGet, "/attendance/evacuation", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("evac: %d %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Groups []struct {
			ID       int `json:"id"`
			Children []struct {
				FirstName string `json:"first_name"`
			} `json:"children"`
		} `json:"groups"`
		Staff []map[string]any `json:"staff"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	// Eigene Gruppe zuerst, dann die übrigen.
	if len(resp.Groups) != 2 || resp.Groups[0].ID != f.mice.ID || len(resp.Groups[0].Children) != 1 || resp.Groups[0].Children[0].FirstName != "Anna" {
		t.Fatalf("children: %s", rr.Body.String())
	}
	if len(resp.Staff) != 2 {
		t.Fatalf("staff: %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "07:00") || strings.Contains(rr.Body.String(), "punch") {
		t.Fatalf("staff times must not be shown: %s", rr.Body.String())
	}

	// Korrigierter (geschlossener) Block zählt nicht mehr.
	list, _ := wps.ListByUserDateRange(ctx, f.fach.ID, attToday, attToday)
	_ = sqlite.NewCorrectionStore(f.db).Create(ctx, &model.TimeCorrection{
		WorkPeriodID: list[0].ID, CorrectedIn: t0, CorrectedOut: t1, Reason: "x", CorrectedBy: f.lead.ID,
	})
	ids, _ := f.store.ListPresentStaff(ctx, attToday)
	if len(ids) != 1 || ids[0] != f.kitchen.ID {
		t.Fatalf("present after correction: %v", ids)
	}

	// Mitarbeitende sehen alle Gruppen.
	rr = f.asUser(t, f.fach, http.MethodGet, "/attendance/evacuation", nil)
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Groups) != 2 {
		t.Fatalf("staff evac groups: %s", rr.Body.String())
	}
}

func TestGroupAccountLoginAndToken(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	users := sqlite.NewUserStore(db)
	groups := sqlite.NewGroupStore(db)
	att := sqlite.NewAttendanceStore(db)
	svc := authsvc.New("secret", 8)
	svc.SetGroupAccountLookup(func(ctx context.Context, id int) (bool, int, error) {
		a, err := att.GetGroupAccount(ctx, id)
		if err != nil {
			return false, 0, err
		}
		return a.Active, a.SessionVersion, nil
	})
	g := &model.Group{Name: "Mäuse"}
	_ = groups.Create(ctx, g)
	hash, _ := svc.HashPassword("geheim123")
	ga := &model.GroupAccount{GroupID: g.ID, Username: "flur-maeuse", PasswordHash: hash, Active: true}
	if err := att.CreateGroupAccount(ctx, ga); err != nil {
		t.Fatal(err)
	}
	ah := &AuthHandler{Users: users, Auth: svc, GroupAccounts: att, Groups: groups, PasswordLogin: "admins"}

	login := func(name, pw string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"username": name, "password": pw})
		rr := httptest.NewRecorder()
		ah.Login(rr, httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body)))
		return rr
	}
	if rr := login("flur-maeuse", "falsch"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", rr.Code)
	}
	// Gruppenaccounts dürfen trotz password_login=admins mit Passwort anmelden.
	rr := login("Flur-Maeuse", "geheim123")
	if rr.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rr.Code, rr.Body.String())
	}
	var resp loginResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if string(resp.User.Role) != model.RoleGroupAccount || resp.User.GroupID == nil || *resp.User.GroupID != g.ID || resp.User.DisplayName != "Gruppe Mäuse" {
		t.Fatalf("user: %+v", resp.User)
	}

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	call := func(mw func(http.Handler) http.Handler) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+resp.Token)
		rr := httptest.NewRecorder()
		mw(ok).ServeHTTP(rr, req)
		return rr.Code
	}
	if c := call(apimw.AuthJWT(svc)); c != http.StatusForbidden {
		t.Fatalf("group token on staff routes: %d", c)
	}
	if c := call(apimw.AuthJWTAllowGroup(svc)); c != http.StatusOK {
		t.Fatalf("group token on attendance: %d", c)
	}
	// Neues Passwort meldet angemeldete Geräte ab.
	_ = att.SetGroupAccountPassword(ctx, ga.ID, hash)
	if c := call(apimw.AuthJWTAllowGroup(svc)); c != http.StatusUnauthorized {
		t.Fatalf("after reset: %d", c)
	}
}
