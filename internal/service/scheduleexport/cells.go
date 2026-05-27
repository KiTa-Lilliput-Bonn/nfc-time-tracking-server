package scheduleexport

import (
	"strconv"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
)

// formatExcelClock wandelt HH:MM in Excel-Anzeige (z. B. 8.30) um.
func formatExcelClock(hhmm string) string {
	hhmm = strings.TrimSpace(hhmm)
	parts := strings.Split(hhmm, ":")
	if len(parts) != 2 {
		return hhmm
	}
	h, err1 := strconv.Atoi(parts[0])
	m := parts[1]
	if err1 != nil {
		return hhmm
	}
	if len(m) == 1 {
		m = "0" + m
	}
	return strconv.Itoa(h) + "." + m
}

func formatExcelShiftRange(start, end string) string {
	return formatExcelClock(start) + "-" + formatExcelClock(end)
}

func formatTeamMeetingClock(hhmm string) string {
	hhmm = strings.TrimSpace(hhmm)
	parts := strings.Split(hhmm, ":")
	if len(parts) != 2 {
		return hhmm
	}
	h, _ := strconv.Atoi(parts[0])
	return fmtInt2(h) + ":" + parts[1]
}

func fmtInt2(h int) string {
	if h < 10 {
		return "0" + strconv.Itoa(h)
	}
	return strconv.Itoa(h)
}

func isFixedNonWorkDay(fixed []int, date time.Time) bool {
	if len(fixed) == 0 {
		return false
	}
	return model.IsFixedNonWorkWeekday(date.Weekday(), fixed)
}

type weekCellData struct {
	schedules map[int]map[string]*model.Schedule // userID -> date -> schedule
	absences  map[int]map[string]*model.Absence
	fnwByUser map[int][]model.FixedNonWorkWeekdays
	skipDay   [5]bool
	holiday   [5]string
}

func (d *weekCellData) cellValue(u *model.User, dayIdx int, date time.Time) string {
	if d.skipDay[dayIdx] {
		return ""
	}
	dateISO := date.Format("2006-01-02")
	if sch := d.schedules[u.ID][dateISO]; sch != nil {
		return formatExcelShiftRange(sch.ShiftStart, sch.ShiftEnd)
	}
	if abs := d.absences[u.ID][dateISO]; abs != nil {
		switch abs.AbsenceType {
		case model.AbsenceVacation:
			if !abs.HalfDay {
				return "U"
			}
			return ""
		case model.AbsenceCompensationDay:
			return "AT"
		case model.AbsenceSick:
			return "xxx"
		case model.AbsenceOther:
			return otherAbsenceLabel(abs)
		}
	}
	if isFixedNonWorkDay(model.FixedNonWorkWeekdaysForDate(d.fnwByUser[u.ID], dateISO), date) {
		return "xxx"
	}
	// Weder Schicht noch Abwesenheit — freier Planungstag (Import: CellFreeDay).
	return "xxx"
}

func otherAbsenceLabel(_ *model.Absence) string {
	return "Schule"
}

func datumCellText(date time.Time, _ string, _ bool) string {
	return formatShortGermanDate(date)
}
