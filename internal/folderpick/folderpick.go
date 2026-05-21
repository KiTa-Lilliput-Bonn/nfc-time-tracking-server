// Package folderpick opens the OS-native folder selection dialog on the server machine.
package folderpick

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrUnavailable is returned when no native folder picker can be shown.
	ErrUnavailable = errors.New("native folder picker unavailable")
	// ErrCancelled is returned when the user dismisses the dialog without choosing.
	ErrCancelled = errors.New("folder picker cancelled")
)

const pickTitle = "Backup-Zielordner wählen"

// Available reports whether Pick can show a native folder dialog on this host.
func Available() bool {
	return platformAvailable()
}

// Pick shows the system folder picker. startDir may be empty or an absolute directory path.
// Returns cleaned absolute path, ErrCancelled, or ErrUnavailable.
func Pick(startDir string) (string, error) {
	if !Available() {
		return "", ErrUnavailable
	}
	startDir = normalizeStartDir(startDir)
	chosen, err := platformPick(startDir)
	if err != nil {
		return "", err
	}
	chosen = strings.TrimSpace(chosen)
	if chosen == "" {
		return "", ErrCancelled
	}
	abs, err := filepath.Abs(chosen)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(abs) {
		return "", errors.New("chosen path is not absolute")
	}
	return filepath.Clean(abs), nil
}

func normalizeStartDir(startDir string) string {
	startDir = strings.TrimSpace(startDir)
	if startDir == "" {
		return ""
	}
	if cleaned, err := filepath.Abs(filepath.Clean(startDir)); err == nil {
		if st, err := os.Stat(cleaned); err == nil && st.IsDir() {
			return cleaned
		}
		if st, err := os.Stat(filepath.Dir(cleaned)); err == nil && st.IsDir() {
			return filepath.Dir(cleaned)
		}
	}
	return ""
}
