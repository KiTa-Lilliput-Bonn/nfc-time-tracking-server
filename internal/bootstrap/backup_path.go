package bootstrap

import (
	"context"
	"strings"

	"nfc-time-tracking-server/internal/service/backup"
	"nfc-time-tracking-server/internal/store"
)

// SeedBackupTargetPath sets backup_target_path from path when the setting is still empty.
// Does not enable scheduled backups or overwrite an existing path.
func SeedBackupTargetPath(ctx context.Context, s store.SettingsStore, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	existing, err := s.Get(ctx, backup.SettingTargetPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(existing) != "" {
		return nil
	}
	svc := &backup.Service{Settings: s}
	return svc.SaveConfig(ctx, false, backup.MinIntervalMinutes, false, path)
}
