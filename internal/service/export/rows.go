package export

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/daycalc"
	"nfc-time-tracking-server/internal/store"
)

// DayRow is one CSV/PDF line for a calendar day.
type DayRow struct {
	Date        string
	Weekday     string
	ShiftStart  string
	ShiftEnd    string
	PunchWindow string
	GrossHours  float64
	NetHours    float64
	TargetHours float64
	Balance     float64
	Notes       string
}

// Data bundles read-only stores for building export rows.
type Data struct {
	WorkPeriods store.WorkPeriodStore
	Schedules   store.ScheduleStore
	Absences    store.AbsenceStore
	Holidays    store.HolidayStore
	Closures    store.ClosureDayStore
	WeeklyHours store.WeeklyHoursStore
	Settings    store.SettingsStore
	Users       store.UserStore
}

var weekdayDE = map[time.Weekday]string{
	time.Monday:    "Mo",
	time.Tuesday:   "Di",
	time.Wednesday: "Mi",
	time.Thursday:  "Do",
	time.Friday:    "Fr",
	time.Saturday:  "Sa",
	time.Sunday:    "So",
}

// BuildDayRows builds rows from from/to (YYYY-MM-DD, inclusive).
func BuildDayRows(ctx context.Context, d Data, userID int, from, to string) ([]DayRow, error) {
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		return nil, err
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil {
		return nil, err
	}
	if end.Before(start) {
		return nil, fmt.Errorf("to before from")
	}

	var fixed []int
	if d.Users != nil {
		if usr, err := d.Users.GetByID(ctx, userID); err == nil && usr != nil {
			fixed = usr.FixedNonWorkWeekdays
		}
	}

	roundMin := 15
	if v, err := d.Settings.Get(ctx, "rounding_minutes"); err == nil {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			roundMin = n
		}
	}
	var breakRules []model.BreakRule
	if v, err := d.Settings.Get(ctx, "break_rules"); err == nil {
		_ = json.Unmarshal([]byte(v), &breakRules)
	}

	var rows []DayRow
	var running float64
	loc := time.Local
	for t := start; !t.After(end); t = t.AddDate(0, 0, 1) {
		ds := t.Format("2006-01-02")
		localD := t.In(loc)

		wps, err := d.WorkPeriods.ListByUserDateRange(ctx, userID, ds, ds)
		if err != nil {
			return nil, err
		}

		sch, _ := d.Schedules.GetForUserDate(ctx, userID, ds)
		shiftBounds := daycalc.ShiftBoundsFromSchedule(sch)
		shiftStart, shiftEnd := "", ""
		if sch != nil {
			shiftStart, shiftEnd = sch.ShiftStart, sch.ShiftEnd
		}

		grossH := daycalc.GrossWorkHours(wps, shiftBounds)
		netH := daycalc.NetHours(wps, breakRules, roundMin, shiftBounds)

		wh, _ := d.WeeklyHours.GetForDate(ctx, userID, ds)
		var daily float64
		if wh != nil {
			daily = model.DailyHours(wh.HoursPerWeek, fixed)
		}

		hol, _ := d.Holidays.GetForDate(ctx, ds)
		abs, _ := d.Absences.GetForUserDate(ctx, userID, ds)
		clo, _ := d.Closures.GetForDate(ctx, ds)

		target := daycalc.DailyTarget(localD, daily, fixed, hol, abs, clo)
		credit := daycalc.AbsenceCreditHours(localD, daily, fixed, hol, abs, clo)
		bal := (netH + credit) - target
		running += bal

		notes := ""
		switch {
		case hol != nil:
			notes = "Feiertag: " + hol.Name
		case clo != nil:
			notes = "Schließtag: " + clo.Name
		case abs != nil:
			switch abs.AbsenceType {
			case model.AbsenceSick:
				notes = "Krank"
			case model.AbsenceVacation:
				notes = "Urlaub"
			case model.AbsenceCompensationDay:
				notes = "Ausgleichstag"
			default:
				notes = "Abwesend"
			}
			if abs.HalfDay {
				notes += " (halber Tag)"
			}
		}

		rows = append(rows, DayRow{
			Date:        ds,
			Weekday:     weekdayDE[localD.Weekday()],
			ShiftStart:  shiftStart,
			ShiftEnd:    shiftEnd,
			PunchWindow: formatPunchWindows(wps),
			GrossHours:  round2(grossH),
			NetHours:    round2(netH + credit),
			TargetHours: round2(target),
			Balance:     round2(running),
			Notes:       notes,
		})
	}
	return rows, nil
}

func formatPunchWindows(wps []model.WorkPeriod) string {
	var parts []string
	for _, wp := range wps {
		if wp.IsBreak {
			continue
		}
		pin := wp.PunchIn.Format("15:04")
		if wp.PunchOut == nil {
			parts = append(parts, pin+"-?")
			continue
		}
		parts = append(parts, pin+"-"+wp.PunchOut.Format("15:04"))
	}
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "; "
		}
		out += p
	}
	return out
}

func round2(x float64) float64 {
	return float64(int64(x*100+0.5)) / 100
}
