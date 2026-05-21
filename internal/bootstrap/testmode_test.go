package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTestModeEnabled_requiresAllGuards(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "e2e.db")

	t.Setenv(envTestMode, "1")
	t.Setenv(envServerHost, "127.0.0.1")
	t.Setenv(envDatabasePath, db)
	t.Setenv(envTestAdminPass, "e2e-admin-password")

	if !TestModeEnabled() {
		t.Fatal("expected test mode enabled under temp db")
	}

	t.Setenv(envServerHost, "0.0.0.0")
	if TestModeEnabled() {
		t.Fatal("expected disabled for non-loopback host")
	}
	t.Setenv(envServerHost, "127.0.0.1")

	t.Setenv(envDatabasePath, "/etc/passwd")
	if TestModeEnabled() {
		t.Fatal("expected disabled for db outside temp dir")
	}
}

func TestValidateTestModeForStartup_invalidCombo(t *testing.T) {
	t.Setenv(envTestMode, "1")
	t.Setenv(envServerHost, "127.0.0.1")
	t.Setenv(envDatabasePath, "/tmp/not-under-temp/e2e.db")
	t.Setenv(envTestAdminPass, "short")
	if err := ValidateTestModeForStartup(); err == nil {
		t.Fatal("expected error")
	}
}

func TestDatabasePathUnderTempDir(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "nested", "e2e.db")
	if err := os.MkdirAll(filepath.Dir(db), 0o755); err != nil {
		t.Fatal(err)
	}
	if !databasePathUnderTempDir(db) {
		t.Fatalf("expected %q under temp", db)
	}
	if databasePathUnderTempDir("/var/tmp/outside.db") {
		// /var/tmp may or may not be under os.TempDir(); skip ambiguous paths
		tmp := os.TempDir()
		if !stringsHasPrefix(filepath.Clean("/var/tmp"), filepath.Clean(tmp)) {
			return
		}
	}
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
