package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/service/export"
	"nfc-time-tracking-server/internal/store"
)

// ExportHandler serves CSV/PDF exports for employees.
type ExportHandler struct {
	Users       store.UserStore
	ExportData  export.Data
}

func (h *ExportHandler) CSV(w http.ResponseWriter, r *http.Request) {
	uid, err := strconv.Atoi(r.URL.Query().Get("employee"))
	if err != nil || uid <= 0 {
		response.Error(w, http.StatusBadRequest, "employee id required")
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" {
		response.Error(w, http.StatusBadRequest, "from and to required (YYYY-MM-DD)")
		return
	}
	if _, e := time.Parse("2006-01-02", from); e != nil {
		response.Error(w, http.StatusBadRequest, "invalid from")
		return
	}
	if _, e := time.Parse("2006-01-02", to); e != nil {
		response.Error(w, http.StatusBadRequest, "invalid to")
		return
	}
	u, err := h.Users.GetByID(r.Context(), uid)
	if err != nil {
		response.Error(w, http.StatusNotFound, "employee not found")
		return
	}
	rows, err := export.BuildDayRows(r.Context(), h.ExportData, uid, from, to)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="export-%s-%s-%s.csv"`, u.Username, from, to))
	_ = export.WriteCSV(w, rows)
}

func (h *ExportHandler) PDF(w http.ResponseWriter, r *http.Request) {
	uid, err := strconv.Atoi(r.URL.Query().Get("employee"))
	if err != nil || uid <= 0 {
		response.Error(w, http.StatusBadRequest, "employee id required")
		return
	}
	month, e1 := strconv.Atoi(r.URL.Query().Get("month"))
	year, e2 := strconv.Atoi(r.URL.Query().Get("year"))
	if e1 != nil || e2 != nil || month < 1 || month > 12 {
		response.Error(w, http.StatusBadRequest, "month and year required")
		return
	}
	u, err := h.Users.GetByID(r.Context(), uid)
	if err != nil {
		response.Error(w, http.StatusNotFound, "employee not found")
		return
	}
	from, to, err := export.MonthDateRange(year, month)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	rows, err := export.BuildDayRows(r.Context(), h.ExportData, uid, from, to)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	pdfBytes, err := export.MonthlyPDF(u.DisplayName, year, month, rows)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "pdf failed")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="report-%s-%d-%02d.pdf"`, u.Username, year, month))
	_, _ = w.Write(pdfBytes)
}
