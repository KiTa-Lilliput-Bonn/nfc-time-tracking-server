package scheduleimport

import (
	"context"
	"errors"
	"fmt"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/fixednonwork"
	"nfc-time-tracking-server/internal/store"
)

func fixedNonWorkWeekdaysForUser(ctx context.Context, fnw store.FixedNonWorkWeekdaysStore, uid int, dateStr string) []int {
	return fixednonwork.WeekdaysForUserDate(ctx, fnw, uid, dateStr)
}

var germanWeekdayName = map[time.Weekday]string{
	time.Sunday:    "Sonntag",
	time.Monday:    "Montag",
	time.Tuesday:   "Dienstag",
	time.Wednesday: "Mittwoch",
	time.Thursday:  "Donnerstag",
	time.Friday:    "Freitag",
	time.Saturday:  "Samstag",
}

func validateVacationAbsenceDate(ctx context.Context, holidays store.HolidayStore, closures store.ClosureDayStore, fixedNonWork []int, dateStr string) error {
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return errors.New("Ungültiges Datum")
	}
	if !model.IsEmployeeWorkday(t, fixedNonWork) {
		wd := t.Weekday()
		if wd == time.Saturday || wd == time.Sunday {
			return fmt.Errorf(
				"Am %s, %s, kann kein Urlaub gebucht werden — das ist ein Wochenende.",
				germanWeekdayName[wd], t.Format("02.01.2006"),
			)
		}
		return fmt.Errorf(
			"Am %s, %s, kann kein Urlaub gebucht werden — das ist ein fester freier Wochentag (Dienstplan).",
			germanWeekdayName[wd], t.Format("02.01.2006"),
		)
	}
	wd := t.Weekday()
	h, err := holidays.GetForDate(ctx, dateStr)
	if err != nil {
		return err
	}
	if h != nil {
		return fmt.Errorf(
			"Am %s, %s, kann kein Urlaub gebucht werden — „%s“ ist ein gesetzlicher Feiertag.",
			germanWeekdayName[wd], t.Format("02.01.2006"), h.Name,
		)
	}
	if closures != nil {
		clo, err := closures.GetForDate(ctx, dateStr)
		if err != nil {
			return err
		}
		if clo != nil && clo.ID != 0 {
			return fmt.Errorf(
				"Am %s, %s, kann kein Urlaub gebucht werden — „%s“ ist ein Schließtag.",
				germanWeekdayName[wd], t.Format("02.01.2006"), clo.Name,
			)
		}
	}
	return nil
}

func validateCompensationDayAbsenceDate(ctx context.Context, holidays store.HolidayStore, fixedNonWork []int, dateStr string) error {
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return errors.New("Ungültiges Datum")
	}
	if !model.IsEmployeeWorkday(t, fixedNonWork) {
		wd := t.Weekday()
		if wd == time.Saturday || wd == time.Sunday {
			return fmt.Errorf(
				"Am %s, %s, kann kein Ausgleichstag gebucht werden — das ist ein Wochenende.",
				germanWeekdayName[wd], t.Format("02.01.2006"),
			)
		}
		return fmt.Errorf(
			"Am %s, %s, kann kein Ausgleichstag gebucht werden — das ist ein fester freier Wochentag (Dienstplan).",
			germanWeekdayName[wd], t.Format("02.01.2006"),
		)
	}
	wd := t.Weekday()
	h, err := holidays.GetForDate(ctx, dateStr)
	if err != nil {
		return err
	}
	if h != nil {
		return fmt.Errorf(
			"Am %s, %s, kann kein Ausgleichstag gebucht werden — „%s“ ist ein gesetzlicher Feiertag.",
			germanWeekdayName[wd], t.Format("02.01.2006"), h.Name,
		)
	}
	return nil
}
