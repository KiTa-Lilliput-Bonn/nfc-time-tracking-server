package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/kibizcalc"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestKibizWeekAndChildDays(t *testing.T) {
	db := openHandlerTestDB(t)
	ctx := context.Background()
	gs := sqlite.NewGroupStore(db)
	g := &model.Group{Name: "Raupen"}
	if err := gs.Create(ctx, g); err != nil {
		t.Fatal(err)
	}
	us := sqlite.NewUserStore(db)
	anna := &model.User{Username: "anna", PasswordHash: "x", DisplayName: "Anna", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, anna); err != nil {
		t.Fatal(err)
	}
	anna.GroupID = &g.ID
	if err := us.Update(ctx, anna); err != nil {
		t.Fatal(err)
	}
	ks := sqlite.NewKibizStore(db)
	st := sqlite.NewSettingsStore(db)
	kh := &KibizHandler{Kibiz: ks, Users: us, Groups: gs, Settings: st}

	put := func(fn http.HandlerFunc, path string, body any, params map[string]string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
		if params != nil {
			rc := chi.NewRouteContext()
			for k, v := range params {
				rc.URLParams.Add(k, v)
			}
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
		}
		rr := httptest.NewRecorder()
		fn(rr, req)
		return rr
	}
	if rr := put(kh.PutQualification, "/planning/qualifications/x", map[string]string{"qualification": "fachkraft"},
		map[string]string{"userId": itoa(anna.ID)}); rr.Code != http.StatusOK {
		t.Fatalf("qualification %d %s", rr.Code, rr.Body.String())
	}
	if rr := put(kh.PutQualification, "/planning/qualifications/x", map[string]string{"qualification": "chef"},
		map[string]string{"userId": itoa(anna.ID)}); rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid qualification accepted: %d", rr.Code)
	}
	pattern := model.ChildPattern{GroupID: g.ID, ValidFrom: "2026-09-01", Counts: []model.ChildCount{
		{GroupForm: model.GroupFormIII, CareHours: 35, Count: 25},
	}}
	if rr := put(kh.PutChildPattern, "/planning/child-patterns", pattern, nil); rr.Code != http.StatusOK {
		t.Fatalf("pattern %d %s", rr.Code, rr.Body.String())
	}
	if rr := put(kh.PutChildDays, "/planning/child-days", map[string]any{
		"group_id": g.ID, "dates": []string{"2026-10-07"},
		"counts": []model.ChildCount{{GroupForm: model.GroupFormIII, CareHours: 35, Count: 20}},
	}, nil); rr.Code != http.StatusNoContent {
		t.Fatalf("child days %d %s", rr.Code, rr.Body.String())
	}
	if rr := put(kh.PutOptions, "/planning/kibiz-options", kibizcalc.Options{CountTeamMeetings: true}, nil); rr.Code != http.StatusOK {
		t.Fatalf("options %d", rr.Code)
	}

	ss := sqlite.NewScheduleStore(db)
	if err := ss.Set(ctx, &model.Schedule{UserID: anna.ID, ScheduleDate: "2026-10-05", ShiftStart: "08:00", ShiftEnd: "12:00"}); err != nil {
		t.Fatal(err)
	}
	h := &ScheduleHandler{
		Schedules: ss, Users: us, Groups: gs, Absences: sqlite.NewAbsenceStore(db), Holidays: sqlite.NewHolidayStore(db),
		Closures: sqlite.NewClosureDayStore(db), TeamMeetings: sqlite.NewTeamMeetingStore(db),
		FixedNonWorkWeekdays: sqlite.NewFixedNonWorkWeekdaysStore(db), Settings: st, Kibiz: ks,
	}
	get := func() (out struct {
		Groups  []kibizcalc.Group `json:"groups"`
		Options kibizcalc.Options `json:"options"`
	}) {
		rr := httptest.NewRecorder()
		h.KibizWeek(rr, httptest.NewRequest(http.MethodGet, "/schedules/kibiz?year=2026&week=41", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("kibiz week %d %s", rr.Code, rr.Body.String())
		}
		if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	out := get()
	if !out.Options.CountTeamMeetings || len(out.Groups) != 1 {
		t.Fatalf("out %+v", out)
	}
	gr := out.Groups[0]
	// III/35: 25 Kinder, 38,5 h Fachkraft → Normaltag 7,7 h = 462 min; Mittwoch 20 Kinder = 369,6 min
	if gr.Days[0].NeedFachkraft != 462 || gr.Days[2].NeedFachkraft != 370 || !gr.Days[2].Adjusted {
		t.Fatalf("need %+v", gr.Days)
	}
	if gr.Days[0].PlannedFachkraft != 240 || gr.Week.PlannedFachkraft != 240 {
		t.Fatalf("planned %+v", gr.Week)
	}

	if rr := put(kh.PutChildDays, "/planning/child-days", map[string]any{
		"group_id": g.ID, "dates": []string{"2026-10-07"}, "reset": true,
	}, nil); rr.Code != http.StatusNoContent {
		t.Fatalf("reset %d", rr.Code)
	}
	if gr := get().Groups[0]; gr.Days[2].Adjusted || gr.Days[2].NeedFachkraft != 462 {
		t.Fatalf("after reset %+v", gr.Days[2])
	}
}

func TestKibizRatesSeededAndValidated(t *testing.T) {
	db := openHandlerTestDB(t)
	ks := sqlite.NewKibizStore(db)
	rates, err := ks.ListRates(context.Background())
	if err != nil || len(rates) != 9 {
		t.Fatalf("seed: %v %d", err, len(rates))
	}
	kh := &KibizHandler{Kibiz: ks}
	b, _ := json.Marshal(map[string]any{"rates": []model.KibizRate{
		{GroupForm: model.GroupFormI, CareHours: 25, Children: 20, TotalHours: 50, FachkraftMinHours: 60},
	}})
	rr := httptest.NewRecorder()
	kh.PutRates(rr, httptest.NewRequest(http.MethodPut, "/planning/kibiz-rates", bytes.NewReader(b)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}
