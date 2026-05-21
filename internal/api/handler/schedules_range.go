package handler

import (
	"context"
	"fmt"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store"
)

func schedulesForUserRange(ctx context.Context, sch store.ScheduleStore, userID int, from, to string) ([]model.Schedule, error) {
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		return nil, err
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil {
		return nil, err
	}
	if end.Before(start) {
		return nil, fmt.Errorf("to before from")
	}
	return sch.ListByUserDateRange(ctx, userID, from, to)
}
