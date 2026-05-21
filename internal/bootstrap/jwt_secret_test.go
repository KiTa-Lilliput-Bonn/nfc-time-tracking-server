package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveJWTSecret_ExplicitWins(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "app.db")
	got, err := ResolveJWTSecret("fixed-secret", db)
	if err != nil {
		t.Fatal(err)
	}
	if got != "fixed-secret" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveJWTSecret_LegacyPlaceholderUsesFile(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "app.db")
	if err := os.MkdirAll(filepath.Dir(db), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveJWTSecret("auto-generated-on-first-start", db)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 32 {
		t.Fatalf("expected generated secret, got len %d", len(got))
	}
}

func TestResolveJWTSecret_PersistsFile(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "app.db")
	if err := os.MkdirAll(filepath.Dir(db), 0o755); err != nil {
		t.Fatal(err)
	}
	a, err := ResolveJWTSecret("", db)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) < 32 {
		t.Fatalf("unexpected secret length %d", len(a))
	}
	b, err := ResolveJWTSecret("", db)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("second call should read same secret from file")
	}
}
