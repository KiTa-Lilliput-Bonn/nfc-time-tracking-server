package vacationentitlement

import (
	"math"
	"sort"
	"time"

	"nfc-time-tracking-server/internal/model"
)

// EarliestValidFromMonthFirst ist der erste Kalendermonat, in dem mindestens eine Urlaubsregel gilt.
func EarliestValidFromMonthFirst(list []model.VacationEntitlement, loc *time.Location) (time.Time, bool) {
	if loc == nil {
		loc = time.Local
	}
	var best time.Time
	for i := range list {
		vf := NormalizeVacationEntDate(list[i].ValidFrom)
		if vf == "" {
			continue
		}
		t, err := time.ParseInLocation("2006-01-02", vf, loc)
		if err != nil {
			continue
		}
		m := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
		if best.IsZero() || m.Before(best) {
			best = m
		}
	}
	return best, !best.IsZero()
}

// EarliestValidFromDate ist das früheste gespeicherte valid_from (Kalendertag, nicht auf Monatsanfang gekürzt).
func EarliestValidFromDate(list []model.VacationEntitlement, loc *time.Location) (time.Time, bool) {
	if loc == nil {
		loc = time.Local
	}
	var best time.Time
	for i := range list {
		vf := NormalizeVacationEntDate(list[i].ValidFrom)
		if vf == "" {
			continue
		}
		t, err := time.ParseInLocation("2006-01-02", vf, loc)
		if err != nil {
			continue
		}
		if best.IsZero() || t.Before(best) {
			best = t
		}
	}
	return best, !best.IsZero()
}

// EndOfCalendarYear ist der 31.12. des Kalenderjahres von ref (Ortsdatum).
func EndOfCalendarYear(ref time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	y, _, _ := ref.In(loc).Date()
	return time.Date(y, 12, 31, 0, 0, 0, 0, loc)
}

func ceilHalf(x float64) float64 {
	const eps = 1e-9
	return math.Ceil(x*2 - eps) / 2
}

// monthlyTwelfthContribution ist die Summe der anteiligen Zwölftel für einen Kalendermonat
// (Resttage im Monat / Tage im Monat × days_per_year/12 pro Segment mit konstantem Satz).
func monthlyTwelfthContribution(list []model.VacationEntitlement, year int, month time.Month, loc *time.Location) float64 {
	if len(list) == 0 {
		return 0
	}
	if loc == nil {
		loc = time.Local
	}
	dim := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()

	breaks := map[int]struct{}{1: {}, dim + 1: {}}
	for i := range list {
		vf := NormalizeVacationEntDate(list[i].ValidFrom)
		if vf == "" {
			continue
		}
		t, err := time.ParseInLocation("2006-01-02", vf, loc)
		if err != nil {
			continue
		}
		if t.Year() == year && t.Month() == month {
			breaks[t.Day()] = struct{}{}
		}
	}
	bp := make([]int, 0, len(breaks))
	for d := range breaks {
		bp = append(bp, d)
	}
	sort.Ints(bp)

	var sum float64
	for i := 0; i < len(bp)-1; i++ {
		d1 := bp[i]
		d2 := bp[i+1]
		if d1 > dim {
			break
		}
		segLen := d2 - d1
		if segLen <= 0 {
			continue
		}
		ds := time.Date(year, month, d1, 0, 0, 0, 0, loc).Format("2006-01-02")
		rate := daysPerYearOnDate(list, ds)
		sum += float64(segLen) / float64(dim) * (rate / 12.0)
	}
	return sum
}

// AccruedTwelfthsThroughDateFromList summiert anteilige Zwölftel vom frühesten Urlaubs-Monat
// (EarliestValidFromMonthFirst) bis einschließlich des Kalendermonats von through (der Monat von through
// zählt voll mit), anschließend Aufrundung auf halbe Urlaubstage.
func AccruedTwelfthsThroughDateFromList(list []model.VacationEntitlement, through time.Time, loc *time.Location) float64 {
	if len(list) == 0 {
		return 0
	}
	if loc == nil {
		loc = time.Local
	}
	y, m, d := through.In(loc).Date()
	throughDay := time.Date(y, m, d, 0, 0, 0, 0, loc)
	startMonth, ok := EarliestValidFromMonthFirst(list, loc)
	if !ok {
		return 0
	}
	endMonth := time.Date(throughDay.Year(), throughDay.Month(), 1, 0, 0, 0, 0, loc)
	if startMonth.After(endMonth) {
		return 0
	}
	var sum float64
	for t := startMonth; !t.After(endMonth); t = t.AddDate(0, 1, 0) {
		sum += monthlyTwelfthContribution(list, t.Year(), t.Month(), loc)
	}
	return ceilHalf(sum)
}

// AnnualProRataFromList ist die Berechnung für genau ein Kalenderjahr (Summe der Monats-Zwölftel, Rundung halbe Tage).
func AnnualProRataFromList(list []model.VacationEntitlement, year int, loc *time.Location) float64 {
	if len(list) == 0 {
		return 0
	}
	if loc == nil {
		loc = time.Local
	}
	var sum float64
	for mo := 1; mo <= 12; mo++ {
		sum += monthlyTwelfthContribution(list, year, time.Month(mo), loc)
	}
	return ceilHalf(sum)
}

// daysPerYearOnDate wählt den Eintrag mit dem größten valid_from, das noch ≤ date ist.
// Ein späterer Eintrag beendet den vorherigen Block implizit (kein valid_until).
func daysPerYearOnDate(list []model.VacationEntitlement, date string) float64 {
	var best *model.VacationEntitlement
	for i := range list {
		ve := &list[i]
		vf := NormalizeVacationEntDate(ve.ValidFrom)
		if vf > date {
			continue
		}
		if best == nil || vf > NormalizeVacationEntDate(best.ValidFrom) {
			best = ve
		}
	}
	if best == nil {
		return 0
	}
	return best.DaysPerYear
}
