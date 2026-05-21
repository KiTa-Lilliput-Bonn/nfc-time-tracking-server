package vacationbalance

import (
	"context"
	"fmt"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/timesummary"
	"nfc-time-tracking-server/internal/service/vacationentitlement"
	"nfc-time-tracking-server/internal/store"
)

// ComputeForUser liefert Urlaubs-KPIs für das Kalenderjahr, in dem `today` (lokales Datum) liegt.
func ComputeForUser(
	ctx context.Context,
	uid int,
	users store.UserStore,
	vacEnt store.VacationEntitlementStore,
	absences store.AbsenceStore,
	today time.Time,
) (*model.VacationBalance, error) {
	loc := today.Location()
	y, mo, d := today.Date()
	today0 := time.Date(y, mo, d, 0, 0, 0, 0, loc)
	todayStr := today0.Format("2006-01-02")
	year := y

	list, err := vacEnt.ListByUser(ctx, uid)
	if err != nil {
		return nil, err
	}
	// Baseline: Stand 01.01. des aktuellen Jahres = Übertrag (Vorjahr) + Startsaldo + Anspruch (Jahr).
	entitlement := vacationentitlement.AnnualProRataFromList(list, year, loc)

	prevYear := year - 1
	prevDec31 := time.Date(prevYear, 12, 31, 0, 0, 0, 0, loc)
	prevYearEntitlement := vacationentitlement.AccruedTwelfthsThroughDateFromList(list, prevDec31, loc)
	prevYearFrom := fmt.Sprintf("%d-01-01", prevYear)
	prevYearTo := fmt.Sprintf("%d-12-31", prevYear)
	prevAbs, err := absences.ListByUserDateRange(ctx, uid, prevYearFrom, prevYearTo)
	if err != nil {
		return nil, err
	}
	carryover := prevYearEntitlement - timesummary.SumVacationDays(prevAbs)

	var fromStr string
	startVac, hasRules := vacationentitlement.EarliestValidFromDate(list, loc)
	if hasRules {
		yearStart := time.Date(year, 1, 1, 0, 0, 0, 0, loc)
		rangeStart := yearStart
		if startVac.After(yearStart) {
			rangeStart = startVac
		}
		fromStr = rangeStart.Format("2006-01-02")
	} else {
		fromStr = fmt.Sprintf("%d-01-01", year)
	}

	absTaken, err := absences.ListByUserDateRange(ctx, uid, fromStr, todayStr)
	if err != nil {
		return nil, err
	}
	takenPast := timesummary.SumVacationDays(absTaken)

	var planned float64
	yearEnd := time.Date(year, 12, 31, 0, 0, 0, 0, loc)
	tomorrow := today0.AddDate(0, 0, 1)
	if !tomorrow.After(yearEnd) {
		tomorrowStr := tomorrow.Format("2006-01-02")
		yearEndStr := yearEnd.Format("2006-01-02")
		absPlanned, err := absences.ListByUserDateRange(ctx, uid, tomorrowStr, yearEndStr)
		if err != nil {
			return nil, err
		}
		planned = timesummary.SumVacationDays(absPlanned)
	}

	u, err := users.GetByID(ctx, uid)
	if err != nil {
		return nil, err
	}
	openV := u.OpeningVacationDays
	rem := openV + entitlement - takenPast
	if !hasRules {
		rem -= planned
	}
	return &model.VacationBalance{
		Year: year, Entitlement: entitlement, Carryover: carryover,
		Taken: takenPast, Planned: planned,
		Remaining: rem, CarriedOver: openV,
	}, nil
}
