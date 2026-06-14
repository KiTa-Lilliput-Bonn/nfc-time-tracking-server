package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store"
)

// TestSeedHandler exposes E2E-only helpers (registered only when NFC_TEST_MODE is active).
type TestSeedHandler struct {
	WorkPeriods store.WorkPeriodStore
}

type testSeedImportedBody struct {
	EmployeeID int       `json:"employee_id"`
	WorkDate   string    `json:"work_date"`
	PunchIn    time.Time `json:"punch_in"`
	PunchOut   time.Time `json:"punch_out"`
}

func (h *TestSeedHandler) SeedImportedWorkPeriod(w http.ResponseWriter, r *http.Request) {
	var body testSeedImportedBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.EmployeeID <= 0 || body.WorkDate == "" {
		response.Error(w, http.StatusBadRequest, "employee_id and work_date required")
		return
	}
	if !body.PunchOut.After(body.PunchIn) {
		response.Error(w, http.StatusBadRequest, "punch_out must be after punch_in")
		return
	}
	wp := model.WorkPeriod{
		PunchIn:  body.PunchIn,
		PunchOut: &body.PunchOut,
		IsBreak:  false,
	}
	if err := h.WorkPeriods.ReplaceForUserDate(r.Context(), body.EmployeeID, body.WorkDate, []model.WorkPeriod{wp}); err != nil {
		response.Error(w, http.StatusInternalServerError, "seed failed")
		return
	}
	list, err := h.WorkPeriods.ListByUserDateRange(r.Context(), body.EmployeeID, body.WorkDate, body.WorkDate)
	if err != nil || len(list) == 0 {
		response.Error(w, http.StatusInternalServerError, "seed lookup failed")
		return
	}
	var imported *model.WorkPeriod
	for i := range list {
		if list[i].Source != "manual" {
			imported = &list[i]
			break
		}
	}
	if imported == nil {
		response.Error(w, http.StatusInternalServerError, "no imported period")
		return
	}
	response.JSON(w, http.StatusCreated, imported)
}
