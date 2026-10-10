package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/model"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	"nfc-time-tracking-server/internal/service/childretention"
	"nfc-time-tracking-server/internal/store"
)

// AttendanceHandler bedient die Anwesenheitsliste der Kinder. Zugriff: Gruppenaccounts auf ihre Gruppe,
// pädagogisches Personal und Leitung auf alle Gruppen. Kinder und Gruppenaccounts verwaltet die Leitung.
type AttendanceHandler struct {
	Attendance store.AttendanceStore
	Groups     store.GroupStore
	Users      store.UserStore
	Kibiz      store.KibizStore
	Auth       *authsvc.Service
	Audit      *audit.Logger
	// Settings: Löschfristen (childretention); nil = Standardfristen.
	Settings store.SettingsStore
	Now      func() time.Time
}

func (h *AttendanceHandler) retention(r *http.Request) childretention.Config {
	if h.Settings == nil {
		return childretention.Default
	}
	return childretention.ReadConfig(r.Context(), h.Settings)
}

func (h *AttendanceHandler) now() time.Time {
	if h.Now != nil {
		return h.Now().In(time.Local)
	}
	return time.Now().In(time.Local)
}

func (h *AttendanceHandler) today() string {
	return h.now().Format("2006-01-02")
}

type attendanceActor struct {
	groupAccountID int
	groups         []model.Group
	canManage      bool
	// defaultGroupID: Gruppe des Gruppenaccounts bzw. der Person; 0 = keine (Ansicht „Alle Gruppen“).
	defaultGroupID int
}

func (a *attendanceActor) hasGroup(id int) bool {
	for _, g := range a.groups {
		if g.ID == id {
			return true
		}
	}
	return false
}

func (a *attendanceActor) group(id int) *model.Group {
	for i := range a.groups {
		if a.groups[i].ID == id {
			return &a.groups[i]
		}
	}
	return nil
}

// actor ermittelt die sichtbaren Gruppen. Alle mit Zugriff (auch Gruppenaccounts) sehen und bearbeiten
// alle Gruppen; die eigene Gruppe ist nur die Startansicht. Schreibt die Fehlerantwort und liefert nil
// ohne Zugriff.
func (h *AttendanceHandler) actor(w http.ResponseWriter, r *http.Request) *attendanceActor {
	ctx := r.Context()
	if gaID := middleware.GroupAccountID(r); gaID != 0 {
		ga, err := h.Attendance.GetGroupAccount(ctx, gaID)
		if err != nil || !ga.Active {
			response.Error(w, http.StatusUnauthorized, "unauthorized")
			return nil
		}
		groups, err := h.Groups.List(ctx)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "query failed")
			return nil
		}
		return &attendanceActor{groupAccountID: ga.ID, groups: groups, defaultGroupID: ga.GroupID}
	}
	u, err := h.Users.GetByID(ctx, middleware.UserID(r))
	if err != nil || !u.Active {
		response.Error(w, http.StatusUnauthorized, "unauthorized")
		return nil
	}
	var q model.Qualification
	hasQ := false
	if h.Kibiz != nil {
		quals, err := h.Kibiz.ListQualifications(ctx)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "query failed")
			return nil
		}
		q, hasQ = quals[u.ID]
	}
	if !model.AttendanceStaffAccess(u.Role, q, hasQ) {
		response.Error(w, http.StatusForbidden, "Kein Zugriff auf die Anwesenheitsliste.")
		return nil
	}
	groups, err := h.Groups.List(ctx)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return nil
	}
	a := &attendanceActor{groups: groups, canManage: isLeitungRole(string(u.Role))}
	if u.GroupID != nil && a.hasGroup(*u.GroupID) {
		a.defaultGroupID = *u.GroupID
	}
	return a
}

func (h *AttendanceHandler) audit(r *http.Request, action, entity, id string, summary map[string]any) {
	if gaID := middleware.GroupAccountID(r); gaID != 0 {
		summary["group_account_id"] = gaID
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: action, EntityType: entity, EntityID: id, Summary: audit.JSONSummary(summary),
	})
}

type attendanceGroupRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Access liefert die sichtbaren Gruppen und ob Kinder verwaltet werden dürfen (GET /attendance/access).
func (h *AttendanceHandler) Access(w http.ResponseWriter, r *http.Request) {
	a := h.actor(w, r)
	if a == nil {
		return
	}
	groups := make([]attendanceGroupRef, 0, len(a.groups))
	for _, g := range a.groups {
		groups = append(groups, attendanceGroupRef{ID: g.ID, Name: g.Name})
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"groups":           groups,
		"can_manage":       a.canManage,
		"default_group_id": a.defaultGroupID,
		"is_group_account": a.groupAccountID != 0,
		"today":            h.today(),
		// Ältere Tage sind nach der Löschfrist gelöscht und lassen sich nicht mehr bearbeiten.
		"oldest_day": h.retention(r).TimesFrom(h.now()),
	})
}

type attendanceRow struct {
	ID        int                `json:"id"`
	GroupID   int                `json:"group_id"`
	FirstName string             `json:"first_name"`
	LastName  string             `json:"last_name"`
	ArrivedAt *string            `json:"arrived_at"`
	LeftAt    *string            `json:"left_at"`
	Notice    *model.ChildNotice `json:"notice"`
	// Upcoming: Anzahl gemeldeter Abwesenheiten ab morgen (Hinweis in der Liste).
	Upcoming int `json:"upcoming"`
}

type attendanceGroup struct {
	ID       int             `json:"id"`
	Name     string          `json:"name"`
	Children []attendanceRow `json:"children"`
}

func parseDateParam(s string) (string, bool) {
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return "", false
	}
	return s, true
}

// List liefert die Anwesenheit eines Tages je Gruppe (GET /attendance?date=&group_id=).
func (h *AttendanceHandler) List(w http.ResponseWriter, r *http.Request) {
	a := h.actor(w, r)
	if a == nil {
		return
	}
	ctx := r.Context()
	date := h.today()
	if v := r.URL.Query().Get("date"); v != "" {
		d, ok := parseDateParam(v)
		if !ok {
			response.Error(w, http.StatusBadRequest, "invalid date")
			return
		}
		date = d
	}
	onlyGroup := 0
	if v := r.URL.Query().Get("group_id"); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil || !a.hasGroup(id) {
			response.Error(w, http.StatusForbidden, "forbidden")
			return
		}
		onlyGroup = id
	}
	children, err := h.Attendance.ListChildren(ctx, true)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	att, err := h.Attendance.ListAttendance(ctx, date)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	notices, err := h.Attendance.ListNoticesForDate(ctx, date)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	noticeBy := map[int]*model.ChildNotice{}
	for i := range notices {
		n := &notices[i]
		// Bei mehreren Meldungen hat ein ganzer Fehltag Vorrang.
		if cur, ok := noticeBy[n.ChildID]; !ok || (!cur.FullDay() && n.FullDay()) {
			noticeBy[n.ChildID] = n
		}
	}
	ahead, err := h.Attendance.ListNoticesEndingFrom(ctx, h.now().AddDate(0, 0, 1).Format("2006-01-02"))
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	upcoming := map[int]int{}
	for _, n := range ahead {
		upcoming[n.ChildID]++
	}
	out := []attendanceGroup{}
	idx := map[int]int{}
	for _, g := range a.groups {
		if onlyGroup != 0 && g.ID != onlyGroup {
			continue
		}
		idx[g.ID] = len(out)
		out = append(out, attendanceGroup{ID: g.ID, Name: g.Name, Children: []attendanceRow{}})
	}
	for _, c := range children {
		gi, ok := idx[c.GroupID]
		if !ok {
			continue
		}
		rec, hasRec := att[c.ID]
		// Abgemeldete Kinder nur an Tagen zeigen, an denen sie noch eingetragen sind.
		if !c.Active && !hasRec {
			continue
		}
		out[gi].Children = append(out[gi].Children, attendanceRow{
			ID: c.ID, GroupID: c.GroupID, FirstName: c.FirstName, LastName: c.LastName,
			ArrivedAt: rec.ArrivedAt, LeftAt: rec.LeftAt, Notice: noticeBy[c.ID], Upcoming: upcoming[c.ID],
		})
	}
	response.JSON(w, http.StatusOK, map[string]any{"date": date, "today": h.today(), "groups": out})
}

// childForActor lädt Kind {id} und prüft, ob der Aufrufer dessen Gruppe sieht.
func (h *AttendanceHandler) childForActor(w http.ResponseWriter, r *http.Request, a *attendanceActor, param string) *model.Child {
	id, err := strconv.Atoi(chi.URLParam(r, param))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return nil
	}
	c, err := h.Attendance.GetChild(r.Context(), id)
	if err != nil || !a.hasGroup(c.GroupID) {
		response.Error(w, http.StatusNotFound, "Kind nicht gefunden")
		return nil
	}
	return c
}

// PutDay setzt Kommen/Gehen eines Kindes an einem Tag (PUT /attendance/children/{id}/days/{date}).
func (h *AttendanceHandler) PutDay(w http.ResponseWriter, r *http.Request) {
	a := h.actor(w, r)
	if a == nil {
		return
	}
	c := h.childForActor(w, r, a, "id")
	if c == nil {
		return
	}
	date, ok := parseDateParam(chi.URLParam(r, "date"))
	if !ok {
		response.Error(w, http.StatusBadRequest, "invalid date")
		return
	}
	if date > h.today() {
		response.Error(w, http.StatusBadRequest, "Kommen und Gehen lassen sich nicht für künftige Tage eintragen.")
		return
	}
	if date < h.retention(r).TimesFrom(h.now()) {
		response.Error(w, http.StatusBadRequest, "Dieser Tag liegt hinter der Löschfrist und wird nicht mehr gespeichert.")
		return
	}
	var body struct {
		ArrivedAt *string `json:"arrived_at"`
		LeftAt    *string `json:"left_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	rec := model.ChildAttendance{ChildID: c.ID, Date: date, ArrivedAt: body.ArrivedAt, LeftAt: body.LeftAt}
	if err := rec.Validate(); err != nil {
		msg := "Ungültige Uhrzeit."
		if strings.Contains(err.Error(), "before") {
			msg = "Gehen darf nicht vor Kommen liegen."
		} else if strings.Contains(err.Error(), "requires") {
			msg = "Bitte zuerst Kommen eintragen."
		}
		response.Error(w, http.StatusBadRequest, msg)
		return
	}
	if err := h.Attendance.PutAttendance(r.Context(), rec); err != nil {
		response.Error(w, http.StatusInternalServerError, "save failed")
		return
	}
	h.audit(r, audit.ActionUpdate, audit.EntityChildAttendance, strconv.Itoa(c.ID)+":"+date,
		map[string]any{"arrived_at": rec.ArrivedAt, "left_at": rec.LeftAt})
	response.JSON(w, http.StatusOK, rec)
}

// ListNotices liefert die gemeldeten Abwesenheiten eines Kindes ab heute (GET /attendance/children/{id}/notices).
func (h *AttendanceHandler) ListNotices(w http.ResponseWriter, r *http.Request) {
	a := h.actor(w, r)
	if a == nil {
		return
	}
	c := h.childForActor(w, r, a, "id")
	if c == nil {
		return
	}
	list, err := h.Attendance.ListNoticesForChild(r.Context(), c.ID, h.today())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, list)
}

type noticeBody struct {
	DateFrom   string                  `json:"date_from"`
	DateTo     string                  `json:"date_to"`
	Reason     model.ChildNoticeReason `json:"reason"`
	ArriveFrom *string                 `json:"arrive_from"`
	LeaveAt    *string                 `json:"leave_at"`
	Note       string                  `json:"note"`
}

func (h *AttendanceHandler) decodeNotice(w http.ResponseWriter, r *http.Request, n *model.ChildNotice) bool {
	var body noticeBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return false
	}
	if body.DateTo == "" {
		body.DateTo = body.DateFrom
	}
	n.DateFrom, n.DateTo, n.Reason = body.DateFrom, body.DateTo, body.Reason
	n.ArriveFrom, n.LeaveAt, n.Note = body.ArriveFrom, body.LeaveAt, body.Note
	if err := n.Validate(); err != nil {
		msg := "Bitte Zeitraum und Grund prüfen."
		switch {
		case strings.Contains(err.Error(), "date_to before"):
			msg = "Das Ende liegt vor dem Beginn."
		case strings.Contains(err.Error(), "leave_at must be after"):
			msg = "Die Abholzeit muss nach der Bringzeit liegen."
		case strings.Contains(err.Error(), "invalid time"):
			msg = "Ungültige Uhrzeit."
		case strings.Contains(err.Error(), "range too long"):
			msg = "Der Zeitraum darf höchstens ein Jahr lang sein."
		}
		response.Error(w, http.StatusBadRequest, msg)
		return false
	}
	if n.DateTo < h.retention(r).NoticesFrom(h.now()) {
		response.Error(w, http.StatusBadRequest, "Diese Meldung liegt hinter der Löschfrist und wird nicht mehr gespeichert.")
		return false
	}
	return true
}

func noticeSummary(n *model.ChildNotice) map[string]any {
	return map[string]any{
		"child_id": n.ChildID, "date_from": n.DateFrom, "date_to": n.DateTo, "reason": n.Reason,
		"arrive_from": n.ArriveFrom, "leave_at": n.LeaveAt,
	}
}

// CreateNotice meldet ein Fehlen (POST /attendance/children/{id}/notices).
func (h *AttendanceHandler) CreateNotice(w http.ResponseWriter, r *http.Request) {
	a := h.actor(w, r)
	if a == nil {
		return
	}
	c := h.childForActor(w, r, a, "id")
	if c == nil {
		return
	}
	n := model.ChildNotice{ChildID: c.ID}
	if !h.decodeNotice(w, r, &n) {
		return
	}
	if err := h.Attendance.CreateNotice(r.Context(), &n); err != nil {
		response.Error(w, http.StatusInternalServerError, "save failed")
		return
	}
	h.audit(r, audit.ActionCreate, audit.EntityChildNotice, strconv.Itoa(n.ID), noticeSummary(&n))
	response.JSON(w, http.StatusCreated, n)
}

func (h *AttendanceHandler) noticeForActor(w http.ResponseWriter, r *http.Request, a *attendanceActor) *model.ChildNotice {
	id, err := strconv.Atoi(chi.URLParam(r, "noticeId"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return nil
	}
	n, err := h.Attendance.GetNotice(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusNotFound, "Meldung nicht gefunden")
		return nil
	}
	c, err := h.Attendance.GetChild(r.Context(), n.ChildID)
	if err != nil || !a.hasGroup(c.GroupID) {
		response.Error(w, http.StatusNotFound, "Meldung nicht gefunden")
		return nil
	}
	return n
}

// UpdateNotice ändert eine Meldung (PUT /attendance/notices/{noticeId}).
func (h *AttendanceHandler) UpdateNotice(w http.ResponseWriter, r *http.Request) {
	a := h.actor(w, r)
	if a == nil {
		return
	}
	n := h.noticeForActor(w, r, a)
	if n == nil {
		return
	}
	if !h.decodeNotice(w, r, n) {
		return
	}
	if err := h.Attendance.UpdateNotice(r.Context(), n); err != nil {
		response.Error(w, http.StatusInternalServerError, "save failed")
		return
	}
	h.audit(r, audit.ActionUpdate, audit.EntityChildNotice, strconv.Itoa(n.ID), noticeSummary(n))
	response.JSON(w, http.StatusOK, n)
}

// DeleteNotice entfernt eine Meldung (DELETE /attendance/notices/{noticeId}).
func (h *AttendanceHandler) DeleteNotice(w http.ResponseWriter, r *http.Request) {
	a := h.actor(w, r)
	if a == nil {
		return
	}
	n := h.noticeForActor(w, r, a)
	if n == nil {
		return
	}
	if err := h.Attendance.DeleteNotice(r.Context(), n.ID); err != nil {
		response.Error(w, http.StatusInternalServerError, "delete failed")
		return
	}
	h.audit(r, audit.ActionDelete, audit.EntityChildNotice, strconv.Itoa(n.ID), noticeSummary(n))
	w.WriteHeader(http.StatusNoContent)
}

type evacuationChild struct {
	ID        int    `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type evacuationGroup struct {
	ID       int               `json:"id"`
	Name     string            `json:"name"`
	Children []evacuationChild `json:"children"`
}

type evacuationStaff struct {
	ID          int    `json:"id"`
	DisplayName string `json:"display_name"`
	GroupName   string `json:"group_name"`
}

// Evacuation liefert alle jetzt anwesenden Kinder (gekommen, nicht gegangen) und alle eingestempelten
// Mitarbeitenden ohne Stempelzeiten (GET /attendance/evacuation?group_id=).
func (h *AttendanceHandler) Evacuation(w http.ResponseWriter, r *http.Request) {
	a := h.actor(w, r)
	if a == nil {
		return
	}
	ctx := r.Context()
	onlyGroup := 0
	if v := r.URL.Query().Get("group_id"); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil || !a.hasGroup(id) {
			response.Error(w, http.StatusForbidden, "forbidden")
			return
		}
		onlyGroup = id
	}
	today := h.today()
	children, err := h.Attendance.ListChildren(ctx, true)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	att, err := h.Attendance.ListAttendance(ctx, today)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	groups := []evacuationGroup{}
	idx := map[int]int{}
	ordered := make([]model.Group, 0, len(a.groups))
	if g := a.group(a.defaultGroupID); g != nil {
		ordered = append(ordered, *g)
	}
	for _, g := range a.groups {
		if g.ID != a.defaultGroupID {
			ordered = append(ordered, g)
		}
	}
	for _, g := range ordered {
		if onlyGroup != 0 && g.ID != onlyGroup {
			continue
		}
		idx[g.ID] = len(groups)
		groups = append(groups, evacuationGroup{ID: g.ID, Name: g.Name, Children: []evacuationChild{}})
	}
	for _, c := range children {
		gi, ok := idx[c.GroupID]
		rec, has := att[c.ID]
		if !ok || !has || rec.ArrivedAt == nil || rec.LeftAt != nil {
			continue
		}
		groups[gi].Children = append(groups[gi].Children, evacuationChild{ID: c.ID, FirstName: c.FirstName, LastName: c.LastName})
	}
	ids, err := h.Attendance.ListPresentStaff(ctx, today)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	allGroups, err := h.Groups.List(ctx)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	groupName := map[int]string{}
	for _, g := range allGroups {
		groupName[g.ID] = g.Name
	}
	staff := []evacuationStaff{}
	for _, id := range ids {
		u, err := h.Users.GetByID(ctx, id)
		if err != nil {
			continue
		}
		s := evacuationStaff{ID: u.ID, DisplayName: u.DisplayName}
		if u.GroupID != nil {
			s.GroupName = groupName[*u.GroupID]
		}
		staff = append(staff, s)
	}
	sort.Slice(staff, func(i, j int) bool {
		return strings.ToLower(staff[i].DisplayName) < strings.ToLower(staff[j].DisplayName)
	})
	response.JSON(w, http.StatusOK, map[string]any{
		"date": today, "time": h.now().Format("15:04"), "groups": groups, "staff": staff,
	})
}

// --- Verwaltung (nur Leitung) ---

// ListChildren liefert alle Kinder inkl. abgemeldeter (GET /children).
func (h *AttendanceHandler) ListChildren(w http.ResponseWriter, r *http.Request) {
	list, err := h.Attendance.ListChildren(r.Context(), true)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, list)
}

type childBody struct {
	GroupID   *int    `json:"group_id"`
	FirstName *string `json:"first_name"`
	LastName  *string `json:"last_name"`
	Active    *bool   `json:"active"`
	// BirthMonth: YYYY-MM; "" entfernt den Wert.
	BirthMonth *string `json:"birth_month"`
}

func (h *AttendanceHandler) applyChild(w http.ResponseWriter, r *http.Request, c *model.Child) bool {
	var body childBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return false
	}
	if body.GroupID != nil {
		c.GroupID = *body.GroupID
	}
	if body.FirstName != nil {
		c.FirstName = *body.FirstName
	}
	if body.LastName != nil {
		c.LastName = *body.LastName
	}
	if body.Active != nil {
		c.Active = *body.Active
	}
	if body.BirthMonth != nil {
		if v := strings.TrimSpace(*body.BirthMonth); v == "" {
			c.BirthMonth = nil
		} else {
			c.BirthMonth = &v
		}
	}
	if err := c.Validate(); err != nil {
		msg := "Bitte Vorname und Gruppe angeben."
		if strings.Contains(err.Error(), "too long") {
			msg = "Der Name ist zu lang."
		} else if strings.Contains(err.Error(), "birth_month") {
			msg = "Ungültiger Geburtsmonat."
		}
		response.Error(w, http.StatusBadRequest, msg)
		return false
	}
	if g, err := h.Groups.GetByID(r.Context(), c.GroupID); err != nil || g == nil {
		response.Error(w, http.StatusBadRequest, "Gruppe nicht gefunden")
		return false
	}
	return true
}

// CreateChild legt ein Kind an (POST /children).
func (h *AttendanceHandler) CreateChild(w http.ResponseWriter, r *http.Request) {
	c := model.Child{Active: true}
	if !h.applyChild(w, r, &c) {
		return
	}
	if err := h.Attendance.CreateChild(r.Context(), &c); err != nil {
		response.Error(w, http.StatusInternalServerError, "save failed")
		return
	}
	h.audit(r, audit.ActionCreate, audit.EntityChild, strconv.Itoa(c.ID), map[string]any{"group_id": c.GroupID})
	response.JSON(w, http.StatusCreated, c)
}

// UpdateChild ändert Name, Gruppe oder Abmeldung (PATCH /children/{id}).
func (h *AttendanceHandler) UpdateChild(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, err := h.Attendance.GetChild(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusNotFound, "Kind nicht gefunden")
		return
	}
	if !h.applyChild(w, r, c) {
		return
	}
	if err := h.Attendance.UpdateChild(r.Context(), c); err != nil {
		response.Error(w, http.StatusInternalServerError, "save failed")
		return
	}
	h.audit(r, audit.ActionUpdate, audit.EntityChild, strconv.Itoa(c.ID), map[string]any{"group_id": c.GroupID, "active": c.Active})
	response.JSON(w, http.StatusOK, c)
}

// DeleteChild löscht ein Kind mit allen Einträgen (DELETE /children/{id}).
func (h *AttendanceHandler) DeleteChild(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.Attendance.DeleteChild(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "Kind nicht gefunden")
			return
		}
		response.Error(w, http.StatusInternalServerError, "delete failed")
		return
	}
	h.audit(r, audit.ActionDelete, audit.EntityChild, strconv.Itoa(id), map[string]any{})
	w.WriteHeader(http.StatusNoContent)
}

// ListGroupAccounts liefert die Gruppenaccounts (GET /group-accounts).
func (h *AttendanceHandler) ListGroupAccounts(w http.ResponseWriter, r *http.Request) {
	list, err := h.Attendance.ListGroupAccounts(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, list)
}

// usernameTaken prüft Benutzer und andere Gruppenaccounts (ohne Groß-/Kleinschreibung).
func (h *AttendanceHandler) usernameTaken(r *http.Request, name string, exceptAccount int) (bool, error) {
	users, err := h.Users.FindByUsernameFold(r.Context(), name)
	if err != nil {
		return false, err
	}
	if len(users) > 0 {
		return true, nil
	}
	ga, err := h.Attendance.GetGroupAccountByUsername(r.Context(), name)
	if err == nil && ga.ID != exceptAccount {
		return true, nil
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return false, err
	}
	return false, nil
}

func validGroupUsername(name string) bool {
	if len(name) < 3 || len(name) > 40 {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

const groupAccountPasswordLength = 10

// CreateGroupAccount legt das Konto einer Gruppe an und liefert das Passwort einmalig (POST /group-accounts).
func (h *AttendanceHandler) CreateGroupAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GroupID  int    `json:"group_id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	if !validGroupUsername(body.Username) {
		response.Error(w, http.StatusBadRequest, "Benutzername: 3–40 Zeichen, nur Buchstaben, Ziffern, Punkt, Binde- und Unterstrich.")
		return
	}
	if g, err := h.Groups.GetByID(r.Context(), body.GroupID); err != nil || g == nil {
		response.Error(w, http.StatusBadRequest, "Gruppe nicht gefunden")
		return
	}
	if _, err := h.Attendance.GetGroupAccountByGroup(r.Context(), body.GroupID); err == nil {
		response.Error(w, http.StatusConflict, "Diese Gruppe hat schon einen Gruppenaccount.")
		return
	}
	taken, err := h.usernameTaken(r, body.Username, 0)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	if taken {
		response.Error(w, http.StatusConflict, "Der Benutzername ist schon vergeben.")
		return
	}
	pw := authsvc.GenerateRandomPassword(groupAccountPasswordLength)
	hash, err := h.Auth.HashPassword(pw)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "hash error")
		return
	}
	ga := model.GroupAccount{GroupID: body.GroupID, Username: body.Username, PasswordHash: hash, Active: true}
	if err := h.Attendance.CreateGroupAccount(r.Context(), &ga); err != nil {
		response.Error(w, http.StatusInternalServerError, "save failed")
		return
	}
	h.audit(r, audit.ActionCreate, audit.EntityGroupAccount, strconv.Itoa(ga.ID), map[string]any{"group_id": ga.GroupID, "username": ga.Username})
	response.JSON(w, http.StatusCreated, map[string]any{"account": ga, "password": pw})
}

func (h *AttendanceHandler) groupAccountParam(w http.ResponseWriter, r *http.Request) *model.GroupAccount {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return nil
	}
	ga, err := h.Attendance.GetGroupAccount(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusNotFound, "Gruppenaccount nicht gefunden")
		return nil
	}
	return ga
}

// PatchGroupAccount ändert Benutzername oder sperrt das Konto (PATCH /group-accounts/{id}).
func (h *AttendanceHandler) PatchGroupAccount(w http.ResponseWriter, r *http.Request) {
	ga := h.groupAccountParam(w, r)
	if ga == nil {
		return
	}
	var body struct {
		Username *string `json:"username"`
		Active   *bool   `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Username != nil {
		name := strings.TrimSpace(*body.Username)
		if !validGroupUsername(name) {
			response.Error(w, http.StatusBadRequest, "Benutzername: 3–40 Zeichen, nur Buchstaben, Ziffern, Punkt, Binde- und Unterstrich.")
			return
		}
		taken, err := h.usernameTaken(r, name, ga.ID)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "query failed")
			return
		}
		if taken {
			response.Error(w, http.StatusConflict, "Der Benutzername ist schon vergeben.")
			return
		}
		ga.Username = name
	}
	if body.Active != nil {
		ga.Active = *body.Active
	}
	if err := h.Attendance.UpdateGroupAccount(r.Context(), ga); err != nil {
		response.Error(w, http.StatusInternalServerError, "save failed")
		return
	}
	h.audit(r, audit.ActionUpdate, audit.EntityGroupAccount, strconv.Itoa(ga.ID), map[string]any{"username": ga.Username, "active": ga.Active})
	response.JSON(w, http.StatusOK, ga)
}

// ResetGroupAccountPassword setzt ein neues Passwort und meldet alle Geräte des Kontos ab
// (POST /group-accounts/{id}/reset-password).
func (h *AttendanceHandler) ResetGroupAccountPassword(w http.ResponseWriter, r *http.Request) {
	ga := h.groupAccountParam(w, r)
	if ga == nil {
		return
	}
	pw := authsvc.GenerateRandomPassword(groupAccountPasswordLength)
	hash, err := h.Auth.HashPassword(pw)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "hash error")
		return
	}
	if err := h.Attendance.SetGroupAccountPassword(r.Context(), ga.ID, hash); err != nil {
		response.Error(w, http.StatusInternalServerError, "save failed")
		return
	}
	h.audit(r, audit.ActionUpdate, audit.EntityGroupAccount, strconv.Itoa(ga.ID), map[string]any{"password_reset": true})
	response.JSON(w, http.StatusOK, map[string]any{"password": pw})
}

// DeleteGroupAccount löscht das Konto (DELETE /group-accounts/{id}).
func (h *AttendanceHandler) DeleteGroupAccount(w http.ResponseWriter, r *http.Request) {
	ga := h.groupAccountParam(w, r)
	if ga == nil {
		return
	}
	if err := h.Attendance.DeleteGroupAccount(r.Context(), ga.ID); err != nil {
		response.Error(w, http.StatusInternalServerError, "delete failed")
		return
	}
	h.audit(r, audit.ActionDelete, audit.EntityGroupAccount, strconv.Itoa(ga.ID), map[string]any{"group_id": ga.GroupID})
	w.WriteHeader(http.StatusNoContent)
}

// groupAccountNameTaken: Benutzer dürfen keinen Namen eines Gruppenaccounts bekommen, sonst wäre
// der Gruppenaccount bei der Anmeldung verdeckt.
func groupAccountNameTaken(r *http.Request, accounts store.AttendanceStore, name string) bool {
	if accounts == nil {
		return false
	}
	_, err := accounts.GetGroupAccountByUsername(r.Context(), name)
	return err == nil
}

// GetRetention liefert die Löschfristen (GET /children/retention).
func (h *AttendanceHandler) GetRetention(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, h.retention(r))
}

// PutRetention speichert die Löschfristen (PUT /children/retention).
func (h *AttendanceHandler) PutRetention(w http.ResponseWriter, r *http.Request) {
	if h.Settings == nil {
		response.Error(w, http.StatusServiceUnavailable, "settings unavailable")
		return
	}
	var c childretention.Config
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := childretention.SaveConfig(r.Context(), h.Settings, c); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(r, audit.ActionUpdate, audit.EntityChildRetention, "config", map[string]any{
		"times_months": c.TimesMonths, "notice_weeks": c.NoticeWeeks, "inactive_months": c.InactiveMonths,
	})
	response.JSON(w, http.StatusOK, c)
}
