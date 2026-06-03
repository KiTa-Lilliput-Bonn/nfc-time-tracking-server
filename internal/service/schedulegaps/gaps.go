package schedulegaps

import (
	"context"
	"sort"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store"
)

// Deps bundles stores needed to detect planned shifts without work or blocking absence.
type Deps struct {
	Users       store.UserStore
	Schedules   store.ScheduleStore
	WorkPeriods store.WorkPeriodStore
	Absences    store.AbsenceStore
	WeeklyHours store.WeeklyHoursStore
}

// Item is one open schedule gap (planned shift, no work, no blocking absence, through yesterday).
type Item struct {
	UserID       int    `json:"user_id"`
	DisplayName  string `json:"display_name"`
	ScheduleDate string `json:"schedule_date"`
	ShiftStart   string `json:"shift_start"`
	ShiftEnd     string `json:"shift_end"`
	ISOWeekYear  int    `json:"iso_week_year"`
	ISOWeek      int    `json:"iso_week"`
}

// Result is the schedule-gaps API payload.
type Result struct {
	From    string `json:"from"`
	Through string `json:"through"`
	Count   int    `json:"count"`
	Items   []Item `json:"items"`
}

// Build lists gap days per active non-superadmin user from userHoursRangeStart through yesterday (local).
// from is the earliest user range start in the team (YYYY-MM-DD).
func Build(ctx context.Context, d Deps, now time.Time) (Result, error) {
	loc := time.Local
	y, m, day := now.In(loc).Date()
	today := time.Date(y, m, day, 0, 0, 0, 0, loc)
	yesterday := today.AddDate(0, 0, -1)
	yesterdayStr := yesterday.Format("2006-01-02")

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

	var items []Item
	earliestStart := time.Time{}
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
		fromStr := startDay.Format("2006-01-02")

		schedules, err := d.Schedules.ListByUserDateRange(ctx, u.ID, fromStr, yesterdayStr)
		if err != nil {
			return Result{}, err
		}
		absences, err := d.Absences.ListByUserDateRange(ctx, u.ID, fromStr, yesterdayStr)
		if err != nil {
			return Result{}, err
		}
		wps, err := d.WorkPeriods.ListByUserDateRange(ctx, u.ID, fromStr, yesterdayStr)
		if err != nil {
			return Result{}, err
		}

		absByDate := absenceByDate(absences)
		wpByDate := workPeriodsByDate(wps)

		for _, sch := range schedules {
			ds := normDate(sch.ScheduleDate)
			if ds > yesterdayStr {
				continue
			}
			if ds < fromStr {
				continue
			}
			if !hasPlannedShift(sch.ShiftStart, sch.ShiftEnd) {
				continue
			}
			if isBlockingAbsence(absByDate[ds]) {
				continue
			}
			if hasNonBreakWork(wpByDate[ds]) {
				continue
			}
			dt, err := time.ParseInLocation("2006-01-02", ds, loc)
			if err != nil {
				continue
			}
			isoY, isoW := dt.ISOWeek()
			items = append(items, Item{
				UserID:       u.ID,
				DisplayName:  u.DisplayName,
				ScheduleDate: ds,
				ShiftStart:   strings.TrimSpace(sch.ShiftStart),
				ShiftEnd:     strings.TrimSpace(sch.ShiftEnd),
				ISOWeekYear:  isoY,
				ISOWeek:      isoW,
			})
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].ScheduleDate != items[j].ScheduleDate {
			return items[i].ScheduleDate > items[j].ScheduleDate
		}
		return items[i].DisplayName < items[j].DisplayName
	})

	fromISO := yesterdayStr
	if !firstEarliest {
		fromISO = earliestStart.Format("2006-01-02")
	}

	return Result{
		From:    fromISO,
		Through: yesterdayStr,
		Count:   len(items),
		Items:   items,
	}, nil
}

func hasPlannedShift(shiftStart, shiftEnd string) bool {
	return strings.TrimSpace(shiftStart) != "" && strings.TrimSpace(shiftEnd) != ""
}

func isBlockingAbsence(abs *model.Absence) bool {
	if abs == nil {
		return false
	}
	switch abs.AbsenceType {
	case model.AbsenceVacation, model.AbsenceSick, model.AbsenceOther, model.AbsenceCompensationDay:
		return true
	default:
		return false
	}
}

func hasNonBreakWork(wps []model.WorkPeriod) bool {
	for i := range wps {
		if !wps[i].IsBreak {
			return true
		}
	}
	return false
}

func absenceByDate(absences []model.Absence) map[string]*model.Absence {
	m := make(map[string]*model.Absence)
	for i := range absences {
		ds := normDate(absences[i].AbsenceDate)
		m[ds] = &absences[i]
	}
	return m
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
