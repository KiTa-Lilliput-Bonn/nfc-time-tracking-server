package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromFile(t *testing.T) {
	content := []byte(`
server:
  port: 9090
  host: "127.0.0.1"
database:
  path: "./test.db"
auth:
  jwt_secret: "testsecret"
  token_expiry_hours: 4
logging:
  level: "debug"
  file: "./test.log"
  max_age_days: 7
  max_backups: 3
  max_size_mb: 10
`)
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yaml")
	os.WriteFile(f, content, 0644)

	cfg, err := Load(f)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Auth.TokenExpiryHours != 4 {
		t.Errorf("expected expiry 4h, got %d", cfg.Auth.TokenExpiryHours)
	}
	if cfg.Logging.File != "./test.log" {
		t.Errorf("expected logging.file=./test.log, got %q", cfg.Logging.File)
	}
	if cfg.Logging.MaxAgeDays != 7 {
		t.Errorf("expected logging.max_age_days=7, got %d", cfg.Logging.MaxAgeDays)
	}
	if cfg.Logging.MaxBackups != 3 {
		t.Errorf("expected logging.max_backups=3, got %d", cfg.Logging.MaxBackups)
	}
	if cfg.Logging.MaxSizeMB != 10 {
		t.Errorf("expected logging.max_size_mb=10, got %d", cfg.Logging.MaxSizeMB)
	}
}

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Database.Path != "./data/timetracking.db" {
		t.Errorf("expected default db path, got %s", cfg.Database.Path)
	}
	if cfg.Logging.File != "./data/server.log" {
		t.Errorf("expected default logging.file=./data/server.log, got %q", cfg.Logging.File)
	}
	if cfg.Logging.MaxAgeDays != 14 {
		t.Errorf("expected default logging.max_age_days=14, got %d", cfg.Logging.MaxAgeDays)
	}
	if cfg.Logging.MaxSizeMB != 20 {
		t.Errorf("expected default logging.max_size_mb=20, got %d", cfg.Logging.MaxSizeMB)
	}
	if cfg.Logging.MaxBackups != 0 {
		t.Errorf("expected default logging.max_backups=0, got %d", cfg.Logging.MaxBackups)
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv("NFC_SERVER_PORT", "3000")
	cfg := Defaults()
	cfg.ApplyEnv()
	if cfg.Server.Port != 3000 {
		t.Errorf("expected port 3000 from env, got %d", cfg.Server.Port)
	}
}

func TestBackupTargetPathEnvOverride(t *testing.T) {
	t.Setenv("NFC_BACKUP_TARGET_PATH", "/backup")
	cfg := Defaults()
	cfg.ApplyEnv()
	if cfg.BackupTargetPath != "/backup" {
		t.Errorf("expected BackupTargetPath=/backup, got %q", cfg.BackupTargetPath)
	}
}

func TestLoggingEnvOverride(t *testing.T) {
	t.Setenv("NFC_LOGGING_FILE", "/var/log/nfc.log")
	t.Setenv("NFC_LOGGING_MAX_AGE_DAYS", "30")
	cfg := Defaults()
	cfg.ApplyEnv()
	if cfg.Logging.File != "/var/log/nfc.log" {
		t.Errorf("expected logging.file from env, got %q", cfg.Logging.File)
	}
	if cfg.Logging.MaxAgeDays != 30 {
		t.Errorf("expected logging.max_age_days=30 from env, got %d", cfg.Logging.MaxAgeDays)
	}
}
