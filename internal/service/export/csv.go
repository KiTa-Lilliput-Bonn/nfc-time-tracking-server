package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"
)

// WriteCSV writes UTF-8 CSV with header.
func WriteCSV(w io.Writer, rows []DayRow) error {
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	if err := cw.Write([]string{
		"Datum", "Wochentag", "SchichtBeginn", "SchichtEnde", "Stempel",
		"Bruttostunden", "Nettostunden", "Soll", "Saldo", "Hinweise",
	}); err != nil {
		return err
	}
	for _, r := range rows {
		line := []string{
			r.Date,
			r.Weekday,
			r.ShiftStart,
			r.ShiftEnd,
			r.PunchWindow,
			formatFloat(r.GrossHours),
			formatFloat(r.NetHours),
			formatFloat(r.TargetHours),
			formatFloat(r.Balance),
			r.Notes,
		}
		if err := cw.Write(line); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', 2, 64)
}

// MonthDateRange returns first/last day of month (YYYY-MM-DD).
func MonthDateRange(year, month int) (from, to string, err error) {
	if month < 1 || month > 12 {
		return "", "", fmt.Errorf("invalid month")
	}
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1)
	return first.Format("2006-01-02"), last.Format("2006-01-02"), nil
}
