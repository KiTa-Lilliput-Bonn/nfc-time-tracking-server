package handler

import (
	"context"
	"time"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/entrylock"
	"nfc-time-tracking-server/internal/service/fixednonwork"
	"nfc-time-tracking-server/internal/store"
)

type employeeJSON struct {
	model.User
	FixedNonWorkWeekdays []int `json:"fixed_non_work_weekdays,omitempty"`
}

func employeeJSONForUser(ctx context.Context, fnw store.FixedNonWorkWeekdaysStore, u model.User, date string) employeeJSON {
	return employeeJSON{
		User:                 u,
		FixedNonWorkWeekdays: fixednonwork.WeekdaysForUserDate(ctx, fnw, u.ID, date),
	}
}

func employeesJSONForList(ctx context.Context, fnw store.FixedNonWorkWeekdaysStore, users []model.User) []employeeJSON {
	today := time.Now().In(time.Local).Format("2006-01-02")
	out := make([]employeeJSON, len(users))
	for i, u := range users {
		out[i] = employeeJSONForUser(ctx, fnw, u, today)
	}
	return out
}

type fixedNonWorkWeekdaysBody struct {
	Weekdays  []int  `json:"weekdays"`
	ValidFrom string `json:"valid_from"`
}

type fixedNonWorkWeekdaysResponse struct {
	model.FixedNonWorkWeekdays
	Mutable bool `json:"mutable"`
}

func fixedNonWorkWeekdaysResponses(list []model.FixedNonWorkWeekdays) []fixedNonWorkWeekdaysResponse {
	now := time.Now().UTC()
	out := make([]fixedNonWorkWeekdaysResponse, len(list))
	for i, row := range list {
		out[i] = fixedNonWorkWeekdaysResponse{
			FixedNonWorkWeekdays: row,
			Mutable:              entrylock.IsMutable(row.CreatedAt, now),
		}
	}
	return out
}
