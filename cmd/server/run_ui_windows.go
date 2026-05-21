//go:build windows

package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/config"

	"github.com/getlantern/systray"
)

//go:embed tray.ico
var trayIcon []byte

func runPlatformUI(srv *http.Server, cfg *config.Config) {
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("server: %v", err)
		}
	}()

	systray.Run(func() { trayOnReady(srv, cfg) }, func() {})
}

func trayOnReady(srv *http.Server, cfg *config.Config) {
	systray.SetIcon(trayIcon)
	systray.SetTooltip("NFC Time Tracking Server")

	mBrowser := systray.AddMenuItem("Im Browser öffnen", "Web-Oberfläche im Browser öffnen")
	mFolder := systray.AddMenuItem("Ordner öffnen", "Ordner dieser Programmdatei öffnen")
	mQuit := systray.AddMenuItem("Beenden", "Server beenden")

	go func() {
		for {
			select {
			case <-mBrowser.ClickedCh:
				url := browserBaseURL(cfg)
				if err := exec.Command("cmd", "/c", "start", "", url).Start(); err != nil {
					log.Printf("tray: open browser: %v", err)
				}
			case <-mFolder.ClickedCh:
				dir := exeDirOrFallback()
				if err := exec.Command("cmd", "/c", "start", "", dir).Start(); err != nil {
					log.Printf("tray: open folder: %v", err)
				}
			case <-mQuit.ClickedCh:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := srv.Shutdown(ctx); err != nil {
					log.Printf("tray: server shutdown: %v", err)
				}
				cancel()
				systray.Quit()
				return
			}
		}
	}()
}

func browserBaseURL(cfg *config.Config) string {
	h := strings.TrimSpace(cfg.Server.Host)
	if h == "" || h == "0.0.0.0" || h == "::" {
		return fmt.Sprintf("http://127.0.0.1:%d/", cfg.Server.Port)
	}
	hostport := net.JoinHostPort(h, strconv.Itoa(cfg.Server.Port))
	return "http://" + hostport + "/"
}

func exeDirOrFallback() string {
	exe, err := os.Executable()
	if err != nil {
		if wd, err2 := os.Getwd(); err2 == nil {
			return wd
		}
		return "."
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe)
}
