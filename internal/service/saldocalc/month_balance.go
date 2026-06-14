package saldocalc

import (
	"context"
	"fmt"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/timesummary"
	"nfc-time-tracking-server/internal/store"
)

// MonthWithOpening builds month view and YTD total: opening + Σ(Ist−Soll) Jan..Monat.
func MonthWithOpening(
	ctx context.Context,
	userID int,
	year, month int,
	openingHours float64,
	fnw store.FixedNonWorkWeekdaysStore,
	wps store.WorkPeriodStore,
	corrections store.CorrectionStore,
	whs store.WeeklyHoursStore,
	holidays store.HolidayStore,
	absences store.AbsenceStore,
	schedules store.ScheduleStore,
	scheduleBound store.ScheduleBoundStore,
	closures store.ClosureDayStore,
	settings store.SettingsStore,
) (model.MonthBalance, error) {
	if month < 1 || month > 12 {
		return model.MonthBalance{}, fmt.Errorf("invalid month")
	}

	deps := Deps{
		WorkPeriods:          wps,
		Corrections:          corrections,
		Absences:             absences,
		Holidays:             holidays,
		Closures:             closures,
		WeeklyHours:          whs,
		FixedNonWorkWeekdays: fnw,
		ScheduleBound:        scheduleBound,
		Schedules:            schedules,
		Settings:             settings,
	}

	from, to := timesummary.MonthDateRange(year, month)
	monthTotals, err := SumRange(ctx, deps, userID, from, to)
	if err != nil {
		return model.MonthBalance{}, err
	}

	yearFrom := fmt.Sprintf("%d-01-01", year)
	ytdTotals, err := SumRange(ctx, deps, userID, yearFrom, to)
	if err != nil {
		return model.MonthBalance{}, err
	}

	total := openingHours + ytdTotals.BalanceHours
	carry := total - monthTotals.BalanceHours

	return model.MonthBalance{
		Year: year, Month: month,
		WorkedHours: monthTotals.WorkedHours, TargetHours: monthTotals.TargetHours, BalanceHours: monthTotals.BalanceHours,
		Carryover: carry, TotalBalance: total,
	}, nil
}
