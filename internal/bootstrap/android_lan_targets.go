package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"nfc-time-tracking-server/internal/store"
)

// LanBaseURL returns http://host:port for this target.
func (t AndroidLanTarget) LanBaseURL() string {
	return fmt.Sprintf("http://%s:%d", strings.TrimSpace(t.Host), t.Port)
}

// MaxAndroidLanTargets limits how many LAN devices can be configured.
const MaxAndroidLanTargets = 20

// AndroidLanTarget is one Android app LAN endpoint (stamps poll + employee sync).
type AndroidLanTarget struct {
	ID           string `json:"id"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	APIClientID  string `json:"api_client_id"`
	Label        string `json:"label,omitempty"`
}

type androidLanTargetWire struct {
	ID          string `json:"id"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	APIClientID string `json:"api_client_id"`
	Label       string `json:"label"`
}

// ParseAndroidLanTargetsJSON parses and validates the android_lan_targets JSON payload.
func ParseAndroidLanTargetsJSON(raw string) ([]AndroidLanTarget, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, nil
	}
	var wires []androidLanTargetWire
	if err := json.Unmarshal([]byte(raw), &wires); err != nil {
		return nil, fmt.Errorf("android_lan_targets: invalid JSON: %w", err)
	}
	if len(wires) > MaxAndroidLanTargets {
		return nil, fmt.Errorf("android_lan_targets: at most %d entries", MaxAndroidLanTargets)
	}
	out := make([]AndroidLanTarget, 0, len(wires))
	for i, w := range wires {
		id := strings.TrimSpace(w.ID)
		host := strings.TrimSpace(w.Host)
		apiID := strings.TrimSpace(w.APIClientID)
		if id == "" {
			return nil, fmt.Errorf("android_lan_targets[%d]: id required", i)
		}
		if host == "" {
			return nil, fmt.Errorf("android_lan_targets[%d]: host required", i)
		}
		if apiID == "" {
			return nil, fmt.Errorf("android_lan_targets[%d]: api_client_id required", i)
		}
		if w.Port < 1 || w.Port > 65535 {
			return nil, fmt.Errorf("android_lan_targets[%d]: port out of range", i)
		}
		out = append(out, AndroidLanTarget{
			ID:          id,
			Host:        host,
			Port:        w.Port,
			APIClientID: apiID,
			Label:       strings.TrimSpace(w.Label),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Host != out[j].Host {
			return out[i].Host < out[j].Host
		}
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// LanTargetsFromSettings reads and parses android_lan_targets.
func LanTargetsFromSettings(ctx context.Context, s store.SettingsStore) ([]AndroidLanTarget, error) {
	raw, err := s.Get(ctx, "android_lan_targets")
	if err != nil {
		return nil, err
	}
	return ParseAndroidLanTargetsJSON(raw)
}

// MarshalAndroidLanTargetsJSON serializes targets for the settings store.
func MarshalAndroidLanTargetsJSON(targets []AndroidLanTarget) (string, error) {
	if len(targets) == 0 {
		return "[]", nil
	}
	wires := make([]androidLanTargetWire, 0, len(targets))
	for _, t := range targets {
		wires = append(wires, androidLanTargetWire{
			ID:          t.ID,
			Host:        t.Host,
			Port:        t.Port,
			APIClientID: t.APIClientID,
			Label:       t.Label,
		})
	}
	b, err := json.Marshal(wires)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// UpsertAndroidLanTarget replaces an entry with the same id or api_client_id, or appends.
func UpsertAndroidLanTarget(targets []AndroidLanTarget, entry AndroidLanTarget) ([]AndroidLanTarget, error) {
	entry.ID = strings.TrimSpace(entry.ID)
	entry.Host = strings.TrimSpace(entry.Host)
	entry.APIClientID = strings.TrimSpace(entry.APIClientID)
	entry.Label = strings.TrimSpace(entry.Label)
	if entry.ID == "" || entry.Host == "" || entry.APIClientID == "" {
		return nil, fmt.Errorf("android_lan_targets: id, host, and api_client_id required")
	}
	if entry.Port < 1 || entry.Port > 65535 {
		return nil, fmt.Errorf("android_lan_targets: port out of range")
	}

	out := make([]AndroidLanTarget, 0, len(targets)+1)
	replaced := false
	for _, t := range targets {
		if t.ID == entry.ID || t.APIClientID == entry.APIClientID {
			out = append(out, entry)
			replaced = true
			continue
		}
		out = append(out, t)
	}
	if !replaced {
		if len(out) >= MaxAndroidLanTargets {
			return nil, fmt.Errorf("android_lan_targets: at most %d entries", MaxAndroidLanTargets)
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Host != out[j].Host {
			return out[i].Host < out[j].Host
		}
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
