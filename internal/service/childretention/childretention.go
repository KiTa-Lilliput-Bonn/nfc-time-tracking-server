// Package childretention löscht Daten der Anwesenheitsliste nach einstellbaren Fristen (Datenschutz):
// Kommen/Gehen nach Monaten (vorher anonym gezählt), Meldungen Wochen nach ihrem letzten Tag und
// abgemeldete Kinder samt allen Daten Monate nach der Abmeldung.
package childretention

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/store"
)

// Settings keys.
const (
	SettingTimesMonths    = "child_attendance_retention_months"
	SettingNoticeWeeks    = "child_notice_retention_weeks"
	SettingInactiveMonths = "child_inactive_retention_months"
)

// Config sind die Löschfristen.
type Config struct {
	// TimesMonths: Kommen/Gehen bleibt so viele Monate erhalten.
	TimesMonths int `json:"times_months"`
	// NoticeWeeks: Meldungen bleiben so viele Wochen nach ihrem letzten Tag erhalten.
	NoticeWeeks int `json:"notice_weeks"`
	// InactiveMonths: abgemeldete Kinder bleiben so viele Monate nach der Abmeldung erhalten.
	InactiveMonths int `json:"inactive_months"`
}

// Default: 3 Monate, 4 Wochen, 3 Monate.
var Default = Config{TimesMonths: 3, NoticeWeeks: 4, InactiveMonths: 3}

const (
	MaxMonths = 24
	MaxWeeks  = 52
)

// Validate prüft die Grenzen (mindestens 1, höchstens 24 Monate bzw. 52 Wochen).
func (c Config) Validate() error {
	if c.TimesMonths < 1 || c.TimesMonths > MaxMonths || c.InactiveMonths < 1 || c.InactiveMonths > MaxMonths {
		return fmt.Errorf("Monatsfristen müssen zwischen 1 und %d liegen.", MaxMonths)
	}
	if c.NoticeWeeks < 1 || c.NoticeWeeks > MaxWeeks {
		return fmt.Errorf("Die Frist für Meldungen muss zwischen 1 und %d Wochen liegen.", MaxWeeks)
	}
	return nil
}

func readInt(ctx context.Context, s store.SettingsStore, key string, def int) int {
	v, err := s.Get(ctx, key)
	if err != nil {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 {
		return def
	}
	return n
}

// ReadConfig liest die Fristen; fehlende oder ungültige Werte fallen auf Default zurück.
func ReadConfig(ctx context.Context, s store.SettingsStore) Config {
	return Config{
		TimesMonths:    readInt(ctx, s, SettingTimesMonths, Default.TimesMonths),
		NoticeWeeks:    readInt(ctx, s, SettingNoticeWeeks, Default.NoticeWeeks),
		InactiveMonths: readInt(ctx, s, SettingInactiveMonths, Default.InactiveMonths),
	}
}

// SaveConfig speichert gültige Fristen.
func SaveConfig(ctx context.Context, s store.SettingsStore, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	for k, v := range map[string]int{
		SettingTimesMonths: c.TimesMonths, SettingNoticeWeeks: c.NoticeWeeks, SettingInactiveMonths: c.InactiveMonths,
	} {
		if err := s.Set(ctx, k, strconv.Itoa(v)); err != nil {
			return err
		}
	}
	return nil
}

// TimesFrom ist der älteste Tag (YYYY-MM-DD), dessen Kommen/Gehen noch gespeichert wird.
func (c Config) TimesFrom(now time.Time) string {
	return now.AddDate(0, -c.TimesMonths, 0).Format("2006-01-02")
}

// NoticesFrom: Meldungen, deren letzter Tag davor liegt, werden gelöscht.
func (c Config) NoticesFrom(now time.Time) string {
	return now.AddDate(0, 0, -7*c.NoticeWeeks).Format("2006-01-02")
}

// Purge liefert die Löschgrenzen zum Zeitpunkt now (Ortszeit).
func (c Config) Purge(now time.Time) store.ChildPurge {
	return store.ChildPurge{
		TimesBefore:    c.TimesFrom(now),
		NoticesBefore:  c.NoticesFrom(now),
		InactiveBefore: now.AddDate(0, -c.InactiveMonths, 0).UTC(),
	}
}

// Service führt die Löschung aus (täglich aus main.go).
type Service struct {
	Settings   store.SettingsStore
	Attendance store.AttendanceStore
	Audit      *audit.Logger
	Now        func() time.Time
}

// Run löscht alles, dessen Frist abgelaufen ist, und protokolliert die Anzahl.
func (s *Service) Run(ctx context.Context) (store.ChildPurgeResult, error) {
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	now = now.In(time.Local)
	cfg := ReadConfig(ctx, s.Settings)
	p := cfg.Purge(now)
	res, err := s.Attendance.PurgeChildData(ctx, p)
	if err != nil {
		return res, err
	}
	if res.AttendanceDays+res.Notices+res.Children > 0 && s.Audit != nil {
		s.Audit.Log(ctx, audit.Entry{
			ActorRole: audit.RoleSystem, Action: audit.ActionDelete, EntityType: audit.EntityChildRetention,
			EntityID: now.Format("2006-01-02"),
			Summary: audit.JSONSummary(map[string]any{
				"attendance_days": res.AttendanceDays, "notices": res.Notices, "children": res.Children,
				"times_before": p.TimesBefore, "notices_before": p.NoticesBefore,
			}),
		})
	}
	return res, nil
}
