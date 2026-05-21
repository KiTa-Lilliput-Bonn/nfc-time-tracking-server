package handler

import (
	"context"
	"testing"

	"nfc-time-tracking-server/internal/model"
)

func TestValidateCompensationDayAbsenceDate(t *testing.T) {
	ctx := context.Background()
	st := stubHolidayStore{}

	if err := validateCompensationDayAbsenceDate(ctx, st, nil, "2026-04-07", true); err == nil {
		t.Fatal("expected half-day error")
	}
	if err := validateCompensationDayAbsenceDate(ctx, st, nil, "2026-04-04", false); err == nil {
		t.Fatal("expected weekend error")
	}
	holidayStore := stubHolidayStore{
		getForDate: func(_ context.Context, date string) (*model.Holiday, error) {
			if date == "2026-04-06" {
				return &model.Holiday{HolidayDate: date, Name: "Ostermontag"}, nil
			}
			return nil, nil
		},
	}
	if err := validateCompensationDayAbsenceDate(ctx, holidayStore, nil, "2026-04-06", false); err == nil {
		t.Fatal("expected holiday error")
	}
	if err := validateCompensationDayAbsenceDate(ctx, st, nil, "2026-04-07", false); err != nil {
		t.Fatal(err)
	}
}
