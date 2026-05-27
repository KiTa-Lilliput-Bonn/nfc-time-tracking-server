package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/fixednonwork"
	"nfc-time-tracking-server/internal/store"
)

type ClosureHandler struct {
	Closures             store.ClosureDayStore
	Holidays             store.HolidayStore
	Users                store.UserStore
	FixedNonWorkWeekdays store.FixedNonWorkWeekdaysStore
	Absences             store.AbsenceStore
	Audit                *audit.Logger
}

func normalizeClosureDateISO(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return s
}

func (h *ClosureHandler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.Closures.List(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{"closure_days": list})
}

type closureBody struct {
	ClosureDate string `json:"closure_date"`
	Name        string `json:"name"`
}

func (h *ClosureHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body closureBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	dateStr := normalizeClosureDateISO(body.ClosureDate)
	if dateStr == "" {
		response.Error(w, http.StatusBadRequest, "closure_date required")
		return
	}
	if h.Holidays != nil {
		if hol, err := h.Holidays.GetForDate(r.Context(), dateStr); err == nil && hol != nil && hol.ID != 0 {
			response.Error(w, http.StatusBadRequest, "closure_date is a holiday")
			return
		}
	}
	c := &model.ClosureDay{
		ClosureDate: dateStr, Name: body.Name, CreatedBy: middleware.UserID(r),
	}
	if err := h.Closures.Create(r.Context(), c); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.Users != nil && h.Absences != nil {
		syncVacationAbsencesForClosureDay(r.Context(), h.Users, h.FixedNonWorkWeekdays, h.Absences, dateStr, middleware.UserID(r))
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionCreate, EntityType: audit.EntityClosureDay, EntityID: auditID(c.ID),
		Summary: audit.JSONSummary(map[string]any{"closure_date": c.ClosureDate, "name": c.Name}),
	})
	response.JSON(w, http.StatusCreated, c)
}

// syncVacationAbsencesForClosureDay legt für alle aktiven Nicht-Superadmin-Nutzer an Werktagen
// (ohne fix frei) einen Ganztags-Urlaub an, sofern noch keine Abwesenheit existiert.
func syncVacationAbsencesForClosureDay(ctx context.Context, users store.UserStore, fnw store.FixedNonWorkWeekdaysStore, absences store.AbsenceStore, dateStr string, createdBy int) {
	day, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return
	}
	list, err := users.List(ctx, true)
	if err != nil {
		return
	}
	for i := range list {
		u := &list[i]
		if u.Role == model.RoleSuperadmin {
			continue
		}
		fixed := fixednonwork.WeekdaysForUserDate(ctx, fnw, u.ID, dateStr)
		if !model.IsEmployeeWorkday(day, fixed) {
			continue
		}
		existing, err := absences.GetForUserDate(ctx, u.ID, dateStr)
		if err != nil || existing != nil {
			continue
		}
		a := &model.Absence{
			UserID: u.ID, AbsenceDate: dateStr, AbsenceType: model.AbsenceVacation,
			HalfDay: false, CreatedBy: createdBy,
		}
		_ = absences.Create(ctx, a)
	}
}

func (h *ClosureHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.Closures.Delete(r.Context(), id); err != nil {
		response.Error(w, http.StatusInternalServerError, "delete failed")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionDelete, EntityType: audit.EntityClosureDay, EntityID: auditID(id),
	})
	w.WriteHeader(http.StatusNoContent)
}
