package teamoverview

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/daycalc"
	"nfc-time-tracking-server/internal/service/timesummary"
	"nfc-time-tracking-server/internal/service/vacationentitlement"
	"nfc-time-tracking-server/internal/store"
)

// Deps bundles stores needed to compute team overview rows (same roles as export.Data plus users, corrections, vacation).
type Deps struct {
	Users       store.UserStore
	WorkPeriods store.WorkPeriodStore
	Corrections store.CorrectionStore
	Absences    store.AbsenceStore
	Holidays    store.HolidayStore
	Closures    store.ClosureDayStore
	WeeklyHours store.WeeklyHoursStore
	Settings              store.SettingsStore
	FixedNonWorkWeekdays  store.FixedNonWorkWeekdaysStore
	ScheduleBound         store.ScheduleBoundStore
	VacationEnt           store.VacationEntitlementStore
	CompensationDayClaims store.CompensationDayClaimStore
	Schedules             store.ScheduleStore
}

// Row is one team overview line per active non-superadmin user.
type Row struct {
	ID                     int     `json:"id"`
	DisplayName            string  `json:"display_name"`
	HoursBalance           float64 `json:"hours_balance"`
	VacationPlanned        float64 `json:"vacation_planned"`
	VacationFree           float64 `json:"vacation_free"`
	VacationRemainingTotal float64 `json:"vacation_remaining_total"`
	VacationCarryover      float64 `json:"vacation_carryover"` // Rest aus Vorjahr: Anspruch bis 31.12.(J−1) − Urlaub im Kalenderjahr J−1
	VacationEntitlement    float64 `json:"vacation_entitlement"` // Urlaubsanspruch nur Kalenderjahr von „heute“ (Zwölftel/Jahr)
	VacationTaken          float64 `json:"vacation_taken"`       // Urlaub genommen nur im Kalenderjahr von „heute“ (bis heute)
	// VacationOpeningDays ist der in vacation_remaining_total eingerechnete Urlaubs-Startsaldo (users.opening_vacation_days), analog GET /me/vacation.
	VacationOpeningDays float64 `json:"vacation_opening_days"`
	CompensationDayClaimsOpen int `json:"compensation_day_claims_open"`
}

// userHoursRangeStart is the first calendar day included in the hours balance for this user:
// earliest weekly-hours valid_from; if none, 1 January of the calendar year of yesterday (local).
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

// Build aggregates hours balance (per user: earliest weekly-hours valid_from .. yesterday, local calendar;
// without weekly-hours rows from 1 Jan of yesterday's year) and vacation buckets.
// now definiert „heute“ und „gestern“ (Ortszeit). vacation_entitlement = Anspruch nur im Kalenderjahr von „heute“;
// Rest gesamt = Kumulation bis 31.12. − genommener Urlaub (Regeln wie unten) + opening_vacation_days (wie GET /me/vacation); vacation_taken nur aktuelles Jahr bis heute.
// Urlaub geplant = alle Termine nach heute.
// vacationYear nur für Fallback ohne Urlaubsregeln (Genommen innerhalb dieses Jahres).
// vacationYear 0 → Kalenderjahr von now.
// periodStartISO is the earliest user range start in the team (YYYY-MM-DD) for response metadata.
func Build(ctx context.Context, d Deps, vacationYear int, now time.Time) ([]Row, string, error) {
	loc := time.Local

	y, m, day := now.In(loc).Date()
	today := time.Date(y, m, day, 0, 0, 0, 0, loc)
	yesterday := today.AddDate(0, 0, -1)

	if vacationYear == 0 {
		vacationYear = y
	}

	roundMin, breakRules := loadExportSettings(ctx, d.Settings)

	users, err := d.Users.List(ctx, true)
	if err != nil {
		return nil, "", err
	}
	var filtered []model.User
	for _, u := range users {
		if u.Role == model.RoleSuperadmin {
			continue
		}
		filtered = append(filtered, u)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].DisplayName < filtered[j].DisplayName
	})

	todayStr := today.Format("2006-01-02") // YYYY-MM-DD for comparisons with normalized DB dates
	tomorrowStr := today.AddDate(0, 0, 1).Format("2006-01-02")
	const plannedVacationHorizon = "2099-12-31" // alle geplanten Urlaubstage strikt nach heute

	type userPrep struct {
		u                 model.User
		whRows            []model.WeeklyHours
		fnwRows           []model.FixedNonWorkWeekdays
		scheduleBoundRows []model.ScheduleBoundSetting
		startDay          time.Time
	}
	prep := make([]userPrep, 0, len(filtered))
	for _, u := range filtered {
		whRows, err := d.WeeklyHours.ListByUser(ctx, u.ID)
		if err != nil {
			return nil, "", err
		}
		var fnwRows []model.FixedNonWorkWeekdays
		if d.FixedNonWorkWeekdays != nil {
			fnwRows, err = d.FixedNonWorkWeekdays.ListByUser(ctx, u.ID)
			if err != nil {
				return nil, "", err
			}
		}
		var scheduleBoundRows []model.ScheduleBoundSetting
		if d.ScheduleBound != nil {
			scheduleBoundRows, err = d.ScheduleBound.ListByUser(ctx, u.ID)
			if err != nil {
				return nil, "", err
			}
		}
		startDay := userHoursRangeStart(whRows, yesterday, loc)
		prep = append(prep, userPrep{
			u: u, whRows: whRows, fnwRows: fnwRows,
			scheduleBoundRows: scheduleBoundRows, startDay: startDay,
		})
	}

	hasTeamRange := false
	var teamFrom time.Time
	for _, p := range prep {
		if p.startDay.After(yesterday) {
			continue
		}
		if !hasTeamRange || p.startDay.Before(teamFrom) {
			teamFrom = p.startDay
			hasTeamRange = true
		}
	}
	toStr := yesterday.Format("2006-01-02")
	skipTeamHours := !hasTeamRange || teamFrom.After(yesterday)
	periodStartISO := toStr
	fromStr := toStr
	if hasTeamRange {
		periodStartISO = teamFrom.Format("2006-01-02")
		fromStr = periodStartISO
	}

	var holidayByDate map[string]*model.Holiday
	var closureByDate map[string]*model.ClosureDay
	if !skipTeamHours {
		var err error
		holidayByDate, err = loadHolidayMap(ctx, d.Holidays, fromStr, toStr)
		if err != nil {
			return nil, "", err
		}
		closureByDate, err = loadClosureMap(ctx, d.Closures, fromStr, toStr)
		if err != nil {
			return nil, "", err
		}
	}

	yearFrom := fmt.Sprintf("%d-01-01", vacationYear)
	yearTo := fmt.Sprintf("%d-12-31", vacationYear)

	rows := make([]Row, 0, len(prep))
	for _, p := range prep {
		u := p.u
		userStart := p.startDay
		includeOpening := openingDateInRange(u.CreatedAt, userStart, yesterday, loc)
		var hoursBal float64
		skipUser := skipTeamHours || userStart.After(yesterday)
		if !skipUser {
			wps, err := d.WorkPeriods.ListByUserDateRange(ctx, u.ID, fromStr, toStr)
			if err != nil {
				return nil, "", err
			}
			corrs, err := d.Corrections.ListByUser(ctx, u.ID, fromStr, toStr)
			if err != nil {
				return nil, "", err
			}
			latest := latestCorrectionPerPeriod(corrs)
			corrected := applyCorrections(wps, latest)
			byDate := groupWorkPeriodsByDate(corrected)

			hoursAbsences, err := d.Absences.ListByUserDateRange(ctx, u.ID, fromStr, toStr)
			if err != nil {
				return nil, "", err
			}
			hoursAbsByDate := indexFirstAbsenceByDate(hoursAbsences)

			whRows := p.whRows
			fnwRows := p.fnwRows

			var schByDate map[string]*model.Schedule
			if d.Schedules != nil {
				schRows, err := d.Schedules.ListByUserDateRange(ctx, u.ID, fromStr, toStr)
				if err != nil {
					return nil, "", err
				}
				schByDate = indexSchedulesByDate(schRows)
			}

			for dday := userStart; !dday.After(yesterday); dday = dday.AddDate(0, 0, 1) {
				ds := dday.Format("2006-01-02")
				dayWps := byDate[ds]
				var shiftBounds *daycalc.ShiftBounds
				if schByDate != nil {
					if sch := schByDate[ds]; sch != nil {
						bound := model.ScheduleBoundForDate(p.scheduleBoundRows, ds)
						shiftBounds = daycalc.ShiftBoundsIfBound(sch, bound)
					}
				}
				net := daycalc.NetHours(dayWps, breakRules, roundMin, shiftBounds)

				fixed := model.FixedNonWorkWeekdaysForDate(fnwRows, ds)
				var daily float64
				if wh := weeklyHoursForDate(whRows, ds); wh != nil {
					daily = model.DailyHours(wh.HoursPerWeek, fixed)
				}
				hol := holidayByDate[ds]
				abs := hoursAbsByDate[ds]
				clo := closureByDate[ds]
				localD := dday
				target := daycalc.DailyTarget(localD, daily, fixed, hol, abs, clo)
				credit := daycalc.AbsenceCreditHours(localD, daily, fixed, hol, abs, clo)
				hoursBal += (net + credit) - target
			}
		}
		if includeOpening {
			hoursBal += u.OpeningHoursBalance
		}

		vacList, err := d.VacationEnt.ListByUser(ctx, u.ID)
		if err != nil {
			return nil, "", err
		}
		entitlementThrough := vacationentitlement.EndOfCalendarYear(today, loc)
		entitlementCumulative := vacationentitlement.AccruedTwelfthsThroughDateFromList(vacList, entitlementThrough, loc)
		entitlementThisYear := vacationentitlement.AnnualProRataFromList(vacList, y, loc)

		var takenAll, planned float64
		startVac, hasVacRules := vacationentitlement.EarliestValidFromDate(vacList, loc)
		if hasVacRules {
			absTaken, err := d.Absences.ListByUserDateRange(ctx, u.ID, startVac.Format("2006-01-02"), todayStr)
			if err != nil {
				return nil, "", err
			}
			takenAll = timesummary.SumVacationDays(absTaken)
			absPlanned, err := d.Absences.ListByUserDateRange(ctx, u.ID, tomorrowStr, plannedVacationHorizon)
			if err != nil {
				return nil, "", err
			}
			planned = timesummary.SumVacationDays(absPlanned)
		} else {
			absences, err := d.Absences.ListByUserDateRange(ctx, u.ID, yearFrom, yearTo)
			if err != nil {
				return nil, "", err
			}
			for _, a := range absences {
				if a.AbsenceType != model.AbsenceVacation {
					continue
				}
				days := 1.0
				if a.HalfDay {
					days = 0.5
				}
				ad := normDate(a.AbsenceDate)
				if ad <= todayStr {
					takenAll += days
				}
			}
			absPlanned, err := d.Absences.ListByUserDateRange(ctx, u.ID, tomorrowStr, plannedVacationHorizon)
			if err != nil {
				return nil, "", err
			}
			planned = timesummary.SumVacationDays(absPlanned)
		}
		// Urlaubs-Startsaldo wie Me-Vacation: immer in Rest gesamt (nicht an das Eröffnungsdatum-Fenster für Stunden gebunden).
		openingVacation := u.OpeningVacationDays

		prevYear := y - 1
		prevDec31 := time.Date(prevYear, 12, 31, 0, 0, 0, 0, loc)
		prevYearEntitlement := vacationentitlement.AccruedTwelfthsThroughDateFromList(vacList, prevDec31, loc)
		prevYearTaken, err := vacationTakenInCalendarYear(ctx, d, u.ID, prevYear, vacList, hasVacRules, loc)
		if err != nil {
			return nil, "", err
		}
		vacationCarryover := prevYearEntitlement - prevYearTaken

		takenThisYear, err := vacationTakenInCalendarYearThrough(ctx, d, u.ID, y, vacList, hasVacRules, loc, today)
		if err != nil {
			return nil, "", err
		}

		remaining := entitlementCumulative - takenAll + openingVacation
		free := remaining - planned

		var openCompensationDayClaims int
		if d.CompensationDayClaims != nil {
			if n, err := d.CompensationDayClaims.CountOpen(ctx, u.ID); err == nil {
				openCompensationDayClaims = n
			}
		}

		rows = append(rows, Row{
			ID:                        u.ID,
			DisplayName:               u.DisplayName,
			HoursBalance:              round2(hoursBal),
			VacationPlanned:           round2(planned),
			VacationFree:              round2(free),
			VacationRemainingTotal:    round2(remaining),
			VacationCarryover:         round2(vacationCarryover),
			VacationEntitlement:       round2(entitlementThisYear),
			VacationTaken:             round2(takenThisYear),
			VacationOpeningDays:       round2(openingVacation),
			CompensationDayClaimsOpen: openCompensationDayClaims,
		})
	}
	return rows, periodStartISO, nil
}

// vacationTakenInCalendarYear zählt Urlaub im gesamten Kalenderjahr year (bis 31.12.).
func vacationTakenInCalendarYear(ctx context.Context, d Deps, userID int, year int, vacList []model.VacationEntitlement, hasVacRules bool, loc *time.Location) (float64, error) {
	if loc == nil {
		loc = time.Local
	}
	end := time.Date(year, 12, 31, 0, 0, 0, 0, loc)
	return vacationTakenInCalendarYearThrough(ctx, d, userID, year, vacList, hasVacRules, loc, end)
}

// vacationTakenInCalendarYearThrough wie vacationTakenInCalendarYear, aber das Ende ist höchstens throughInclusive (Ortsdatum).
func vacationTakenInCalendarYearThrough(ctx context.Context, d Deps, userID int, year int, vacList []model.VacationEntitlement, hasVacRules bool, loc *time.Location, throughInclusive time.Time) (float64, error) {
	if loc == nil {
		loc = time.Local
	}
	yy, mm, dd := throughInclusive.In(loc).Date()
	throughDay := time.Date(yy, mm, dd, 0, 0, 0, 0, loc)

	yearJan1 := time.Date(year, 1, 1, 0, 0, 0, 0, loc)
	yearDec31 := time.Date(year, 12, 31, 0, 0, 0, 0, loc)
	if throughDay.Before(yearJan1) {
		return 0, nil
	}
	rangeEnd := yearDec31
	if throughDay.Before(yearDec31) {
		rangeEnd = throughDay
	}

	fromStr := yearJan1.Format("2006-01-02")
	toStr := rangeEnd.Format("2006-01-02")

	if hasVacRules {
		startVac, ok := vacationentitlement.EarliestValidFromDate(vacList, loc)
		if !ok {
			return 0, nil
		}
		rangeStart := yearJan1
		if startVac.After(yearJan1) {
			rangeStart = startVac
		}
		if rangeStart.After(rangeEnd) {
			return 0, nil
		}
		fromStr = rangeStart.Format("2006-01-02")
	}

	abs, err := d.Absences.ListByUserDateRange(ctx, userID, fromStr, toStr)
	if err != nil {
		return 0, err
	}
	return timesummary.SumVacationDays(abs), nil
}

func openingDateInRange(createdAt time.Time, asOfDay, yesterday time.Time, loc *time.Location) bool {
	if createdAt.IsZero() || asOfDay.After(yesterday) {
		return false
	}
	cy, cm, cd := createdAt.In(loc).Date()
	openingDay := time.Date(cy, cm, cd, 0, 0, 0, 0, loc)
	return !openingDay.Before(asOfDay) && !openingDay.After(yesterday)
}

func loadExportSettings(ctx context.Context, s store.SettingsStore) (roundMin int, breakRules []model.BreakRule) {
	roundMin = 15
	if v, err := s.Get(ctx, "rounding_minutes"); err == nil {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			roundMin = n
		}
	}
	if v, err := s.Get(ctx, "break_rules"); err == nil {
		_ = json.Unmarshal([]byte(v), &breakRules)
	}
	return roundMin, breakRules
}

func latestCorrectionPerPeriod(corrs []model.TimeCorrection) map[int]model.TimeCorrection {
	// ListByUser orders by created_at DESC — first win per work_period_id is latest.
	out := make(map[int]model.TimeCorrection)
	for _, c := range corrs {
		if _, ok := out[c.WorkPeriodID]; ok {
			continue
		}
		out[c.WorkPeriodID] = c
	}
	return out
}

func applyCorrections(wps []model.WorkPeriod, latest map[int]model.TimeCorrection) []model.WorkPeriod {
	out := make([]model.WorkPeriod, len(wps))
	for i, wp := range wps {
		out[i] = wp
		if c, ok := latest[wp.ID]; ok {
			out[i].PunchIn = c.CorrectedIn
			co := c.CorrectedOut
			out[i].PunchOut = &co
		}
	}
	return out
}

func groupWorkPeriodsByDate(wps []model.WorkPeriod) map[string][]model.WorkPeriod {
	m := make(map[string][]model.WorkPeriod)
	for _, wp := range wps {
		ds := normDate(wp.WorkDate)
		m[ds] = append(m[ds], wp)
	}
	return m
}

// normDate returns YYYY-MM-DD. SQLite may return DATE columns as full RFC3339 timestamps.
func normDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return s
}

func loadHolidayMap(ctx context.Context, hs store.HolidayStore, fromStr, toStr string) (map[string]*model.Holiday, error) {
	fromDay, err := time.ParseInLocation("2006-01-02", fromStr, time.Local)
	if err != nil {
		return nil, fmt.Errorf("holiday map from: %w", err)
	}
	toDay, err := time.ParseInLocation("2006-01-02", toStr, time.Local)
	if err != nil {
		return nil, fmt.Errorf("holiday map to: %w", err)
	}
	m := make(map[string]*model.Holiday)
	for y := fromDay.Year(); y <= toDay.Year(); y++ {
		list, err := hs.ListByYear(ctx, y)
		if err != nil {
			return nil, err
		}
		for i := range list {
			ds := normDate(list[i].HolidayDate)
			if ds < fromStr || ds > toStr {
				continue
			}
			h := &list[i]
			m[ds] = h
		}
	}
	return m, nil
}

func loadClosureMap(ctx context.Context, cs store.ClosureDayStore, fromStr, toStr string) (map[string]*model.ClosureDay, error) {
	all, err := cs.List(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]*model.ClosureDay)
	for i := range all {
		ds := normDate(all[i].ClosureDate)
		if ds < fromStr || ds > toStr {
			continue
		}
		c := &all[i]
		m[ds] = c
	}
	return m, nil
}

// weeklyHoursForDate matches WeeklyHoursStore.GetForDate: greatest valid_from with valid_from ≤ date.
func weeklyHoursForDate(rows []model.WeeklyHours, date string) *model.WeeklyHours {
	date = normDate(date)
	var best *model.WeeklyHours
	for i := range rows {
		wh := &rows[i]
		vf := normDate(wh.ValidFrom)
		if vf > date {
			continue
		}
		if best == nil || vf > normDate(best.ValidFrom) {
			best = wh
		}
	}
	return best
}

func indexFirstAbsenceByDate(abs []model.Absence) map[string]*model.Absence {
	m := make(map[string]*model.Absence)
	for i := range abs {
		ds := normDate(abs[i].AbsenceDate)
		if _, ok := m[ds]; ok {
			continue
		}
		a := &abs[i]
		m[ds] = a
	}
	return m
}

func indexSchedulesByDate(rows []model.Schedule) map[string]*model.Schedule {
	m := make(map[string]*model.Schedule)
	for i := range rows {
		s := &rows[i]
		m[normDate(s.ScheduleDate)] = s
	}
	return m
}

func round2(x float64) float64 {
	return math.Round(x*100) / 100
}
