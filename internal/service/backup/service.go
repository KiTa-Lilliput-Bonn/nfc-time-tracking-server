package backup

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/folderpick"
	"nfc-time-tracking-server/internal/resticpath"
	"nfc-time-tracking-server/internal/store"
	"nfc-time-tracking-server/internal/store/sqlite"
)

// Settings keys (stored in SQLite settings table).
const (
	SettingEnabled           = "backup_enabled"
	SettingIntervalMinutes   = "backup_interval_minutes"
	SettingUseRestic         = "backup_use_restic"
	SettingTargetPath        = "backup_target_path"
	SettingResticPassword    = "backup_restic_password"
	SettingResticInitialized = "backup_restic_initialized"
	SettingLastSuccessUTC    = "backup_last_success_utc"
	SettingLastError         = "backup_last_error"
	SettingKeepDaily         = "backup_keep_daily"
	SettingKeepWeekly        = "backup_keep_weekly"
	SettingKeepMonthly       = "backup_keep_monthly"
	SettingLastPruneUTC      = "backup_last_prune_utc"
)

// Aufbewahrung alter Backups (Datenschutz: gelöschte Daten sollen nicht unbegrenzt in Backups bleiben).
// Standard: tägliche 14 Tage, wöchentliche 8 Wochen, monatliche 6 Monate; alle 0 = nichts löschen.
const (
	DefaultKeepDaily   = 14
	DefaultKeepWeekly  = 8
	DefaultKeepMonthly = 6
	MaxKeep            = 400
	// Backups der letzten zwei Tage bleiben immer vollständig erhalten.
	keepWithin = 48 * time.Hour
)

const MinIntervalMinutes = 15

// Service runs scheduled SQLite copies and optional restic backup.
type Service struct {
	Settings     store.SettingsStore
	DB           *sqlite.DB
	AuditStore   *sqlite.AuditStore
	DatabasePath string
	ResticPath   string // empty = resolve via resticpath.ResolveRestic() (bundle then PATH)

	mu sync.Mutex
}

func (s *Service) resticBin() string {
	if s.ResticPath != "" {
		return s.ResticPath
	}
	return resticpath.ResolveRestic()
}

func (s *Service) readBool(ctx context.Context, key string) bool {
	v, err := s.Settings.Get(ctx, key)
	if err != nil || strings.TrimSpace(v) == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (s *Service) readInt(ctx context.Context, key string, def int) int {
	v, err := s.Settings.Get(ctx, key)
	if err != nil {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

// Run executes one backup cycle if enabled (manual / forced). Safe for concurrent calls (serialized).
func (s *Service) Run(ctx context.Context) error {
	return s.runLocked(ctx, false)
}

// RunScheduled runs a backup only when enabled and the interval has elapsed (or no prior success).
func (s *Service) RunScheduled(ctx context.Context) error {
	return s.runLocked(ctx, true)
}

func (s *Service) runLocked(ctx context.Context, onlyIfDue bool) error {
	if !s.readBool(ctx, SettingEnabled) {
		return nil
	}
	if onlyIfDue && !s.shouldRunScheduled(ctx) {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Re-check after taking the lock (another Run may have just finished).
	if onlyIfDue && !s.shouldRunScheduled(ctx) {
		return nil
	}
	if !s.readBool(ctx, SettingEnabled) {
		return nil
	}

	target, err := s.Settings.Get(ctx, SettingTargetPath)
	if err != nil {
		return s.fail(ctx, fmt.Errorf("read target path: %w", err))
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return s.fail(ctx, fmt.Errorf("backup_target_path is empty"))
	}
	if !filepath.IsAbs(target) {
		return s.fail(ctx, fmt.Errorf("backup_target_path must be absolute"))
	}

	useRestic := s.readBool(ctx, SettingUseRestic)
	if useRestic {
		err = s.runResticPipeline(ctx, target)
	} else {
		err = s.runPlainVacuum(ctx, target)
	}
	if err != nil {
		return err
	}
	if perr := s.pruneIfDue(ctx, target, useRestic); perr != nil {
		log.Printf("backup prune: %v", perr)
		_ = s.Settings.Set(ctx, SettingLastError, "Alte Backups aufräumen: "+perr.Error())
	}
	return nil
}

// Keep ist die Aufbewahrungsregel für alte Backups.
type Keep struct {
	Daily   int `json:"keep_daily"`
	Weekly  int `json:"keep_weekly"`
	Monthly int `json:"keep_monthly"`
}

func (k Keep) none() bool { return k.Daily == 0 && k.Weekly == 0 && k.Monthly == 0 }

func (s *Service) readKeep(ctx context.Context) Keep {
	read := func(key string, def int) int {
		v, err := s.Settings.Get(ctx, key)
		if err != nil || strings.TrimSpace(v) == "" {
			return def
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 0 {
			return def
		}
		return n
	}
	return Keep{
		Daily:   read(SettingKeepDaily, DefaultKeepDaily),
		Weekly:  read(SettingKeepWeekly, DefaultKeepWeekly),
		Monthly: read(SettingKeepMonthly, DefaultKeepMonthly),
	}
}

// SaveKeep speichert die Aufbewahrungsregel (je 0 bis MaxKeep).
func (s *Service) SaveKeep(ctx context.Context, k Keep) error {
	for _, v := range []int{k.Daily, k.Weekly, k.Monthly} {
		if v < 0 || v > MaxKeep {
			return fmt.Errorf("keep values must be between 0 and %d", MaxKeep)
		}
	}
	for key, v := range map[string]int{SettingKeepDaily: k.Daily, SettingKeepWeekly: k.Weekly, SettingKeepMonthly: k.Monthly} {
		if err := s.Settings.Set(ctx, key, strconv.Itoa(v)); err != nil {
			return err
		}
	}
	return nil
}

// pruneIfDue räumt höchstens einmal am Tag alte Backups auf.
func (s *Service) pruneIfDue(ctx context.Context, target string, useRestic bool) error {
	keep := s.readKeep(ctx)
	if keep.none() {
		return nil
	}
	if last, _ := s.Settings.Get(ctx, SettingLastPruneUTC); strings.TrimSpace(last) != "" {
		if t, err := parseRFC3339Flexible(strings.TrimSpace(last)); err == nil && time.Since(t) < 23*time.Hour {
			return nil
		}
	}
	var err error
	if useRestic {
		err = s.pruneRestic(ctx, target, keep)
	} else {
		_, err = PrunePlainBackups(target, keep, time.Now())
	}
	if err != nil {
		return err
	}
	return s.Settings.Set(ctx, SettingLastPruneUTC, time.Now().UTC().Format(time.RFC3339Nano))
}

func (s *Service) pruneRestic(ctx context.Context, repoAbs string, keep Keep) error {
	pw, err := s.Settings.Get(ctx, SettingResticPassword)
	if err != nil {
		return err
	}
	bin := s.resticBin()
	if bin == "" {
		return fmt.Errorf("restic binary not found")
	}
	cmd := exec.CommandContext(ctx, bin, ResticForgetArgs(keep)...)
	cmd.Env = append(os.Environ(),
		"RESTIC_REPOSITORY="+repoAbs,
		"RESTIC_PASSWORD="+pw,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restic forget: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ResticForgetArgs baut den Aufruf für restic forget (nur Snapshots dieses Servers) samt prune.
func ResticForgetArgs(keep Keep) []string {
	args := []string{"forget", "--tag", "nfc-time-tracking", "--group-by", "tags", "--keep-within", "2d"}
	if keep.Daily > 0 {
		args = append(args, "--keep-daily", strconv.Itoa(keep.Daily))
	}
	if keep.Weekly > 0 {
		args = append(args, "--keep-weekly", strconv.Itoa(keep.Weekly))
	}
	if keep.Monthly > 0 {
		args = append(args, "--keep-monthly", strconv.Itoa(keep.Monthly))
	}
	return append(args, "--prune")
}

const plainBackupPrefix = "timetracking-backup-"

// PrunePlainBackups löscht einfache Datenbank-Kopien (timetracking-backup-<ns>.db) in dir, die nach der
// Regel nicht mehr gebraucht werden, und liefert die gelöschten Dateinamen. Andere Dateien bleiben unberührt.
func PrunePlainBackups(dir string, keep Keep, now time.Time) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	type backupFile struct {
		name string
		at   time.Time
	}
	var files []backupFile
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, plainBackupPrefix) || !strings.HasSuffix(name, ".db") {
			continue
		}
		ns, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(name, plainBackupPrefix), ".db"), 10, 64)
		if err != nil {
			continue
		}
		files = append(files, backupFile{name, time.Unix(0, ns)})
	}
	times := make([]time.Time, len(files))
	for i, f := range files {
		times[i] = f.at
	}
	keepIdx := SelectKeep(times, keep, now)
	var removed []string
	for i, f := range files {
		if keepIdx[i] {
			continue
		}
		if err := os.Remove(filepath.Join(dir, f.name)); err != nil {
			return removed, err
		}
		removed = append(removed, f.name)
	}
	return removed, nil
}

// SelectKeep wendet die Regel wie restic forget an: alles der letzten zwei Tage, dazu je das neueste
// Backup der letzten Daily Tage, Weekly Wochen und Monthly Monate (Ortszeit). Liefert die zu behaltenden Indizes.
func SelectKeep(times []time.Time, keep Keep, now time.Time) map[int]bool {
	idx := make([]int, len(times))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return times[idx[a]].After(times[idx[b]]) })
	out := map[int]bool{}
	if len(idx) > 0 {
		out[idx[0]] = true
	}
	for _, i := range idx {
		if now.Sub(times[i]) < keepWithin {
			out[i] = true
		}
	}
	bucket := func(n int, key func(t time.Time) string) {
		seen := map[string]bool{}
		for _, i := range idx {
			if len(seen) >= n {
				return
			}
			k := key(times[i].In(time.Local))
			if seen[k] {
				continue
			}
			seen[k] = true
			out[i] = true
		}
	}
	bucket(keep.Daily, func(t time.Time) string { return t.Format("2006-01-02") })
	bucket(keep.Weekly, func(t time.Time) string { y, w := t.ISOWeek(); return fmt.Sprintf("%d-%02d", y, w) })
	bucket(keep.Monthly, func(t time.Time) string { return t.Format("2006-01") })
	return out
}

func (s *Service) shouldRunScheduled(ctx context.Context) bool {
	interval := s.readInt(ctx, SettingIntervalMinutes, MinIntervalMinutes)
	if interval < MinIntervalMinutes {
		interval = MinIntervalMinutes
	}
	lastStr, err := s.Settings.Get(ctx, SettingLastSuccessUTC)
	if err != nil || strings.TrimSpace(lastStr) == "" {
		return true
	}
	last, err := parseRFC3339Flexible(strings.TrimSpace(lastStr))
	if err != nil {
		return true
	}
	return time.Now().UTC().After(last.Add(time.Duration(interval) * time.Minute))
}

func parseRFC3339Flexible(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

func (s *Service) fail(ctx context.Context, err error) error {
	_ = s.Settings.Set(ctx, SettingLastError, err.Error())
	return err
}

func (s *Service) succeed(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.Settings.Set(ctx, SettingLastSuccessUTC, now); err != nil {
		return err
	}
	return s.Settings.Set(ctx, SettingLastError, "")
}

func (s *Service) runPlainVacuum(ctx context.Context, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return s.fail(ctx, fmt.Errorf("mkdir backup target: %w", err))
	}
	dst := filepath.Join(outDir, fmt.Sprintf("timetracking-backup-%d.db", time.Now().UnixNano()))
	if err := sqlite.VacuumInto(ctx, s.DB.DB, dst); err != nil {
		_ = os.Remove(dst)
		return s.fail(ctx, fmt.Errorf("vacuum into: %w", err))
	}
	if err := s.succeed(ctx); err != nil {
		return err
	}
	s.writeAuditTip(ctx, outDir)
	return nil
}

func (s *Service) runResticPipeline(ctx context.Context, repoAbs string) error {
	pw, err := s.Settings.Get(ctx, SettingResticPassword)
	if err != nil {
		return s.fail(ctx, fmt.Errorf("read restic password: %w", err))
	}
	if strings.TrimSpace(pw) == "" {
		return s.fail(ctx, fmt.Errorf("restic password not set; call init-restic first"))
	}
	if !s.readBool(ctx, SettingResticInitialized) {
		return s.fail(ctx, fmt.Errorf("restic repository not initialized"))
	}

	bin := s.resticBin()
	if bin == "" {
		return s.fail(ctx, fmt.Errorf("restic binary not found (install restic on PATH or place it under tools/ next to the server executable)"))
	}

	dbDir := filepath.Dir(s.DatabasePath)
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return s.fail(ctx, fmt.Errorf("mkdir db dir: %w", err))
	}
	tmp := filepath.Join(dbDir, fmt.Sprintf(".nfc-vacuum-%d.db", time.Now().UnixNano()))
	defer os.Remove(tmp)

	if err := sqlite.VacuumInto(ctx, s.DB.DB, tmp); err != nil {
		_ = os.Remove(tmp)
		return s.fail(ctx, fmt.Errorf("vacuum into: %w", err))
	}

	cmd := exec.CommandContext(ctx, bin, "backup", "--tag", "nfc-time-tracking", tmp)
	cmd.Env = append(os.Environ(),
		"RESTIC_REPOSITORY="+repoAbs,
		"RESTIC_PASSWORD="+pw,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return s.fail(ctx, fmt.Errorf("restic backup: %w: %s", err, strings.TrimSpace(string(out))))
	}
	if err := s.succeed(ctx); err != nil {
		return err
	}
	s.writeAuditTip(ctx, repoAbs)
	return nil
}

func (s *Service) writeAuditTip(ctx context.Context, dir string) {
	if s.AuditStore == nil || strings.TrimSpace(dir) == "" {
		return
	}
	tip, err := s.AuditStore.Tip(ctx)
	if err != nil || tip == nil {
		return
	}
	if err := audit.WriteTipFile(dir, tip); err != nil {
		log.Printf("audit-tip write: %v", err)
	}
}

// InitResticRepo creates a new restic repository at repoAbs using a freshly generated password.
// The password is returned once; callers must persist it via settings (already done by handler).
func (s *Service) InitResticRepo(ctx context.Context, repoAbs string) (password string, err error) {
	bin := s.resticBin()
	if bin == "" {
		return "", fmt.Errorf("restic binary not found (install restic on PATH or place it under tools/ next to the server executable)")
	}
	if !filepath.IsAbs(repoAbs) {
		return "", fmt.Errorf("repository path must be absolute")
	}
	if err := os.MkdirAll(repoAbs, 0o700); err != nil {
		return "", fmt.Errorf("mkdir repo: %w", err)
	}
	cfg := filepath.Join(repoAbs, "config")
	if st, err := os.Stat(cfg); err == nil && !st.IsDir() {
		return "", fmt.Errorf("restic repository already exists at this path")
	}

	password, err = generateResticPassword()
	if err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, bin, "init")
	cmd.Env = append(os.Environ(),
		"RESTIC_REPOSITORY="+repoAbs,
		"RESTIC_PASSWORD="+password,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("restic init: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return password, nil
}

func generateResticPassword() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Status is returned by the admin backup API (no secrets).
type Status struct {
	Enabled             bool   `json:"enabled"`
	IntervalMinutes     int    `json:"interval_minutes"`
	UseRestic           bool   `json:"use_restic"`
	TargetPath          string `json:"target_path"`
	ResticInitialized   bool   `json:"restic_initialized"`
	HasResticPassword   bool   `json:"has_restic_password"`
	ResticBinaryPresent   bool `json:"restic_binary_present"`
	FolderPickerAvailable bool `json:"folder_picker_available"`
	LastSuccessUTC        string `json:"last_success_utc"`
	LastError             string `json:"last_error"`
	Keep
}

func (s *Service) ReadStatus(ctx context.Context) (Status, error) {
	var st Status
	st.Enabled = s.readBool(ctx, SettingEnabled)
	st.IntervalMinutes = s.readInt(ctx, SettingIntervalMinutes, MinIntervalMinutes)
	if st.IntervalMinutes < MinIntervalMinutes {
		st.IntervalMinutes = MinIntervalMinutes
	}
	st.UseRestic = s.readBool(ctx, SettingUseRestic)
	p, err := s.Settings.Get(ctx, SettingTargetPath)
	if err != nil {
		return st, err
	}
	st.TargetPath = strings.TrimSpace(p)
	st.ResticInitialized = s.readBool(ctx, SettingResticInitialized)
	pw, _ := s.Settings.Get(ctx, SettingResticPassword)
	st.HasResticPassword = strings.TrimSpace(pw) != ""
	st.ResticBinaryPresent = s.resticBin() != ""
	st.FolderPickerAvailable = folderpick.Available()
	ls, _ := s.Settings.Get(ctx, SettingLastSuccessUTC)
	st.LastSuccessUTC = strings.TrimSpace(ls)
	le, _ := s.Settings.Get(ctx, SettingLastError)
	st.LastError = strings.TrimSpace(le)
	st.Keep = s.readKeep(ctx)
	return st, nil
}

// SaveConfig validates and persists backup settings (no restic init).
func (s *Service) SaveConfig(ctx context.Context, enabled bool, intervalMinutes int, useRestic bool, targetPath string) error {
	targetPath = strings.TrimSpace(targetPath)
	if enabled {
		if targetPath == "" {
			return fmt.Errorf("target_path is required when backup is enabled")
		}
		if intervalMinutes < MinIntervalMinutes {
			intervalMinutes = MinIntervalMinutes
		}
	}
	if targetPath != "" && !filepath.IsAbs(targetPath) {
		return fmt.Errorf("target_path must be an absolute path")
	}
	if err := s.Settings.Set(ctx, SettingEnabled, boolString(enabled)); err != nil {
		return err
	}
	if err := s.Settings.Set(ctx, SettingIntervalMinutes, strconv.Itoa(intervalMinutes)); err != nil {
		return err
	}
	if err := s.Settings.Set(ctx, SettingUseRestic, boolString(useRestic)); err != nil {
		return err
	}
	if targetPath != "" {
		if err := s.Settings.Set(ctx, SettingTargetPath, targetPath); err != nil {
			return err
		}
	}
	return nil
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
