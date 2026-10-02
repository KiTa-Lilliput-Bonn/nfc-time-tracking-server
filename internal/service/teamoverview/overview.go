package teamoverview

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/saldocalc"
	"nfc-time-tracking-server/internal/service/vacationbalance"
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
	VacationCarryover      float64 `json:"vacation_carryover"` // Übertrag aus Vorjahren: Anspruch bis 31.12.(J−1) − Urlaub bis 31.12.(J−1)
	VacationEntitlement    float64 `json:"vacation_entitlement"` // Urlaubsanspruch nur Kalenderjahr von „heute“ (Zwölftel/Jahr)
	VacationTaken          float64 `json:"vacation_taken"`       // Urlaub genommen nur im Kalenderjahr von „heute“ (bis heute)
	// VacationOpeningDays ist der in vacation_remaining_total eingerechnete Urlaubs-Startsaldo (users.opening_vacation_days), analog GET /me/vacation.
	VacationOpeningDays float64 `json:"vacation_opening_days"`
	CompensationDayClaimsOpen int `json:"compensation_day_claims_open"`
}

// Build aggregates hours balance (per user: earliest weekly-hours valid_from .. yesterday, local calendar;
// without weekly-hours rows from 1 Jan of yesterday's year) and vacation buckets.
// now definiert „heute“ und „gestern“ (Ortszeit). Stunden: saldocalc.AccountStart/OpeningApplies (wie „Mein Saldo“).
// Urlaub: vacationbalance.Compute (wie GET /me/vacation) für das Kalenderjahr von „heute“.
// vacationYear wird nicht mehr ausgewertet (Urlaub immer für das Kalenderjahr von now); Parameter bleibt für die API.
// periodStartISO is the earliest user range start in the team (YYYY-MM-DD) for response metadata.
func Build(ctx context.Context, d Deps, vacationYear int, now time.Time) ([]Row, string, error) {
	loc := time.Local

	y, m, day := now.In(loc).Date()
	today := time.Date(y, m, day, 0, 0, 0, 0, loc)
	yesterday := today.AddDate(0, 0, -1)

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
		startDay := saldocalc.AccountStart(whRows, yesterday, loc)
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
	if hasTeamRange {
		periodStartISO = teamFrom.Format("2006-01-02")
	}


	rows := make([]Row, 0, len(prep))
	for _, p := range prep {
		u := p.u
		userStart := p.startDay
		includeOpening := saldocalc.OpeningApplies(u.CreatedAt, userStart, yesterday, loc)
		var hoursBal float64
		skipUser := skipTeamHours || userStart.After(yesterday)
		if !skipUser {
			userFrom := userStart.Format("2006-01-02")
			totals, err := saldocalc.SumRange(ctx, saldocalc.Deps{
				WorkPeriods:          d.WorkPeriods,
				Corrections:          d.Corrections,
				Absences:             d.Absences,
				Holidays:             d.Holidays,
				Closures:             d.Closures,
				WeeklyHours:          d.WeeklyHours,
				FixedNonWorkWeekdays: d.FixedNonWorkWeekdays,
				ScheduleBound:        d.ScheduleBound,
				Schedules:            d.Schedules,
				Settings:             d.Settings,
			}, u.ID, userFrom, toStr)
			if err != nil {
				return nil, "", err
			}
			hoursBal = totals.BalanceHours
		}
		if includeOpening {
			hoursBal += u.OpeningHoursBalance
		}

		vacList, err := d.VacationEnt.ListByUser(ctx, u.ID)
		if err != nil {
			return nil, "", err
		}
		vb, err := vacationbalance.Compute(ctx, &u, vacList, d.Absences, now.In(loc))
		if err != nil {
			return nil, "", err
		}

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
			VacationPlanned:           vb.Planned,
			VacationFree:              vb.Free,
			VacationRemainingTotal:    vb.Remaining,
			VacationCarryover:         vb.Carryover,
			VacationEntitlement:       vb.Entitlement,
			VacationTaken:             vb.Taken,
			VacationOpeningDays:       vb.CarriedOver,
			CompensationDayClaimsOpen: openCompensationDayClaims,
		})
	}
	return rows, periodStartISO, nil
}

// normDate returns YYYY-MM-DD. SQLite may return DATE columns as full RFC3339 timestamps.
func normDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return s
}

func round2(x float64) float64 {
	return math.Round(x*100) / 100
}
