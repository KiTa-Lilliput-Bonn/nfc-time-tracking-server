package shiftalerts

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"nfc-time-tracking-server/internal/store"
)

const (
	SettingMaxHours = "shift_alert_max_hours"
	SettingLateEnd  = "shift_alert_late_end"
)

// Config holds thresholds for shift alert detection.
type Config struct {
	MaxHours float64 `json:"max_hours"`
	LateEnd  string  `json:"late_end_time"`
}

// LoadConfig reads shift alert settings with defaults (11h, 21:00).
func LoadConfig(ctx context.Context, settings store.SettingsStore) (Config, error) {
	cfg := Config{MaxHours: 11, LateEnd: "21:00"}
	if settings == nil {
		return cfg, nil
	}
	if v, err := settings.Get(ctx, SettingMaxHours); err != nil {
		return Config{}, err
	} else if strings.TrimSpace(v) != "" {
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s", SettingMaxHours)
		}
		cfg.MaxHours = f
	}
	if v, err := settings.Get(ctx, SettingLateEnd); err != nil {
		return Config{}, err
	} else {
		cfg.LateEnd = strings.TrimSpace(v)
	}
	return cfg, nil
}

// ValidateConfig checks PUT body values.
func ValidateConfig(cfg Config) error {
	if cfg.MaxHours < 0 || cfg.MaxHours > 24 {
		return fmt.Errorf("max_hours must be between 0 and 24")
	}
	if cfg.LateEnd != "" && !validHHMM(cfg.LateEnd) {
		return fmt.Errorf("late_end_time must be empty or HH:MM")
	}
	return nil
}

func validHHMM(s string) bool {
	if len(s) != 5 || s[2] != ':' {
		return false
	}
	h, err1 := strconv.Atoi(s[:2])
	m, err2 := strconv.Atoi(s[3:])
	if err1 != nil || err2 != nil {
		return false
	}
	return h >= 0 && h <= 23 && m >= 0 && m <= 59
}

// SaveConfig persists shift alert settings.
func SaveConfig(ctx context.Context, settings store.SettingsStore, cfg Config) error {
	if err := ValidateConfig(cfg); err != nil {
		return err
	}
	if err := settings.Set(ctx, SettingMaxHours, strconv.FormatFloat(cfg.MaxHours, 'f', -1, 64)); err != nil {
		return err
	}
	return settings.Set(ctx, SettingLateEnd, cfg.LateEnd)
}
