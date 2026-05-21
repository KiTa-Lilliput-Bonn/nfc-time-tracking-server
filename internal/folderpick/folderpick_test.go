package folderpick

import (
	"errors"
	"testing"
)

func TestAvailable(t *testing.T) {
	_ = Available()
}

func TestPick_unavailableOnUnsupportedOS(t *testing.T) {
	if Available() {
		t.Skip("native picker available on this host")
	}
	_, err := Pick("/tmp")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestNormalizeStartDir(t *testing.T) {
	dir := t.TempDir()
	got := normalizeStartDir(dir)
	if got != dir {
		t.Fatalf("got %q want %q", got, dir)
	}
}
