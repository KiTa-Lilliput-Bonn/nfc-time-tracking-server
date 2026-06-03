package model

import "testing"

func TestScheduleBoundForDate_DefaultTrue(t *testing.T) {
	if !ScheduleBoundForDate(nil, "2026-06-01") {
		t.Fatal("empty rows should default to bound")
	}
	if !ScheduleBoundForDate([]ScheduleBoundSetting{}, "2026-06-01") {
		t.Fatal("empty slice should default to bound")
	}
}

func TestScheduleBoundForDate_SqliteTimestampValidFrom(t *testing.T) {
	rows := []ScheduleBoundSetting{
		{ValidFrom: "2026-06-02T00:00:00Z", ScheduleBound: false},
	}
	if ScheduleBoundForDate(rows, "2026-06-02") {
		t.Fatal("timestamp valid_from must match calendar day 2026-06-02")
	}
}

func TestScheduleBoundForDate_History(t *testing.T) {
	rows := []ScheduleBoundSetting{
		{ValidFrom: "2026-01-01", ScheduleBound: true},
		{ValidFrom: "2026-06-01", ScheduleBound: false},
		{ValidFrom: "2027-01-01", ScheduleBound: true},
	}
	if !ScheduleBoundForDate(rows, "2026-05-31") {
		t.Fatal("before unbound entry")
	}
	if ScheduleBoundForDate(rows, "2026-06-15") {
		t.Fatal("during unbound period")
	}
	if !ScheduleBoundForDate(rows, "2027-02-01") {
		t.Fatal("after re-bound entry")
	}
	if !ScheduleBoundForDate(rows, "2025-12-31") {
		t.Fatal("before first entry should default true")
	}
}
