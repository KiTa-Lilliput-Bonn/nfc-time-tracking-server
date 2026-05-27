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
	whs store.WeeklyHoursStore,
	schedules store.ScheduleStore,
) (model.MonthBalance, error) {
	if month < 1 || month > 12 {
		return model.MonthBalance{}, fmt.Errorf("invalid month")
	}
	fnwRows, err := fnw.ListByUser(ctx, userID)
	if err != nil {
		return model.MonthBalance{}, err
	}
	from, to := timesummary.MonthDateRange(year, month)
	periods, err := wps.ListByUserDateRange(ctx, userID, from, to)
	if err != nil {
		return model.MonthBalance{}, err
	}
	worked, err := timesummary.SumWorkedHoursFromStore(ctx, userID, periods, schedules, nil)
	if err != nil {
		return model.MonthBalance{}, err
	}
	var hpw float64
	if wh, err := whs.GetForDate(ctx, userID, from); err == nil && wh != nil {
		hpw = wh.HoursPerWeek
	}
	target := timesummary.TargetHoursMonth(hpw, year, month, fnwRows)
	balM := worked - target

	yearFrom := fmt.Sprintf("%d-01-01", year)
	_, yTo := timesummary.MonthDateRange(year, month)
	ytdP, err := wps.ListByUserDateRange(ctx, userID, yearFrom, yTo)
	if err != nil {
		return model.MonthBalance{}, err
	}
	ytdWorked, err := timesummary.SumWorkedHoursFromStore(ctx, userID, ytdP, schedules, nil)
	if err != nil {
		return model.MonthBalance{}, err
	}
	var ytdTarget float64
	for mm := 1; mm <= month; mm++ {
		mf, _ := timesummary.MonthDateRange(year, mm)
		var h float64
		if wh, err := whs.GetForDate(ctx, userID, mf); err == nil && wh != nil {
			h = wh.HoursPerWeek
		}
		ytdTarget += timesummary.TargetHoursMonth(h, year, mm, fnwRows)
	}
	ytdDelta := ytdWorked - ytdTarget
	total := openingHours + ytdDelta
	carry := total - balM

	return model.MonthBalance{
		Year: year, Month: month,
		WorkedHours: worked, TargetHours: target, BalanceHours: balM,
		Carryover: carry, TotalBalance: total,
	}, nil
}
