package saldocalc

import (
	"context"
	"fmt"
	"time"

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
	holidays store.HolidayStore,
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
	holMonth, err := holidayCreditsSumInRange(ctx, userID, from, to, fnwRows, whs, holidays)
	if err != nil {
		return model.MonthBalance{}, err
	}
	worked += holMonth
	target, err := timesummary.TargetHoursMonthByWeekHistory(ctx, userID, year, month, fnwRows, whs)
	if err != nil {
		return model.MonthBalance{}, err
	}
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
	holYTD, err := holidayCreditsSumInRange(ctx, userID, yearFrom, yTo, fnwRows, whs, holidays)
	if err != nil {
		return model.MonthBalance{}, err
	}
	ytdWorked += holYTD
	var ytdTarget float64
	for mm := 1; mm <= month; mm++ {
		mt, err := timesummary.TargetHoursMonthByWeekHistory(ctx, userID, year, mm, fnwRows, whs)
		if err != nil {
			return model.MonthBalance{}, err
		}
		ytdTarget += mt
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

func holidayCreditsSumInRange(
	ctx context.Context,
	userID int,
	from, to string,
	fnwRows []model.FixedNonWorkWeekdays,
	whs store.WeeklyHoursStore,
	holidays store.HolidayStore,
) (float64, error) {
	if whs == nil || holidays == nil {
		return 0, nil
	}
	loc := time.Local
	a, err := time.ParseInLocation("2006-01-02", from, loc)
	if err != nil {
		return 0, err
	}
	b, err := time.ParseInLocation("2006-01-02", to, loc)
	if err != nil {
		return 0, err
	}
	a = time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, loc)
	b = time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, loc)
	var sum float64
	for d := a; !d.After(b); d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		fixed := model.FixedNonWorkWeekdaysForDate(fnwRows, ds)
		if !model.IsEmployeeWorkday(d, fixed) {
			continue
		}
		hol, err := holidays.GetForDate(ctx, ds)
		if err != nil {
			return 0, err
		}
		if hol == nil || hol.ID == 0 {
			continue
		}
		wh, err := whs.GetForDate(ctx, userID, ds)
		if err != nil {
			return 0, err
		}
		if wh == nil || wh.HoursPerWeek <= 0 {
			continue
		}
		sum += model.DailyHours(wh.HoursPerWeek, fixed)
	}
	return sum, nil
}
