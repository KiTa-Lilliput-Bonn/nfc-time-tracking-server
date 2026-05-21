package resticpath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPathNextToExecutable(t *testing.T) {
	tmp := t.TempDir()
	tools := filepath.Join(tmp, "tools")
	if err := os.MkdirAll(tools, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "restic"
	if runtime.GOOS == "windows" {
		name = "restic.exe"
	}
	bin := filepath.Join(tools, name)
	if err := os.WriteFile(bin, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	host := filepath.Join(tmp, "nfc-time-tracker-server")
	if runtime.GOOS == "windows" {
		host += ".exe"
	}
	if err := os.WriteFile(host, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	got := pathNextToExecutable(host)
	if got != bin {
		t.Fatalf("expected %q, got %q", bin, got)
	}

	tmpNoTools := t.TempDir()
	appOnly := filepath.Join(tmpNoTools, "nfc-time-tracker-server")
	if runtime.GOOS == "windows" {
		appOnly += ".exe"
	}
	if err := os.WriteFile(appOnly, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if pathNextToExecutable(appOnly) != "" {
		t.Fatal("expected empty path when tools/restic is absent")
	}
}

func TestResolveRestic_prefersBundledOverLookPath(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	exeDir := filepath.Dir(exe)
	toolsDir := filepath.Join(exeDir, "tools")
	name := "restic"
	if runtime.GOOS == "windows" {
		name = "restic.exe"
	}
	bundlePath := filepath.Join(toolsDir, name)
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundlePath, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(toolsDir) })

	lookCalls := 0
	look := func(string) (string, error) {
		lookCalls++
		return "", errors.New("lookPath should not run when bundle exists")
	}

	got := resolveRestic(look)
	if got != bundlePath {
		t.Fatalf("expected bundled %q, got %q", bundlePath, got)
	}
	if lookCalls != 0 {
		t.Fatalf("expected lookPath not called, got %d calls", lookCalls)
	}
}

func TestResolveRestic_fallsBackToLookPath(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	exeDir := filepath.Dir(exe)
	toolsDir := filepath.Join(exeDir, "tools")
	_ = os.RemoveAll(toolsDir)
	t.Cleanup(func() { _ = os.RemoveAll(toolsDir) })

	want := filepath.Join(t.TempDir(), "restic-from-path")
	if err := os.WriteFile(want, []byte{}, 0o755); err != nil {
		t.Fatal(err)
	}

	look := func(string) (string, error) {
		return want, nil
	}
	if got := resolveRestic(look); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveRestic_emptyWhenLookPathFails(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	_ = os.RemoveAll(filepath.Join(filepath.Dir(exe), "tools"))

	look := func(string) (string, error) {
		return "", fmt.Errorf("not found")
	}
	if resolveRestic(look) != "" {
		t.Fatal("expected empty string")
	}
}

func TestResolveRestic_emptyWhenLookPathReturnsDirectory(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	_ = os.RemoveAll(filepath.Join(filepath.Dir(exe), "tools"))

	dir := t.TempDir()
	look := func(string) (string, error) {
		return dir, nil
	}
	if resolveRestic(look) != "" {
		t.Fatal("expected empty when lookPath points to directory")
	}
}
