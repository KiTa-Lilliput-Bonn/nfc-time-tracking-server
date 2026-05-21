package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	envTestMode         = "NFC_TEST_MODE"
	envTestAdminPass    = "NFC_TEST_ADMIN_PASSWORD"
	envDatabasePath     = "NFC_DATABASE_PATH"
	envServerHost       = "NFC_SERVER_HOST"
	minTestAdminPassLen = 8
)

// TestModeConfig holds validated E2E test-mode settings when active.
type TestModeConfig struct {
	AdminPassword string
}

// TestModeEnabled reports whether guarded E2E test mode is active.
func TestModeEnabled() bool {
	_, ok := ActiveTestMode()
	return ok
}

// ActiveTestMode returns E2E settings when test mode is active.
func ActiveTestMode() (TestModeConfig, bool) {
	if strings.TrimSpace(os.Getenv(envTestMode)) != "1" {
		return TestModeConfig{}, false
	}
	host := strings.TrimSpace(os.Getenv(envServerHost))
	if host != "127.0.0.1" {
		return TestModeConfig{}, false
	}
	dbPath := strings.TrimSpace(os.Getenv(envDatabasePath))
	if dbPath == "" {
		return TestModeConfig{}, false
	}
	if !databasePathUnderTempDir(dbPath) {
		return TestModeConfig{}, false
	}
	pw := os.Getenv(envTestAdminPass)
	if len(pw) < minTestAdminPassLen {
		return TestModeConfig{}, false
	}
	return TestModeConfig{AdminPassword: pw}, true
}

func resolvePathBestEffort(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	parent := filepath.Dir(p)
	if parent != p {
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			return filepath.Join(resolved, filepath.Base(p))
		}
	}
	return p
}

func databasePathUnderTempDir(dbPath string) bool {
	absDB, err := filepath.Abs(dbPath)
	if err != nil {
		return false
	}
	absDB = resolvePathBestEffort(absDB)
	tmpRoot, err := filepath.Abs(os.TempDir())
	if err != nil {
		return false
	}
	tmpRoot = resolvePathBestEffort(tmpRoot)
	rel, err := filepath.Rel(tmpRoot, absDB)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// TestModeHolidayYears returns calendar years to seed in test mode (deterministic, no ticker drift).
func TestModeHolidayYears() []int {
	return []int{2026}
}

// ValidateTestModeForStartup fails fast when NFC_TEST_MODE is set but guards do not pass.
func ValidateTestModeForStartup() error {
	if strings.TrimSpace(os.Getenv(envTestMode)) == "" {
		return nil
	}
	if strings.TrimSpace(os.Getenv(envTestMode)) != "1" {
		return fmt.Errorf("%s: must be \"1\" when set", envTestMode)
	}
	if _, ok := ActiveTestMode(); !ok {
		return fmt.Errorf(
			"%s requires %s under os.TempDir(), %s=127.0.0.1, and %s with at least %d characters",
			envTestMode, envDatabasePath, envServerHost, envTestAdminPass, minTestAdminPassLen,
		)
	}
	return nil
}
