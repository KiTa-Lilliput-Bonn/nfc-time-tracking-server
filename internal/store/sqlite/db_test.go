package sqlite

import (
	"testing"
)

func TestOpenAndMigrate(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Verify tables exist by querying sqlite_master
	rows, err := db.DB.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		tables = append(tables, name)
	}

	expected := []string{"absences", "closure_days", "holidays", "nfc_tags",
		"raw_punches", "schedule_week_notes", "schedules", "settings", "time_corrections",
		"users", "vacation_entitlements", "weekly_hours", "work_periods"}

	for _, exp := range expected {
		found := false
		for _, t2 := range tables {
			if t2 == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected table %q not found in %v", exp, tables)
		}
	}
}

func TestDefaultSettings(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	var val string
	err = db.DB.QueryRow("SELECT value FROM settings WHERE key = 'rounding_minutes'").Scan(&val)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if val != "15" {
		t.Errorf("expected rounding_minutes=15, got %s", val)
	}
}
