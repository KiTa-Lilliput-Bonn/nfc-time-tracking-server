//go:build linux

package folderpick

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func platformAvailable() bool {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return false
	}
	_, err := exec.LookPath("zenity")
	return err == nil
}

func platformPick(startDir string) (string, error) {
	args := []string{"--file-selection", "--directory", "--title=" + pickTitle}
	if startDir != "" {
		args = append(args, "--filename="+startDir+string(os.PathSeparator))
	}
	cmd := exec.Command("zenity", args...)
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return "", ErrCancelled
		}
		return "", fmt.Errorf("zenity: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
