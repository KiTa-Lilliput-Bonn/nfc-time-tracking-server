package shiftalerts

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/timesummary"
	"nfc-time-tracking-server/internal/store"
)

const (
	ReasonLongDuration = "long_duration"
	ReasonLateEnd      = "late_end"
)

// Deps bundles stores needed to detect unusual work times.
type Deps struct {
	Users       store.UserStore
	WorkPeriods store.WorkPeriodStore
	Corrections store.CorrectionStore
	WeeklyHours store.WeeklyHoursStore
	Dismissals  store.ShiftAlertDismissalStore
}

// Item is one day with unusual recorded work times.
type Item struct {
	UserID               int      `json:"user_id"`
	DisplayName          string   `json:"display_name"`
	WorkDate             string   `json:"work_date"`
	TotalDurationMinutes int      `json:"total_duration_minutes"`
	LatestEnd            string   `json:"latest_end"`
	Reasons              []string `json:"reasons"`
	ISOWeekYear          int      `json:"iso_week_year"`
	ISOWeek              int      `json:"iso_week"`
}

// Result is the shift-alerts API payload.
type Result struct {
	From    string `json:"from"`
	Through string `json:"through"`
	Count   int    `json:"count"`
	Items   []Item `json:"items"`
}

// Build lists days per active non-superadmin user with long or late work times through yesterday.
func Build(ctx context.Context, d Deps, cfg Config, now time.Time) (Result, error) {
	loc := time.Local
	y, m, day := now.In(loc).Date()
	today := time.Date(y, m, day, 0, 0, 0, 0, loc)
	yesterday := today.AddDate(0, 0, -1)
	yesterdayStr := yesterday.Format("2006-01-02")

	checkLong := cfg.MaxHours > 0
	checkLate := strings.TrimSpace(cfg.LateEnd) != ""
	if !checkLong && !checkLate {
		return Result{From: yesterdayStr, Through: yesterdayStr, Count: 0, Items: nil}, nil
	}

	maxMinutes := int(cfg.MaxHours * 60)
	lateEndMinutes, lateOK := parseHHMMToMinutes(cfg.LateEnd)
	if checkLate && !lateOK {
		checkLate = false
	}

	users, err := d.Users.List(ctx, true)
	if err != nil {
		return Result{}, err
	}
	var filtered []model.User
	for _, u := range users {
		if u.Role == model.RoleSuperadmin {
			continue
		}
		filtered = append(filtered, u)
	}

	var earliestStart time.Time
	firstEarliest := true
	for _, u := range filtered {
		whRows, err := d.WeeklyHours.ListByUser(ctx, u.ID)
		if err != nil {
			return Result{}, err
		}
		startDay := userHoursRangeStart(whRows, yesterday, loc)
		if firstEarliest || startDay.Before(earliestStart) {
			earliestStart = startDay
			firstEarliest = false
		}
	}

	fromStr := yesterdayStr
	if !firstEarliest {
		fromStr = earliestStart.Format("2006-01-02")
	}

	dismissed := make(map[string]struct{})
	if d.Dismissals != nil {
		rows, err := d.Dismissals.ListByDateRange(ctx, fromStr, yesterdayStr)
		if err != nil {
			return Result{}, err
		}
		for _, row := range rows {
			dismissed[dismissKey(row.UserID, row.WorkDate)] = struct{}{}
		}
	}

	var items []Item
	for _, u := range filtered {
		whRows, err := d.WeeklyHours.ListByUser(ctx, u.ID)
		if err != nil {
			return Result{}, err
		}
		userFrom := userHoursRangeStart(whRows, yesterday, loc).Format("2006-01-02")

		wps, err := d.WorkPeriods.ListByUserDateRange(ctx, u.ID, userFrom, yesterdayStr)
		if err != nil {
			return Result{}, err
		}
		corrs, err := d.Corrections.ListByUser(ctx, u.ID, userFrom, yesterdayStr)
		if err != nil {
			return Result{}, err
		}
		effective := timesummary.ApplyLatestCorrections(wps, corrs)
		byDate := workPeriodsByDate(effective)

		for ds, dayPeriods := range byDate {
			if ds > yesterdayStr || ds < userFrom {
				continue
			}
			if _, ok := dismissed[dismissKey(u.ID, ds)]; ok {
				continue
			}

			totalMin, latestEnd, hasClosed := dayStats(dayPeriods, loc)
			if !hasClosed {
				continue
			}

			var reasons []string
			if checkLong && totalMin > maxMinutes {
				reasons = append(reasons, ReasonLongDuration)
			}
			if checkLate && endsAfter(latestEnd, lateEndMinutes, loc) {
				reasons = append(reasons, ReasonLateEnd)
			}
			if len(reasons) == 0 {
				continue
			}

			dt, err := time.ParseInLocation("2006-01-02", ds, loc)
			if err != nil {
				continue
			}
			isoY, isoW := dt.ISOWeek()
			items = append(items, Item{
				UserID:               u.ID,
				DisplayName:          u.DisplayName,
				WorkDate:             ds,
				TotalDurationMinutes: totalMin,
				LatestEnd:            formatHHMM(latestEnd.In(loc)),
				Reasons:              reasons,
				ISOWeekYear:          isoY,
				ISOWeek:              isoW,
			})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].WorkDate != items[j].WorkDate {
			return items[i].WorkDate > items[j].WorkDate
		}
		return items[i].DisplayName < items[j].DisplayName
	})

	return Result{
		From:    fromStr,
		Through: yesterdayStr,
		Count:   len(items),
		Items:   items,
	}, nil
}

func dismissKey(userID int, workDate string) string {
	return normDate(workDate) + "|" + strconv.Itoa(userID)
}

func dayStats(periods []model.WorkPeriod, loc *time.Location) (totalMin int, latestEnd time.Time, hasClosed bool) {
	for _, wp := range periods {
		if wp.IsBreak || wp.PunchOut == nil {
			continue
		}
		hasClosed = true
		dur := wp.PunchOut.Sub(wp.PunchIn)
		if dur > 0 {
			totalMin += int(dur.Minutes())
		}
		out := wp.PunchOut.In(loc)
		if latestEnd.IsZero() || out.After(latestEnd) {
			latestEnd = out
		}
	}
	return totalMin, latestEnd, hasClosed
}

func endsAfter(latestEnd time.Time, thresholdMinutes int, loc *time.Location) bool {
	if latestEnd.IsZero() {
		return false
	}
	t := latestEnd.In(loc)
	mins := t.Hour()*60 + t.Minute()
	return mins > thresholdMinutes
}

func parseHHMMToMinutes(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	h, err1 := strconv.Atoi(s[:2])
	m, err2 := strconv.Atoi(s[3:])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

func formatHHMM(t time.Time) string {
	return t.Format("15:04")
}

func workPeriodsByDate(wps []model.WorkPeriod) map[string][]model.WorkPeriod {
	m := make(map[string][]model.WorkPeriod)
	for i := range wps {
		ds := normDate(wps[i].WorkDate)
		m[ds] = append(m[ds], wps[i])
	}
	return m
}

func userHoursRangeStart(whRows []model.WeeklyHours, yesterday time.Time, loc *time.Location) time.Time {
	yy, _, _ := yesterday.In(loc).Date()
	fallback := time.Date(yy, 1, 1, 0, 0, 0, 0, loc)
	if len(whRows) == 0 {
		return fallback
	}
	var best time.Time
	first := true
	for i := range whRows {
		vf := normDate(whRows[i].ValidFrom)
		d, err := time.ParseInLocation("2006-01-02", vf, loc)
		if err != nil {
			continue
		}
		d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
		if first || d.Before(best) {
			best = d
			first = false
		}
	}
	if first {
		return fallback
	}
	return best
}

func normDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return s
}
