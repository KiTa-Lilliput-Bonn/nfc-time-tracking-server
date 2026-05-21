//go:build dev

package web

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// FS serves files from web/dist for local development (go run -tags dev).
// The path is resolved from the current working directory by walking up until
// web/dist/index.html is found (so starting the server from cmd/ or web/ still works).
// Override with absolute path: NFC_WEB_DIST_DIR=/path/to/web/dist
func FS() (fs.FS, error) {
	dir, err := resolveWebDistDir()
	if err != nil {
		return nil, err
	}
	log.Printf("nfc-server: Web-UI (dev) wird aus %s ausgeliefert", dir)
	return os.DirFS(dir), nil
}

func resolveWebDistDir() (string, error) {
	if d := strings.TrimSpace(os.Getenv("NFC_WEB_DIST_DIR")); d != "" {
		abs, err := filepath.Abs(d)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(abs, "index.html")); err != nil {
			return "", fmt.Errorf("NFC_WEB_DIST_DIR %q: no index.html: %w", abs, err)
		}
		return abs, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for i := 0; i < 16; i++ {
		candidates := []string{filepath.Join(dir, "web", "dist")}
		if filepath.Base(dir) == "web" {
			candidates = append(candidates, filepath.Join(dir, "dist"))
		}
		for _, cand := range candidates {
			if _, err := os.Stat(filepath.Join(cand, "index.html")); err == nil {
				return filepath.Abs(cand)
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("web/dist/index.html not found (cwd=%s); run: cd web && npm run build — or set NFC_WEB_DIST_DIR", wd)
}
