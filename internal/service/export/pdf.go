package export

import (
	"bytes"
	"fmt"

	"github.com/jung-kurt/gofpdf"
)

// MonthlyPDF builds a simple A4 portrait report.
func MonthlyPDF(employeeName string, year, month int, rows []DayRow) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetTitle("Monatsbericht", false)
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(0, 10, fmt.Sprintf("Zeiterfassung %s %d-%02d", employeeName, year, month))
	pdf.Ln(12)

	pdf.SetFont("Arial", "", 9)
	headers := []string{"Datum", "WT", "Schicht", "Stempel", "Brutto", "Netto", "Soll", "Saldo", "Hinweis"}
	w := []float64{22, 10, 28, 38, 18, 18, 16, 18, 32}
	for i, h := range headers {
		pdf.CellFormat(w[i], 7, h, "1", 0, "L", false, 0, "")
	}
	pdf.Ln(-1)

	for _, r := range rows {
		shift := r.ShiftStart
		if r.ShiftEnd != "" {
			shift += "-" + r.ShiftEnd
		}
		line := []string{
			r.Date,
			r.Weekday,
			shift,
			trunc(r.PunchWindow, 28),
			formatFloat(r.GrossHours),
			formatFloat(r.NetHours),
			formatFloat(r.TargetHours),
			formatFloat(r.Balance),
			trunc(r.Notes, 30),
		}
		for i, cell := range line {
			pdf.CellFormat(w[i], 6, cell, "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func trunc(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-3]) + "..."
}
