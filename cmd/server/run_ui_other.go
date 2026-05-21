//go:build !windows

package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nfc-time-tracking-server/internal/config"
)

func runPlatformUI(srv *http.Server, _ *config.Config) {
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		close(errCh)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("server: received %v, shutting down...", sig)
		shutdownHTTPServer(srv, 10*time.Second)
	case err, ok := <-errCh:
		if ok && err != nil {
			log.Fatalf("server: %v", err)
		}
	}
}
