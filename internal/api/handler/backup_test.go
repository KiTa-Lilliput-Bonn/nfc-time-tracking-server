package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"nfc-time-tracking-server/internal/folderpick"
	"nfc-time-tracking-server/internal/service/backup"
	"nfc-time-tracking-server/internal/store/sqlite"
)

func TestBackupHandler_GetStatus(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "t.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	settings := sqlite.NewSettingsStore(db)
	_ = settings.Set(ctx, backup.SettingEnabled, "true")
	_ = settings.Set(ctx, backup.SettingIntervalMinutes, "30")
	svc := &backup.Service{Settings: settings, DB: db, DatabasePath: dbPath}
	h := &BackupHandler{Backup: svc}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backup/status", nil)
	rr := httptest.NewRecorder()
	h.GetStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var out backup.Status
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.Enabled || out.IntervalMinutes != 30 {
		t.Fatalf("%+v", out)
	}
}

func TestBackupHandler_GetBrowse(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "pick")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "t.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := &backup.Service{Settings: sqlite.NewSettingsStore(db), DB: db, DatabasePath: dbPath}
	h := &BackupHandler{Backup: svc}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backup/browse?path="+sub, nil)
	rr := httptest.NewRecorder()
	h.GetBrowse(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	var out backup.BrowseResult
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Path != sub {
		t.Fatalf("path %q", out.Path)
	}
}

func TestBackupHandler_PostPickFolder_unavailable(t *testing.T) {
	if folderpick.Available() {
		t.Skip("native folder picker available on this host")
	}
	h := &BackupHandler{Backup: &backup.Service{}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backup/pick-folder", nil)
	rr := httptest.NewRecorder()
	h.PostPickFolder(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestBackupHandler_PostPickFolder_nilService(t *testing.T) {
	h := &BackupHandler{Backup: nil}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backup/pick-folder", nil)
	rr := httptest.NewRecorder()
	h.PostPickFolder(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestBackupHandler_GetBrowse_nilService(t *testing.T) {
	h := &BackupHandler{Backup: nil}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backup/browse", nil)
	rr := httptest.NewRecorder()
	h.GetBrowse(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestBackupHandler_GetStatus_nilService(t *testing.T) {
	h := &BackupHandler{Backup: nil}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backup/status", nil)
	rr := httptest.NewRecorder()
	h.GetStatus(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", rr.Code)
	}
}
