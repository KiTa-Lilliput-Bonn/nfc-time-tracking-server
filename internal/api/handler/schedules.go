package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	apimw "nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/scheduleexport"
	"nfc-time-tracking-server/internal/service/scheduleimport"
	"nfc-time-tracking-server/internal/store"
	sqlitesched "nfc-time-tracking-server/internal/store/sqlite"
)

type ScheduleHandler struct {
	Schedules             store.ScheduleStore
	TeamMeetings          store.TeamMeetingStore
	Users                 store.UserStore
	Groups                store.GroupStore
	Absences              store.AbsenceStore
	Holidays              store.HolidayStore
	Closures              store.ClosureDayStore
	CompensationDayClaims store.CompensationDayClaimStore
	FixedNonWorkWeekdays  store.FixedNonWorkWeekdaysStore
	Audit                 *audit.Logger
}

func (h *ScheduleHandler) ListWeek(w http.ResponseWriter, r *http.Request) {
	wk, err1 := strconv.Atoi(r.URL.Query().Get("week"))
	yr, err2 := strconv.Atoi(r.URL.Query().Get("year"))
	if err1 != nil || err2 != nil || wk < 1 || wk > 53 {
		response.Error(w, http.StatusBadRequest, "week and year required")
		return
	}
	list, err := h.Schedules.ListByWeek(r.Context(), yr, wk)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	notes, err := h.Schedules.GetWeekNotes(r.Context(), yr, wk)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	from, to, err := sqlitesched.ISOWeekMondayFriday(yr, wk)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	absences, err := h.Absences.ListByDateRangeTypes(r.Context(), from, to, []model.AbsenceType{
		model.AbsenceVacation,
		model.AbsenceCompensationDay,
	})
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if absences == nil {
		absences = []model.Absence{}
	}
	mon, err := time.ParseInLocation("2006-01-02", from, time.Local)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	weekHolidays := []model.Holiday{}
	for i := 0; i < 5; i++ {
		d := mon.AddDate(0, 0, i).Format("2006-01-02")
		hol, err := h.Holidays.GetForDate(r.Context(), d)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if hol != nil {
			weekHolidays = append(weekHolidays, *hol)
		}
	}
	var teamMeetings []model.TeamMeeting
	if h.TeamMeetings != nil {
		tm, err := h.TeamMeetings.ListByWeek(r.Context(), yr, wk)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		teamMeetings = tm
	}
	if teamMeetings == nil {
		teamMeetings = []model.TeamMeeting{}
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{
		"schedules": list, "week_notes": notes, "absences": absences,
		"week_holidays": weekHolidays, "team_meetings": teamMeetings,
	})
}

type weekNotesPutBody struct {
	Year  int    `json:"year"`
	Week  int    `json:"week"`
	Notes string `json:"notes"`
}

func (h *ScheduleHandler) PutWeekNotes(w http.ResponseWriter, r *http.Request) {
	var body weekNotesPutBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Year < 2000 || body.Year > 2100 || body.Week < 1 || body.Week > 53 {
		response.Error(w, http.StatusBadRequest, "year and week required")
		return
	}
	if err := h.Schedules.PutWeekNotes(r.Context(), body.Year, body.Week, body.Notes); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityScheduleWeekNotes,
		EntityID: strconv.Itoa(body.Year) + "-w" + strconv.Itoa(body.Week),
		Summary:  audit.JSONSummary(map[string]any{"year": body.Year, "week": body.Week}),
	})
	response.JSON(w, http.StatusOK, map[string]string{"notes": body.Notes})
}

type scheduleBody struct {
	UserID       int    `json:"user_id"`
	ScheduleDate string `json:"schedule_date"`
	ShiftStart   string `json:"shift_start"`
	ShiftEnd     string `json:"shift_end"`
}

func (h *ScheduleHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body scheduleBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.UserID == 0 || body.ScheduleDate == "" {
		response.Error(w, http.StatusBadRequest, "user_id and schedule_date required")
		return
	}
	target, err := h.Users.GetByID(r.Context(), body.UserID)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "user not found")
		return
	}
	if !model.ActorMayManageUser(apimw.Role(r), target) {
		response.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	sch := &model.Schedule{
		UserID: body.UserID, ScheduleDate: body.ScheduleDate,
		ShiftStart: body.ShiftStart, ShiftEnd: body.ShiftEnd,
	}
	if err := h.Schedules.Set(r.Context(), sch); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionCreate, EntityType: audit.EntitySchedule, EntityID: auditID(sch.ID),
		TargetUserID: auditTarget(body.UserID),
		Summary:      audit.JSONSummary(map[string]any{"schedule_date": body.ScheduleDate}),
	})
	response.JSON(w, http.StatusCreated, sch)
}

type schedulePatchBody struct {
	ShiftStart string `json:"shift_start"`
	ShiftEnd   string `json:"shift_end"`
}

func (h *ScheduleHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	sch, err := h.Schedules.GetByID(r.Context(), id)
	if err != nil || sch == nil {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	target, err := h.Users.GetByID(r.Context(), sch.UserID)
	if err != nil {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	if !model.ActorMayManageUser(apimw.Role(r), target) {
		response.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	var body schedulePatchBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	sch.ShiftStart = body.ShiftStart
	sch.ShiftEnd = body.ShiftEnd
	if err := h.Schedules.Set(r.Context(), sch); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntitySchedule, EntityID: auditID(sch.ID),
		TargetUserID: auditTarget(sch.UserID),
		Summary:      audit.JSONSummary(map[string]any{"schedule_date": sch.ScheduleDate}),
	})
	response.JSON(w, http.StatusOK, sch)
}

func (h *ScheduleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	sch, err := h.Schedules.GetByID(r.Context(), id)
	if err != nil || sch == nil {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	target, err := h.Users.GetByID(r.Context(), sch.UserID)
	if err != nil {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	if !model.ActorMayManageUser(apimw.Role(r), target) {
		response.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := h.Schedules.Delete(r.Context(), id); err != nil {
		response.Error(w, http.StatusInternalServerError, "delete failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionDelete, EntityType: audit.EntitySchedule, EntityID: auditID(id),
		TargetUserID: auditTarget(sch.UserID),
		Summary:      audit.JSONSummary(map[string]any{"schedule_date": sch.ScheduleDate}),
	})
	w.WriteHeader(http.StatusNoContent)
}

const maxTeamMeetingLabelRunes = 80

type putTeamMeetingBody struct {
	TimeStart   string `json:"time_start"`
	TimeEnd     string `json:"time_end"`
	UserIDs     []int  `json:"user_ids"`
	MeetingDate string `json:"meeting_date,omitempty"`
	Label       string `json:"label,omitempty"`
}

type postTeamMeetingBody struct {
	Year        int    `json:"year"`
	Week        int    `json:"week"`
	Kind        string `json:"kind"`
	MeetingDate string `json:"meeting_date,omitempty"`
	Label       string `json:"label,omitempty"`
	TimeStart   string `json:"time_start"`
	TimeEnd     string `json:"time_end"`
	UserIDs     []int  `json:"user_ids"`
}

func parseTeamMeetingKind(s string) (model.TeamMeetingKind, bool) {
	k := model.TeamMeetingKind(strings.ToLower(strings.TrimSpace(s)))
	switch k {
	case model.TeamMeetingKindKT, model.TeamMeetingKindGT, model.TeamMeetingKindOther:
		return k, true
	default:
		return "", false
	}
}

func normalizeTeamMeetingLabel(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("label required")
	}
	if len([]rune(s)) > maxTeamMeetingLabelRunes {
		return "", fmt.Errorf("label too long (max %d characters)", maxTeamMeetingLabelRunes)
	}
	return s, nil
}

func normalizeMeetingDateYMD(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

// PostTeamMeeting legt eine manuelle Teamsitzung für die angegebene ISO-Kalenderwoche an (Montag laut KW).
func (h *ScheduleHandler) PostTeamMeeting(w http.ResponseWriter, r *http.Request) {
	if h.TeamMeetings == nil {
		response.Error(w, http.StatusInternalServerError, "team meetings not configured")
		return
	}
	var body postTeamMeetingBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.Year < 2000 || body.Year > 2100 || body.Week < 1 || body.Week > 53 {
		response.Error(w, http.StatusBadRequest, "year and week required")
		return
	}
	if body.TimeStart == "" || body.TimeEnd == "" {
		response.Error(w, http.StatusBadRequest, "time_start and time_end required")
		return
	}
	kind, ok := parseTeamMeetingKind(body.Kind)
	if !ok {
		response.Error(w, http.StatusBadRequest, "kind must be kt, gt, or other")
		return
	}
	if len(body.UserIDs) == 0 {
		response.Error(w, http.StatusBadRequest, "user_ids required")
		return
	}
	monday, _, err := sqlitesched.ISOWeekMondayFriday(body.Year, body.Week)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	meetingDate := monday
	label := ""
	if kind == model.TeamMeetingKindOther {
		meetingDate = normalizeMeetingDateYMD(body.MeetingDate)
		if meetingDate == "" {
			response.Error(w, http.StatusBadRequest, "meeting_date required for other")
			return
		}
		if err := sqlitesched.ValidateScheduleWeekday(body.Year, body.Week, meetingDate); err != nil {
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		var labelErr error
		label, labelErr = normalizeTeamMeetingLabel(body.Label)
		if labelErr != nil {
			response.Error(w, http.StatusBadRequest, labelErr.Error())
			return
		}
	}
	secIdx, err := h.TeamMeetings.NextManualSectionIndex(r.Context(), body.Year, body.Week, kind)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	m := &model.TeamMeeting{
		ISOWeekYear:  body.Year,
		ISOWeek:      body.Week,
		MeetingDate:  meetingDate,
		Kind:         kind,
		Label:        label,
		TimeStart:    body.TimeStart,
		TimeEnd:      body.TimeEnd,
		Source:       "manual",
		SectionIndex: secIdx,
		UserIDs:      body.UserIDs,
	}
	if err := h.TeamMeetings.CreateWithUsers(r.Context(), m); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	out, err := h.TeamMeetings.GetByID(r.Context(), m.ID)
	if err != nil || out == nil {
		response.Error(w, http.StatusInternalServerError, "reload failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionCreate, EntityType: audit.EntityTeamMeeting, EntityID: auditID(out.ID),
		Summary: audit.JSONSummary(map[string]any{"year": body.Year, "week": body.Week, "kind": body.Kind}),
	})
	response.JSON(w, http.StatusCreated, out)
}

// PutTeamMeeting aktualisiert Zeiten und Teilnehmer einer Teamsitzung (Leitung).
func (h *ScheduleHandler) PutTeamMeeting(w http.ResponseWriter, r *http.Request) {
	if h.TeamMeetings == nil {
		response.Error(w, http.StatusInternalServerError, "team meetings not configured")
		return
	}
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body putTeamMeetingBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.TimeStart == "" || body.TimeEnd == "" {
		response.Error(w, http.StatusBadRequest, "time_start and time_end required")
		return
	}
	m, err := h.TeamMeetings.GetByID(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if m == nil {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	m.TimeStart = body.TimeStart
	m.TimeEnd = body.TimeEnd
	m.UserIDs = body.UserIDs
	if m.Source == "" {
		m.Source = "manual"
	}
	if m.Kind == model.TeamMeetingKindOther && m.Source != "excel" {
		labelInput := body.Label
		if strings.TrimSpace(labelInput) == "" {
			labelInput = m.Label
		}
		label, err := normalizeTeamMeetingLabel(labelInput)
		if err != nil {
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		m.Label = label
		if body.MeetingDate != "" {
			meetingDate := normalizeMeetingDateYMD(body.MeetingDate)
			if err := sqlitesched.ValidateScheduleWeekday(m.ISOWeekYear, m.ISOWeek, meetingDate); err != nil {
				response.Error(w, http.StatusBadRequest, err.Error())
				return
			}
			m.MeetingDate = meetingDate
		}
	}
	if err := h.TeamMeetings.ReplaceMeetingAndUsers(r.Context(), m); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	out, err := h.TeamMeetings.GetByID(r.Context(), id)
	if err != nil || out == nil {
		response.Error(w, http.StatusInternalServerError, "reload failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityTeamMeeting, EntityID: auditID(id),
		Summary: audit.JSONSummary(map[string]any{"user_count": len(body.UserIDs)}),
	})
	response.JSON(w, http.StatusOK, out)
}

// DeleteTeamMeeting entfernt eine Teamsitzung (Leitung).
func (h *ScheduleHandler) DeleteTeamMeeting(w http.ResponseWriter, r *http.Request) {
	if h.TeamMeetings == nil {
		response.Error(w, http.StatusInternalServerError, "team meetings not configured")
		return
	}
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	m, err := h.TeamMeetings.GetByID(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if m == nil {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	if err := h.TeamMeetings.Delete(r.Context(), id); err != nil {
		response.Error(w, http.StatusInternalServerError, "delete failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionDelete, EntityType: audit.EntityTeamMeeting, EntityID: auditID(id),
		Summary: audit.JSONSummary(map[string]any{
			"year": m.ISOWeekYear, "week": m.ISOWeek, "kind": m.Kind, "meeting_date": m.MeetingDate,
		}),
	})
	w.WriteHeader(http.StatusNoContent)
}

// ExportDefaults liefert Standardwerte für den Excel-Export-Dialog.
func (h *ScheduleHandler) ExportDefaults(w http.ResponseWriter, r *http.Request) {
	startY, startW, endY, endW, err := scheduleexport.ExportDefaults(r.Context(), h.Schedules)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	response.JSON(w, http.StatusOK, map[string]int{
		"start_year": startY, "start_week": startW,
		"end_year": endY, "end_week": endW,
	})
}

// ExportExcel erzeugt eine .xlsx-Datei für den angegebenen KW-Zeitraum.
func (h *ScheduleHandler) ExportExcel(w http.ResponseWriter, r *http.Request) {
	fromYear, err1 := strconv.Atoi(r.URL.Query().Get("from_year"))
	fromWeek, err2 := strconv.Atoi(r.URL.Query().Get("from_week"))
	toYear, err3 := strconv.Atoi(r.URL.Query().Get("to_year"))
	toWeek, err4 := strconv.Atoi(r.URL.Query().Get("to_week"))
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		response.Error(w, http.StatusBadRequest, "from_year, from_week, to_year, to_week required")
		return
	}
	if fromWeek < 1 || fromWeek > 53 || toWeek < 1 || toWeek > 53 {
		response.Error(w, http.StatusBadRequest, "invalid week")
		return
	}
	if fromYear < 2000 || fromYear > 2100 || toYear < 2000 || toYear > 2100 {
		response.Error(w, http.StatusBadRequest, "invalid year")
		return
	}

	buf, err := scheduleexport.BuildXLSX(r.Context(), scheduleexport.Deps{
		Users:                h.Users,
		Groups:               h.Groups,
		Schedules:            h.Schedules,
		TeamMeetings:         h.TeamMeetings,
		Absences:             h.Absences,
		Holidays:             h.Holidays,
		FixedNonWorkWeekdays: h.FixedNonWorkWeekdays,
	}, fromYear, fromWeek, toYear, toWeek)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	filename := fmt.Sprintf("Dienstplan-KW%d-%d-bis-KW%d-%d.xlsx", fromWeek, fromYear, toWeek, toYear)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	_, _ = w.Write(buf)
}

// ImportExcel erwartet multipart/form-data mit Feld "file" (.xlsx).
func (h *ScheduleHandler) ImportExcel(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		response.Error(w, http.StatusBadRequest, "multipart form erwartet")
		return
	}
	fh, _, err := r.FormFile("file")
	if err != nil {
		response.Error(w, http.StatusBadRequest, "Dateifeld 'file' erforderlich")
		return
	}
	defer func() { _ = fh.Close() }()

	buf, err := io.ReadAll(fh)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "Datei konnte nicht gelesen werden")
		return
	}

	scope := scheduleimport.ParseImportScope(r.FormValue("scope"))

	rep, err := scheduleimport.Import(r.Context(), scheduleimport.Deps{
		Users:                h.Users,
		FixedNonWorkWeekdays: h.FixedNonWorkWeekdays,
		Schedules:            h.Schedules,
		TeamMeetings:         h.TeamMeetings,
		Absences:             h.Absences,
		Holidays:             h.Holidays,
		Closures:             h.Closures,
		Claims:               h.CompensationDayClaims,
	}, buf, apimw.UserID(r), scope)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityScheduleImport, EntityID: "excel",
		Summary: audit.JSONSummary(rep),
	})
	response.JSON(w, http.StatusOK, rep)
}
