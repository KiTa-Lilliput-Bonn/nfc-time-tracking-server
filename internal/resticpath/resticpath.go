package resticpath

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// BundledRestic returns the absolute path to the restic binary shipped next to this
// executable under tools/ (tools/restic.exe on Windows, tools/restic elsewhere). If the
// file is missing or os.Executable fails, it returns an empty string.
func BundledRestic() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	resolved := exe
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		resolved = r
	}
	resolved = filepath.Clean(resolved)
	return pathNextToExecutable(resolved)
}

// ResolveRestic returns the path to a restic binary: first the bundled copy next to this
// executable (see BundledRestic), otherwise the first "restic" found via exec.LookPath
// (e.g. system install on PATH). Returns "" if neither is usable.
func ResolveRestic() string {
	return resolveRestic(exec.LookPath)
}

func resolveRestic(look func(string) (string, error)) string {
	if b := BundledRestic(); b != "" {
		return b
	}
	p, err := look("restic")
	if err != nil || strings.TrimSpace(p) == "" {
		return ""
	}
	p = filepath.Clean(strings.TrimSpace(p))
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return ""
	}
	return p
}

func pathNextToExecutable(resolvedExe string) string {
	dir := filepath.Dir(resolvedExe)
	name := "restic"
	if runtime.GOOS == "windows" {
		name = "restic.exe"
	}
	p := filepath.Join(dir, "tools", name)
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return ""
	}
	return p
}
