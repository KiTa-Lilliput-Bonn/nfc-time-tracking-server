package saldocalc

import (
	"context"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store"
)

// monthFull ruft Month für einen vollständig vergangenen Monat auf (now weit in der Zukunft),
// damit die Tests die reine Monatsrechnung prüfen.
func monthFull(
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
	d := Deps{
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
	u := &model.User{ID: userID, OpeningHoursBalance: openingHours}
	return Month(ctx, d, u, year, month, time.Date(2099, 1, 1, 12, 0, 0, 0, time.Local))
}
