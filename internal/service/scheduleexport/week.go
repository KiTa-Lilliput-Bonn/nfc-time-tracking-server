package scheduleexport

import (
	"fmt"
	"time"

	sqlitesched "nfc-time-tracking-server/internal/store/sqlite"
)

// isoWeekPair ist ein ISO-Wochen-Jahr und Kalenderwoche.
type isoWeekPair struct {
	Year int
	Week int
}

func compareISOWeek(a, b isoWeekPair) int {
	aMon, err := mondayOfISOWeek(a.Year, a.Week)
	if err != nil {
		return 0
	}
	bMon, err := mondayOfISOWeek(b.Year, b.Week)
	if err != nil {
		return 0
	}
	if aMon.Before(bMon) {
		return -1
	}
	if aMon.After(bMon) {
		return 1
	}
	return 0
}

func mondayOfISOWeek(year, week int) (time.Time, error) {
	from, _, err := sqlitesched.ISOWeekMondayFriday(year, week)
	if err != nil {
		return time.Time{}, err
	}
	return time.ParseInLocation("2006-01-02", from, time.Local)
}

func iterateISOWeeks(fromYear, fromWeek, toYear, toWeek int) ([]isoWeekPair, error) {
	start := isoWeekPair{Year: fromYear, Week: fromWeek}
	end := isoWeekPair{Year: toYear, Week: toWeek}
	if compareISOWeek(start, end) > 0 {
		return nil, fmt.Errorf("End-KW liegt vor Start-KW")
	}
	var out []isoWeekPair
	cur := start
	for {
		out = append(out, cur)
		if cur.Year == end.Year && cur.Week == end.Week {
			break
		}
		next, err := nextISOWeek(cur.Year, cur.Week)
		if err != nil {
			return nil, err
		}
		cur = next
		if len(out) > 60*53 {
			return nil, fmt.Errorf("Zeitraum zu groß")
		}
	}
	return out, nil
}

func nextISOWeek(year, week int) (isoWeekPair, error) {
	mon, err := mondayOfISOWeek(year, week)
	if err != nil {
		return isoWeekPair{}, err
	}
	nextMon := mon.AddDate(0, 0, 7)
	y, w := nextMon.ISOWeek()
	return isoWeekPair{Year: y, Week: w}, nil
}

func weekDates(year, week int) (monday time.Time, dates [5]time.Time, friday time.Time, err error) {
	from, to, err := sqlitesched.ISOWeekMondayFriday(year, week)
	if err != nil {
		return time.Time{}, [5]time.Time{}, time.Time{}, err
	}
	monday, err = time.ParseInLocation("2006-01-02", from, time.Local)
	if err != nil {
		return time.Time{}, [5]time.Time{}, time.Time{}, err
	}
	friday, err = time.ParseInLocation("2006-01-02", to, time.Local)
	if err != nil {
		return time.Time{}, [5]time.Time{}, time.Time{}, err
	}
	for i := 0; i < 5; i++ {
		dates[i] = monday.AddDate(0, 0, i)
	}
	return monday, dates, friday, nil
}

func formatWeekHeader(monday, friday time.Time) string {
	return fmt.Sprintf("%02d.%02d. - %02d.%02d.%d",
		monday.Day(), int(monday.Month()),
		friday.Day(), int(friday.Month()), friday.Year())
}

func formatShortGermanDate(t time.Time) string {
	return fmt.Sprintf("%02d.%02d.", t.Day(), int(t.Month()))
}
