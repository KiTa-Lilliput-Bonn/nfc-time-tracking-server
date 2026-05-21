package bootstrap

import (
	"context"
	"strconv"
	"strings"

	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/backup"
	"nfc-time-tracking-server/internal/store"
)

// MaskedSecretPlaceholder masks stored secrets in settings API list responses.
const MaskedSecretPlaceholder = "********"

// StampsPollIntervalSeconds returns stamps_poll_interval_seconds (0 = off, default 300 when unset).
func StampsPollIntervalSeconds(ctx context.Context, s store.SettingsStore) int {
	sec := 300
	v, err := s.Get(ctx, "stamps_poll_interval_seconds")
	if err != nil {
		return sec
	}
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
		return n
	}
	return sec
}

// MaskSensitiveSettings masks secret values and drops internal-only keys from API list responses.
func MaskSensitiveSettings(list []model.Setting) []model.Setting {
	out := make([]model.Setting, 0, len(list))
	for _, row := range list {
		if row.Key == "stamps_poll_watermark_utc" || strings.HasPrefix(row.Key, "stamps_poll_watermark_utc_") {
			continue
		}
		v := row.Value
		if row.Key == "stamps_poll_bearer" && strings.TrimSpace(v) != "" {
			v = MaskedSecretPlaceholder
		}
		if row.Key == backup.SettingResticPassword && strings.TrimSpace(v) != "" {
			v = MaskedSecretPlaceholder
		}
		out = append(out, model.Setting{Key: row.Key, Value: v})
	}
	return out
}
