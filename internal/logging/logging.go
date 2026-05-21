// Package logging verdrahtet das stdlib-log mit einer rollierenden Logdatei
// (taegliche Rotation, konfigurierbare Aufbewahrung) und schreibt parallel
// weiterhin nach stderr.
package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"nfc-time-tracking-server/internal/config"

	"gopkg.in/natefinch/lumberjack.v2"
)

// rotatorPollInterval ist das Intervall fuer den Kalendertag-Check. In Tests
// kurz setzen (t.Cleanup zuruecksetzen), damit Rotation ohne Echtzeit-Wartezeit
// geprueft werden kann.
var rotatorPollInterval = time.Minute

// Setup verdrahtet das stdlib-log mit Datei-Output (rolliert taeglich) und
// stderr. Wenn cfg.File leer ist, bleibt der Output unveraendert (stderr) und
// es wird ein No-Op-Closer zurueckgegeben.
//
// Der zurueckgegebene io.Closer sollte beim Shutdown geschlossen werden, um
// den Tages-Rotations-Scheduler zu stoppen und die Logdatei sauber zu schliessen.
func Setup(cfg config.LoggingConfig) (io.Closer, error) {
	if cfg.File == "" {
		return noopCloser{}, nil
	}
	if dir := filepath.Dir(cfg.File); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	lj := &lumberjack.Logger{
		Filename:   cfg.File,
		MaxAge:     cfg.MaxAgeDays,
		MaxBackups: cfg.MaxBackups,
		MaxSize:    cfg.MaxSizeMB,
		LocalTime:  true,
		Compress:   false,
	}
	if err := rotateIfLogFromPreviousCalendarDay(cfg.File, lj); err != nil {
		return nil, err
	}
	fileW := newLineAnsiStripWriter(lj)
	log.SetOutput(io.MultiWriter(os.Stderr, fileW))

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go runDailyRotator(lj, stop, &wg, time.Now)

	log.Printf("logging: file=%s max_age_days=%d max_backups=%d max_size_mb=%d",
		cfg.File, cfg.MaxAgeDays, cfg.MaxBackups, cfg.MaxSizeMB)

	return &fileCloser{lj: lj, fileW: fileW, stop: stop, wg: &wg}, nil
}

// rotateIfLogFromPreviousCalendarDay rotiert einmal, wenn die Logdatei
// existiert und ihr mtime-Kalendertag (lokal) vor dem heutigen Kalendertag
// liegt. So werden Logs nach einem Neustart ueber Mitternacht nicht an die
// Datei des Vortags angehaengt. Rotate-Fehler blockieren den Start nicht
// (Hinweis nach stderr).
func rotateIfLogFromPreviousCalendarDay(logPath string, lj *lumberjack.Logger) error {
	info, err := os.Stat(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	fileDay := localCalendarDate(info.ModTime().In(time.Local))
	today := localCalendarDate(time.Now())
	if !fileDay.Before(today) {
		return nil
	}
	if err := lj.Rotate(); err != nil {
		fmt.Fprintf(os.Stderr, "logging: startup rotate failed (log mtime on earlier calendar day): %v\n", err)
	}
	return nil
}

// localCalendarDate liefert 00:00:00 des lokalen Kalendertags von t (in
// t.Location()).
func localCalendarDate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// runDailyRotator prueft regelmaessig den lokalen Kalendertag und rotiert bei
// Wechsel. So bleibt die Tages-Rotation auch nach Standby/Resume zuverlaessig,
// ohne von einem einzelnen langen Timer bis Mitternacht abzuhaengen.
// nowFn ist injizierbar fuer Tests.
func runDailyRotator(lj *lumberjack.Logger, stop <-chan struct{}, wg *sync.WaitGroup, nowFn func() time.Time) {
	defer wg.Done()
	lastDay := localCalendarDate(nowFn())
	ticker := time.NewTicker(rotatorPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			day := localCalendarDate(nowFn())
			if day.Equal(lastDay) {
				continue
			}
			if err := lj.Rotate(); err != nil {
				log.Printf("logging: rotate failed: %v", err)
				continue
			}
			lastDay = day
		}
	}
}

type fileCloser struct {
	lj     *lumberjack.Logger
	fileW  *lineAnsiStripWriter
	stop   chan struct{}
	wg     *sync.WaitGroup
	once   sync.Once
}

func (c *fileCloser) Close() error {
	var err error
	c.once.Do(func() {
		close(c.stop)
		c.wg.Wait()
		log.SetOutput(os.Stderr)
		if flushErr := c.fileW.Flush(); flushErr != nil && err == nil {
			err = flushErr
		}
		if closeErr := c.lj.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	})
	return err
}

type noopCloser struct{}

func (noopCloser) Close() error { return nil }
