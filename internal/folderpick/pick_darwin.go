//go:build darwin

package folderpick

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func platformAvailable() bool {
	_, err := exec.LookPath("osascript")
	return err == nil
}

func platformPick(startDir string) (string, error) {
	var script string
	if startDir != "" {
		// AppleScript string literal for POSIX path.
		script = fmt.Sprintf(
			`POSIX path of (choose folder with prompt %q default location POSIX file %s)`,
			pickTitle,
			appleScriptQuote(startDir),
		)
	} else {
		script = fmt.Sprintf(`POSIX path of (choose folder with prompt %q)`, pickTitle)
	}
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if isOsascriptCancel(err, out) {
			return "", ErrCancelled
		}
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(msg)
	}
	return strings.TrimSpace(string(out)), nil
}

func appleScriptQuote(posixPath string) string {
	// AppleScript double-quoted string; escape backslashes and quotes.
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range posixPath {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func isOsascriptCancel(err error, out []byte) bool {
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		return false
	}
	s := strings.ToLower(string(out))
	return strings.Contains(s, "user canceled") || strings.Contains(s, "user cancelled") ||
		bytes.Contains(bytes.ToLower(out), []byte("-128"))
}
