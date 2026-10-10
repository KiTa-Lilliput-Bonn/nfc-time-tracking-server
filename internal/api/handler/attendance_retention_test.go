package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/childretention"
)

func clock(s string) *string { return &s }

func TestChildRetention_PurgeCountsThenDeletes(t *testing.T) {
	f := newAttFixture(t)
	ctx := context.Background()
	put := func(c *model.Child, day string, in, out *string) {
		t.Helper()
		if err := f.store.PutAttendance(ctx, model.ChildAttendance{ChildID: c.ID, Date: day, ArrivedAt: in, LeftAt: out}); err != nil {
			t.Fatal(err)
		}
	}
	// Frist Kommen/Gehen: 3 Monate vor 2026-04-15 → ab 2026-01-15 bleibt alles.
	put(f.anna, "2026-01-10", clock("08:10"), clock("09:20"))
	put(f.ben, "2026-01-10", clock("08:45"), nil)
	put(f.anna, "2026-02-01", clock("08:00"), clock("12:00"))
	// Meldungen: 4 Wochen nach dem letzten Tag → vor 2026-03-18 endende werden gelöscht.
	old := &model.ChildNotice{ChildID: f.anna.ID, DateFrom: "2026-03-01", DateTo: "2026-03-10", Reason: model.ChildNoticeSick}
	recent := &model.ChildNotice{ChildID: f.anna.ID, DateFrom: "2026-03-18", DateTo: "2026-03-20", Reason: model.ChildNoticeVacation}
	for _, n := range []*model.ChildNotice{old, recent} {
		if err := f.store.CreateNotice(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	// Carl ist seit Januar abgemeldet: wird samt Daten gelöscht, sein Tag vorher gezählt.
	put(f.carl, "2026-01-05", clock("07:30"), clock("08:00"))
	if err := f.store.CreateNotice(ctx, &model.ChildNotice{ChildID: f.carl.ID, DateFrom: "2026-05-01", DateTo: "2026-05-01", Reason: model.ChildNoticeVacation}); err != nil {
		t.Fatal(err)
	}
	f.carl.Active = false
	if err := f.store.UpdateChild(ctx, f.carl); err != nil {
		t.Fatal(err)
	}
	if f.carl.DeactivatedAt == nil {
		t.Fatal("deactivated_at not set")
	}
	if _, err := f.db.DB.Exec(`UPDATE children SET deactivated_at = '2026-01-10T09:00:00Z' WHERE id = ?`, f.carl.ID); err != nil {
		t.Fatal(err)
	}
	// Ben ist erst kürzlich abgemeldet und bleibt.
	f.ben.Active = false
	if err := f.store.UpdateChild(ctx, f.ben); err != nil {
		t.Fatal(err)
	}

	svc := &childretention.Service{
		Settings: f.settings, Attendance: f.store,
		Now: func() time.Time { return time.Date(2026, 4, 15, 3, 0, 0, 0, time.Local) },
	}
	res, err := svc.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.AttendanceDays != 3 || res.Notices != 2 || res.Children != 1 {
		t.Fatalf("result %+v", res)
	}
	stats := func() map[string]int {
		rows, err := f.db.DB.Query(`SELECT group_id, day, slot, children FROM child_attendance_stats`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]int{}
		for rows.Next() {
			var g, n int
			var day, slot string
			if err := rows.Scan(&g, &day, &slot, &n); err != nil {
				t.Fatal(err)
			}
			out[itoa(g)+" "+day+" "+slot] = n
		}
		return out
	}
	m, b := itoa(f.mice.ID), itoa(f.bears.ID)
	want := map[string]int{
		m + " 2026-01-10 ": 2, m + " 2026-01-10 08:00": 1, m + " 2026-01-10 08:30": 2, m + " 2026-01-10 09:00": 1,
		b + " 2026-01-05 ": 1, b + " 2026-01-05 07:30": 1,
	}
	got := stats()
	if len(got) != len(want) {
		t.Fatalf("stats %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("stats[%q] = %d, want %d (all: %v)", k, got[k], v, got)
		}
	}
	if day, _ := f.store.ListAttendance(ctx, "2026-02-01"); len(day) != 1 {
		t.Fatal("recent day must stay")
	}
	if day, _ := f.store.ListAttendance(ctx, "2026-01-10"); len(day) != 0 {
		t.Fatal("old day must be gone")
	}
	if _, err := f.store.GetNotice(ctx, recent.ID); err != nil {
		t.Fatal("recent notice must stay")
	}
	if _, err := f.store.GetChild(ctx, f.carl.ID); err == nil {
		t.Fatal("carl must be deleted")
	}
	if _, err := f.store.GetChild(ctx, f.ben.ID); err != nil {
		t.Fatal("ben must stay")
	}

	// Zweiter Lauf ändert nichts mehr.
	res, err = svc.Run(ctx)
	if err != nil || res.AttendanceDays+res.Notices+res.Children != 0 {
		t.Fatalf("second run %+v %v", res, err)
	}
	if got := stats(); got[m+" 2026-01-10 08:30"] != 2 {
		t.Fatalf("stats changed: %v", got)
	}
}

func TestChildRetention_API(t *testing.T) {
	f := newAttFixture(t)
	var access struct {
		OldestDay string `json:"oldest_day"`
	}
	rr := f.asUser(t, f.fach, http.MethodGet, "/attendance/access", nil)
	_ = json.Unmarshal(rr.Body.Bytes(), &access)
	if access.OldestDay != "2026-01-15" {
		t.Fatalf("oldest_day %q", access.OldestDay)
	}
	path := "/attendance/children/" + itoa(f.anna.ID)
	if rr := f.asUser(t, f.fach, http.MethodPut, path+"/days/2026-01-14", map[string]any{"arrived_at": "08:00"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("too old day: %d", rr.Code)
	}
	if rr := f.asUser(t, f.fach, http.MethodPut, path+"/days/2026-01-15", map[string]any{"arrived_at": "08:00"}); rr.Code != http.StatusOK {
		t.Fatalf("oldest kept day: %d %s", rr.Code, rr.Body.String())
	}
	notice := map[string]any{"date_from": "2026-03-01", "date_to": "2026-03-17", "reason": "vacation"}
	if rr := f.asUser(t, f.fach, http.MethodPost, path+"/notices", notice); rr.Code != http.StatusBadRequest {
		t.Fatalf("too old notice: %d", rr.Code)
	}
	notice["date_to"] = "2026-03-18"
	if rr := f.asUser(t, f.fach, http.MethodPost, path+"/notices", notice); rr.Code != http.StatusCreated {
		t.Fatalf("notice within period: %d %s", rr.Code, rr.Body.String())
	}

	if rr := f.asUser(t, f.lead, http.MethodPut, "/children/retention", map[string]any{"times_months": 0, "notice_weeks": 4, "inactive_months": 3}); rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid config: %d", rr.Code)
	}
	if rr := f.asUser(t, f.lead, http.MethodPut, "/children/retention", map[string]any{"times_months": 6, "notice_weeks": 8, "inactive_months": 12}); rr.Code != http.StatusOK {
		t.Fatalf("save config: %d %s", rr.Code, rr.Body.String())
	}
	var cfg childretention.Config
	_ = json.Unmarshal(f.asUser(t, f.lead, http.MethodGet, "/children/retention", nil).Body.Bytes(), &cfg)
	if cfg != (childretention.Config{TimesMonths: 6, NoticeWeeks: 8, InactiveMonths: 12}) {
		t.Fatalf("config %+v", cfg)
	}
	rr = f.asUser(t, f.fach, http.MethodGet, "/attendance/access", nil)
	_ = json.Unmarshal(rr.Body.Bytes(), &access)
	if access.OldestDay != "2025-10-15" {
		t.Fatalf("oldest_day after change %q", access.OldestDay)
	}
}
