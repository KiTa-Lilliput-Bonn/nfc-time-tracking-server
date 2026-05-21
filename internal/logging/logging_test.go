package logging

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nfc-time-tracking-server/internal/config"

	"gopkg.in/natefinch/lumberjack.v2"
)

func TestSetupWritesToFile(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "server.log")

	closer, err := Setup(config.LoggingConfig{
		File:       logFile,
		MaxAgeDays: 7,
		MaxBackups: 3,
		MaxSizeMB:  20,
	})
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer closer.Close()

	log.Printf("hello-from-test")

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "hello-from-test") {
		t.Errorf("expected log file to contain test message, got: %q", got)
	}
	if !strings.Contains(got, "logging: file=") {
		t.Errorf("expected log file to contain initial logging banner, got: %q", got)
	}
}

func TestSetupEmptyFileIsNoop(t *testing.T) {
	closer, err := Setup(config.LoggingConfig{File: ""})
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	if closer == nil {
		t.Fatalf("expected non-nil closer for empty file config")
	}
	if err := closer.Close(); err != nil {
		t.Errorf("noop closer should not error, got: %v", err)
	}
}

func TestRotateCreatesBackupFile(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "server.log")

	closer, err := Setup(config.LoggingConfig{
		File:       logFile,
		MaxAgeDays: 7,
		MaxBackups: 3,
		MaxSizeMB:  20,
	})
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer closer.Close()

	log.Printf("before-rotate")

	fc, ok := closer.(*fileCloser)
	if !ok {
		t.Fatalf("expected *fileCloser, got %T", closer)
	}
	if err := fc.lj.Rotate(); err != nil {
		t.Fatalf("manual Rotate failed: %v", err)
	}

	log.Printf("after-rotate")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var hasActive, hasBackup bool
	for _, e := range entries {
		name := e.Name()
		switch {
		case name == "server.log":
			hasActive = true
		case strings.HasPrefix(name, "server-") && strings.HasSuffix(name, ".log"):
			hasBackup = true
		}
	}
	if !hasActive {
		t.Errorf("expected active server.log to exist after rotate")
	}
	if !hasBackup {
		t.Errorf("expected at least one backup file with prefix 'server-' and suffix '.log', got entries: %v", entries)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read active log: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "after-rotate") {
		t.Errorf("expected new active log to contain 'after-rotate', got: %q", got)
	}
	if strings.Contains(got, "before-rotate") {
		t.Errorf("active log should not contain pre-rotate message, got: %q", got)
	}
}

func TestSetupStripsANSIFromFile(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "server.log")

	closer, err := Setup(config.LoggingConfig{
		File:       logFile,
		MaxAgeDays: 7,
		MaxBackups: 3,
		MaxSizeMB:  20,
	})
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer closer.Close()

	log.Printf("plain \x1b[32mgreen\x1b[0m tail")

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	got := string(data)
	if strings.Contains(got, "\x1b[") {
		t.Errorf("log file should not contain ANSI escapes, got: %q", got)
	}
	if !strings.Contains(got, "plain green tail") {
		t.Errorf("expected stripped text in file, got: %q", got)
	}
}

func TestLocalCalendarDate(t *testing.T) {
	loc := time.FixedZone("test", 2*3600)
	ti := time.Date(2026, 7, 4, 15, 30, 45, 123456789, loc)
	got := localCalendarDate(ti)
	want := time.Date(2026, 7, 4, 0, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("localCalendarDate(%v) = %v, want %v", ti, got, want)
	}
}

func TestDailyRotatorTriggersRotateOnDayChange(t *testing.T) {
	oldPoll := rotatorPollInterval
	rotatorPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { rotatorPollInterval = oldPoll })

	dir := t.TempDir()
	logFile := filepath.Join(dir, "server.log")

	lj := &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    1,
		MaxBackups: 5,
		LocalTime:  true,
	}
	defer lj.Close()
	if _, err := lj.Write([]byte("seed\n")); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	loc := time.FixedZone("testrot", 0)
	var mu sync.Mutex
	cur := time.Date(2026, 5, 12, 10, 0, 0, 0, loc)
	nowFn := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return cur
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go runDailyRotator(lj, stop, &wg, nowFn)

	time.Sleep(15 * time.Millisecond)

	mu.Lock()
	cur = time.Date(2026, 5, 13, 8, 0, 0, 0, loc)
	mu.Unlock()

	time.Sleep(30 * time.Millisecond)

	close(stop)
	wg.Wait()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var backups int
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "server-") && strings.HasSuffix(name, ".log") {
			backups++
		}
	}
	if backups == 0 {
		t.Errorf("expected at least one backup file after calendar day change, got entries: %v", entries)
	}
}

func TestStartupRotatesWhenLogMtimeIsPreviousDay(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "server.log")
	if err := os.WriteFile(logFile, []byte("pre-restart-log\n"), 0o644); err != nil {
		t.Fatalf("write seed log: %v", err)
	}
	now := time.Now().In(time.Local)
	yesterday := now.AddDate(0, 0, -1)
	mtime := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 12, 0, 0, 0, time.Local)
	if err := os.Chtimes(logFile, mtime, mtime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	closer, err := Setup(config.LoggingConfig{
		File:       logFile,
		MaxAgeDays: 7,
		MaxBackups: 5,
		MaxSizeMB:  20,
	})
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer closer.Close()

	log.Printf("post-setup-line")

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read active log: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "post-setup-line") {
		t.Errorf("expected active log to contain post-setup-line, got: %q", got)
	}
	if strings.Contains(got, "pre-restart-log") {
		t.Errorf("active log should not contain pre-restart-log after startup rotate, got: %q", got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var backups int
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "server-") && strings.HasSuffix(name, ".log") {
			backups++
		}
	}
	if backups == 0 {
		t.Errorf("expected at least one backup after startup rotate, got entries: %v", entries)
	}
}
