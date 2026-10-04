package vacationbalance

import (
	"context"
	"fmt"
	"math"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/timesummary"
	"nfc-time-tracking-server/internal/service/vacationentitlement"
	"nfc-time-tracking-server/internal/store"
)

// plannedHorizon: geplant sind alle eingetragenen Urlaubstage strikt nach heute.
const plannedHorizon = "2099-12-31"

// ComputeForUser liefert Urlaubs-KPIs für das Kalenderjahr, in dem `today` (lokales Datum) liegt.
// Siehe Compute für die Definition.
func ComputeForUser(
	ctx context.Context,
	uid int,
	users store.UserStore,
	vacEnt store.VacationEntitlementStore,
	absences store.AbsenceStore,
	today time.Time,
) (*model.VacationBalance, error) {
	u, err := users.GetByID(ctx, uid)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, fmt.Errorf("user %d not found", uid)
	}
	list, err := vacEnt.ListByUser(ctx, uid)
	if err != nil {
		return nil, err
	}
	return Compute(ctx, u, list, absences, today)
}

// Compute ist die einzige Urlaubsrechnung (Mitarbeitende, Mitarbeiter-Detail, Team-Übersicht):
//
//	Startsaldo (Import, users.opening_vacation_days)
//	+ Übertrag aus Vorjahren   = Anspruch bis 31.12. Vorjahr − Urlaub bis 31.12. Vorjahr (ab Beginn der Urlaubsregeln)
//	+ Anspruch aktuelles Jahr  = Anspruch bis 31.12. − Anspruch bis 31.12. Vorjahr
//	= Gesamt
//	− genommen (aktuelles Jahr bis einschließlich heute)
//	= Rest
//	− geplant (alle eingetragenen Urlaubstage nach heute)
//	= frei verplanbar
//
// Ohne Urlaubsregeln gibt es keinen Anspruch und keinen Übertrag; genommen zählt im aktuellen Kalenderjahr.
func Compute(
	ctx context.Context,
	u *model.User,
	list []model.VacationEntitlement,
	absences store.AbsenceStore,
	today time.Time,
) (*model.VacationBalance, error) {
	loc := today.Location()
	y, mo, d := today.Date()
	today0 := time.Date(y, mo, d, 0, 0, 0, 0, loc)
	todayStr := today0.Format("2006-01-02")
	tomorrowStr := today0.AddDate(0, 0, 1).Format("2006-01-02")
	yearStart := time.Date(y, 1, 1, 0, 0, 0, 0, loc)
	prevDec31 := time.Date(y-1, 12, 31, 0, 0, 0, 0, loc)
	dec31 := time.Date(y, 12, 31, 0, 0, 0, 0, loc)

	startVac, hasRules := vacationentitlement.EarliestValidFromDate(list, loc)

	var entitlement, carryover float64
	takenFrom := yearStart
	if hasRules {
		cumPrev := vacationentitlement.AccruedTwelfthsThroughDateFromList(list, prevDec31, loc)
		cumThis := vacationentitlement.AccruedTwelfthsThroughDateFromList(list, dec31, loc)
		entitlement = cumThis - cumPrev

		if startVac.Before(yearStart) {
			prevAbs, err := absences.ListByUserDateRange(ctx, u.ID, startVac.Format("2006-01-02"), prevDec31.Format("2006-01-02"))
			if err != nil {
				return nil, err
			}
			carryover = cumPrev - timesummary.SumVacationDays(prevAbs)
		}
		if startVac.After(takenFrom) {
			takenFrom = startVac
		}
	}

	var taken float64
	if !takenFrom.After(today0) {
		absTaken, err := absences.ListByUserDateRange(ctx, u.ID, takenFrom.Format("2006-01-02"), todayStr)
		if err != nil {
			return nil, err
		}
		taken = timesummary.SumVacationDays(absTaken)
	}

	absPlanned, err := absences.ListByUserDateRange(ctx, u.ID, tomorrowStr, plannedHorizon)
	if err != nil {
		return nil, err
	}
	planned := timesummary.SumVacationDays(absPlanned)

	opening := u.OpeningVacationDays
	total := opening + carryover + entitlement
	remaining := total - taken
	return &model.VacationBalance{
		Year:        y,
		CarriedOver: round2(opening),
		Carryover:   round2(carryover),
		Entitlement: round2(entitlement),
		Total:       round2(total),
		Taken:       round2(taken),
		Remaining:   round2(remaining),
		Planned:     round2(planned),
		Free:        round2(remaining - planned),
	}, nil
}

func round2(x float64) float64 {
	return math.Round(x*100) / 100
}
