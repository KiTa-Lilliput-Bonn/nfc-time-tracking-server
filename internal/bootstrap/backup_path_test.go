package bootstrap

import (
	"context"
	"testing"

	"nfc-time-tracking-server/internal/service/backup"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestSeedBackupTargetPath_setsWhenEmpty(t *testing.T) {
	db, err := sqlite.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := sqlite.NewSettingsStore(db)
	ctx := context.Background()

	if err := SeedBackupTargetPath(ctx, s, "/backup"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, backup.SettingTargetPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/backup" {
		t.Fatalf("got %q want /backup", got)
	}
	enabled, err := s.Get(ctx, backup.SettingEnabled)
	if err != nil {
		t.Fatal(err)
	}
	if enabled == "true" {
		t.Fatalf("backup should stay disabled, got enabled=%q", enabled)
	}
}

func TestSeedBackupTargetPath_skipsWhenSet(t *testing.T) {
	db, err := sqlite.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := sqlite.NewSettingsStore(db)
	ctx := context.Background()
	if err := s.Set(ctx, backup.SettingTargetPath, "/existing"); err != nil {
		t.Fatal(err)
	}
	if err := SeedBackupTargetPath(ctx, s, "/backup"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(ctx, backup.SettingTargetPath)
	if got != "/existing" {
		t.Fatalf("got %q want /existing", got)
	}
}

func TestSeedBackupTargetPath_emptyPathNoOp(t *testing.T) {
	db, err := sqlite.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := sqlite.NewSettingsStore(db)
	ctx := context.Background()
	if err := SeedBackupTargetPath(ctx, s, ""); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, backup.SettingTargetPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q want empty", got)
	}
}
