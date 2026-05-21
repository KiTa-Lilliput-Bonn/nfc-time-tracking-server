package backup

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestSaveConfig_requiresAbsolutePath(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := sqlite.NewSettingsStore(db)
	s := &Service{Settings: st, DB: db, DatabasePath: "/tmp/x.db"}
	if err := s.SaveConfig(ctx, true, 60, false, "relative/path"); err == nil {
		t.Fatal("expected error for non-absolute path")
	}
}

func TestShouldRunScheduled_noLastSuccess(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "a.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := sqlite.NewSettingsStore(db)
	_ = st.Set(ctx, SettingEnabled, "true")
	_ = st.Set(ctx, SettingIntervalMinutes, "60")
	s := &Service{Settings: st, DB: db, DatabasePath: dbPath}
	if !s.shouldRunScheduled(ctx) {
		t.Fatal("expected due when no last success")
	}
}

func TestShouldRunScheduled_respectsInterval(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "b.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := sqlite.NewSettingsStore(db)
	_ = st.Set(ctx, SettingEnabled, "true")
	_ = st.Set(ctx, SettingIntervalMinutes, "60")
	_ = st.Set(ctx, SettingLastSuccessUTC, "2099-01-01T00:00:00Z")
	s := &Service{Settings: st, DB: db, DatabasePath: dbPath}
	if s.shouldRunScheduled(ctx) {
		t.Fatal("expected not due in far future")
	}
}

func TestBrowseDirectoriesAt_listsOnlyDirs(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "alpha")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := BrowseDirectoriesAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 1 || res.Entries[0].Name != "alpha" {
		t.Fatalf("entries: %+v", res.Entries)
	}
	if res.Path != root {
		t.Fatalf("path %q", res.Path)
	}
}

func TestBrowseDirectoriesAt_missingDir(t *testing.T) {
	_, err := BrowseDirectoriesAt(filepath.Join(t.TempDir(), "nope"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBrowseDirectories_rejectsRelative(t *testing.T) {
	ctx := context.Background()
	s := &Service{DatabasePath: "/tmp/x.db"}
	_, err := s.BrowseDirectories(ctx, "relative/path")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBrowseDirectories_defaultFromTargetPath(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	sub := filepath.Join(dir, "backups")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := sqlite.NewSettingsStore(db)
	_ = st.Set(ctx, SettingTargetPath, sub)
	s := &Service{Settings: st, DB: db, DatabasePath: filepath.Join(dir, "db.sqlite")}
	res, err := s.BrowseDirectories(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != sub {
		t.Fatalf("path %q want %q", res.Path, sub)
	}
}

func TestBrowseRoots_unix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix roots")
	}
	res, err := BrowseRoots()
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "/" {
		t.Fatalf("path %q", res.Path)
	}
}
