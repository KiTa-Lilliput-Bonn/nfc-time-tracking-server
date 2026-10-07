package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/kibizcalc"
	"nfc-time-tracking-server/internal/store"
	sqlitesched "nfc-time-tracking-server/internal/store/sqlite"
)

const (
	settingKibizCountLeitung      = "kibiz_count_leitung"
	settingKibizCountTeamMeetings = "kibiz_count_team_meetings"
)

func loadKibizOptions(ctx context.Context, settings store.SettingsStore) (kibizcalc.Options, error) {
	var o kibizcalc.Options
	if settings == nil {
		return o, nil
	}
	v, err := settings.Get(ctx, settingKibizCountLeitung)
	if err != nil {
		return o, err
	}
	o.CountLeitung = strings.TrimSpace(v) == "true"
	v, err = settings.Get(ctx, settingKibizCountTeamMeetings)
	if err != nil {
		return o, err
	}
	o.CountTeamMeetings = strings.TrimSpace(v) == "true"
	return o, nil
}

// KibizWeek rechnet für eine ISO-Woche je Gruppe die nötigen und geplanten Fachkraft- und
// Ergänzungskraftstunden nach KiBiz aus (GET /schedules/kibiz?year=&week=).
func (h *ScheduleHandler) KibizWeek(w http.ResponseWriter, r *http.Request) {
	wk, err1 := strconv.Atoi(r.URL.Query().Get("week"))
	yr, err2 := strconv.Atoi(r.URL.Query().Get("year"))
	if err1 != nil || err2 != nil || wk < 1 || wk > 53 {
		response.Error(w, http.StatusBadRequest, "week and year required")
		return
	}
	from, to, err := sqlitesched.ISOWeekMondayFriday(yr, wk)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	fail := func(err error) { response.Error(w, http.StatusInternalServerError, err.Error()) }

	mon, err := time.Parse("2006-01-02", from)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	in := kibizcalc.Input{Closed: map[string]bool{}, FixedNonWork: map[int][]int{}}
	for i := 0; i < 5; i++ {
		in.Dates = append(in.Dates, mon.AddDate(0, 0, i).Format("2006-01-02"))
	}
	if in.Groups, err = h.Groups.List(ctx); err != nil {
		fail(err)
		return
	}
	if in.Users, err = h.Users.List(ctx, true); err != nil {
		fail(err)
		return
	}
	if in.Qualifications, err = h.Kibiz.ListQualifications(ctx); err != nil {
		fail(err)
		return
	}
	if in.Rates, err = h.Kibiz.ListRates(ctx); err != nil {
		fail(err)
		return
	}
	if in.Patterns, err = h.Kibiz.ListChildPatterns(ctx); err != nil {
		fail(err)
		return
	}
	if in.ChildDays, err = h.Kibiz.ListChildDays(ctx, from, to); err != nil {
		fail(err)
		return
	}
	if in.Schedules, err = h.Schedules.ListByWeek(ctx, yr, wk); err != nil {
		fail(err)
		return
	}
	if in.Absences, err = h.Absences.ListByDateRangeTypes(ctx, from, to, []model.AbsenceType{
		model.AbsenceVacation, model.AbsenceCompensationDay, model.AbsenceSick, model.AbsenceOther,
	}); err != nil {
		fail(err)
		return
	}
	for _, d := range in.Dates {
		hol, err := h.Holidays.GetForDate(ctx, d)
		if err != nil {
			fail(err)
			return
		}
		if hol != nil {
			in.Closed[d] = true
		}
	}
	if h.Closures != nil {
		all, err := h.Closures.List(ctx)
		if err != nil {
			fail(err)
			return
		}
		for _, c := range all {
			if d := model.NormCalendarDate(c.ClosureDate); d >= from && d <= to {
				in.Closed[d] = true
			}
		}
	}
	if h.FixedNonWorkWeekdays != nil {
		for _, u := range in.Users {
			rows, err := h.FixedNonWorkWeekdays.ListByUser(ctx, u.ID)
			if err != nil {
				fail(err)
				return
			}
			in.FixedNonWork[u.ID] = model.FixedNonWorkWeekdaysForDate(rows, from)
		}
	}
	if h.TeamMeetings != nil {
		if in.TeamMeetings, err = h.TeamMeetings.ListByWeek(ctx, yr, wk); err != nil {
			fail(err)
			return
		}
	}
	if h.Settings != nil {
		if v, err := h.Settings.Get(ctx, "break_rules"); err == nil && v != "" {
			_ = json.Unmarshal([]byte(v), &in.BreakRules)
		}
	}
	if in.Options, err = loadKibizOptions(ctx, h.Settings); err != nil {
		fail(err)
		return
	}

	res := kibizcalc.Compute(in)
	response.JSON(w, http.StatusOK, map[string]any{
		"from": from, "to": to,
		"groups":               res.Groups,
		"unqualified_user_ids": res.Unqualified,
		"options":              in.Options,
		"rates_configured":     len(in.Rates) > 0,
	})
}

// KibizHandler verwaltet die Planungsgrundlagen der Leitung: Qualifikationen, KiBiz-Tabelle,
// Kinder-Wochenmuster, Abweichungen pro Tag und die Schalter.
type KibizHandler struct {
	Kibiz    store.KibizStore
	Users    store.UserStore
	Groups   store.GroupStore
	Settings store.SettingsStore
	Audit    *audit.Logger
}

func (h *KibizHandler) audit(r *http.Request, action, id string, summary map[string]any) {
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: action, EntityType: audit.EntityKibizPlanning, EntityID: id,
		Summary: audit.JSONSummary(summary),
	})
}

// Get liefert alles für die Seite „Planungsgrundlagen“ (GET /planning/kibiz).
func (h *KibizHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	quals, err := h.Kibiz.ListQualifications(ctx)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	rates, err := h.Kibiz.ListRates(ctx)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	patterns, err := h.Kibiz.ListChildPatterns(ctx)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	opts, err := loadKibizOptions(ctx, h.Settings)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	q := make([]map[string]any, 0, len(quals))
	for uid, v := range quals {
		q = append(q, map[string]any{"user_id": uid, "qualification": v})
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"qualifications": q, "rates": rates, "child_patterns": patterns, "options": opts,
	})
}

// PutQualification setzt die Qualifikation einer Person (PUT /planning/qualifications/{userId}).
func (h *KibizHandler) PutQualification(w http.ResponseWriter, r *http.Request) {
	uid, err := strconv.Atoi(chi.URLParam(r, "userId"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid user id")
		return
	}
	var body struct {
		Qualification model.Qualification `json:"qualification"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Qualification != "" && !body.Qualification.Valid() {
		response.Error(w, http.StatusBadRequest, "invalid qualification")
		return
	}
	if _, err := h.Users.GetByID(r.Context(), uid); err != nil {
		response.Error(w, http.StatusNotFound, "user not found")
		return
	}
	if err := h.Kibiz.SetQualification(r.Context(), uid, body.Qualification); err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, audit.ActionUpdate, strconv.Itoa(uid), map[string]any{"qualification": body.Qualification})
	response.JSON(w, http.StatusOK, map[string]any{"user_id": uid, "qualification": body.Qualification})
}

// PutRates schreibt die KiBiz-Tabelle (PUT /planning/kibiz-rates).
func (h *KibizHandler) PutRates(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Rates []model.KibizRate `json:"rates"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	for _, rt := range body.Rates {
		if err := rt.Validate(); err != nil {
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := h.Kibiz.PutRates(r.Context(), body.Rates); err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, audit.ActionUpdate, "rates", map[string]any{"rates": body.Rates})
	rates, err := h.Kibiz.ListRates(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"rates": rates})
}

// PutOptions speichert die Schalter (PUT /planning/kibiz-options).
func (h *KibizHandler) PutOptions(w http.ResponseWriter, r *http.Request) {
	var body kibizcalc.Options
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	for k, v := range map[string]bool{
		settingKibizCountLeitung:      body.CountLeitung,
		settingKibizCountTeamMeetings: body.CountTeamMeetings,
	} {
		if err := h.Settings.Set(r.Context(), k, strconv.FormatBool(v)); err != nil {
			response.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	h.audit(r, audit.ActionUpdate, "options", map[string]any{"options": body})
	response.JSON(w, http.StatusOK, body)
}

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func (h *KibizHandler) checkGroup(ctx context.Context, id int) error {
	if id <= 0 {
		return fmt.Errorf("group_id required")
	}
	if _, err := h.Groups.GetByID(ctx, id); err != nil {
		return fmt.Errorf("group not found")
	}
	return nil
}

// PutChildPattern legt eine Version des Kinder-Wochenmusters an oder ersetzt sie (PUT /planning/child-patterns).
func (h *KibizHandler) PutChildPattern(w http.ResponseWriter, r *http.Request) {
	var body model.ChildPattern
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.checkGroup(r.Context(), body.GroupID); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if !validDate(body.ValidFrom) {
		response.Error(w, http.StatusBadRequest, "valid_from must be YYYY-MM-DD")
		return
	}
	if body.Counts == nil {
		body.Counts = []model.ChildCount{}
	}
	if err := model.ValidateChildCounts(body.Counts); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Kibiz.PutChildPattern(r.Context(), body); err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, audit.ActionUpdate, fmt.Sprintf("pattern:%d:%s", body.GroupID, body.ValidFrom), map[string]any{"counts": body.Counts})
	response.JSON(w, http.StatusOK, body)
}

// DeleteChildPattern entfernt eine Version (DELETE /planning/child-patterns?group_id=&valid_from=).
func (h *KibizHandler) DeleteChildPattern(w http.ResponseWriter, r *http.Request) {
	gid, err := strconv.Atoi(r.URL.Query().Get("group_id"))
	vf := r.URL.Query().Get("valid_from")
	if err != nil || !validDate(vf) {
		response.Error(w, http.StatusBadRequest, "group_id and valid_from required")
		return
	}
	if err := h.Kibiz.DeleteChildPattern(r.Context(), gid, vf); err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, audit.ActionDelete, fmt.Sprintf("pattern:%d:%s", gid, vf), map[string]any{})
	w.WriteHeader(http.StatusNoContent)
}

type childDaysBody struct {
	GroupID int                `json:"group_id"`
	Dates   []string           `json:"dates"`
	Counts  []model.ChildCount `json:"counts"`
	// Reset: Tage auf das Muster zurücksetzen (Counts wird ignoriert).
	Reset bool `json:"reset"`
}

// PutChildDays setzt die Kinderzahl einer Gruppe für einzelne Tage oder setzt sie zurück (PUT /planning/child-days).
func (h *KibizHandler) PutChildDays(w http.ResponseWriter, r *http.Request) {
	var body childDaysBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.checkGroup(r.Context(), body.GroupID); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.Dates) == 0 || len(body.Dates) > 7 {
		response.Error(w, http.StatusBadRequest, "1 to 7 dates required")
		return
	}
	for _, d := range body.Dates {
		if !validDate(d) {
			response.Error(w, http.StatusBadRequest, "dates must be YYYY-MM-DD")
			return
		}
	}
	id := fmt.Sprintf("days:%d:%s", body.GroupID, strings.Join(body.Dates, ","))
	if body.Reset {
		if err := h.Kibiz.DeleteChildDays(r.Context(), body.GroupID, body.Dates); err != nil {
			response.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		h.audit(r, audit.ActionDelete, id, map[string]any{})
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if body.Counts == nil {
		body.Counts = []model.ChildCount{}
	}
	if err := model.ValidateChildCounts(body.Counts); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Kibiz.PutChildDays(r.Context(), body.GroupID, body.Dates, body.Counts); err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, audit.ActionUpdate, id, map[string]any{"counts": body.Counts})
	w.WriteHeader(http.StatusNoContent)
}
