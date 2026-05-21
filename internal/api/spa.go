package api

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

// WithSPA serves the API under /api/ and static files from static for all other GET/HEAD requests,
// falling back to index.html for SPA routing.
func WithSPA(apiHandler http.Handler, static fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			apiHandler.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		serveStaticOrIndex(w, r, static)
	})
}

func serveStaticOrIndex(w http.ResponseWriter, r *http.Request, static fs.FS) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	if !fs.ValidPath(p) {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(static, p)
	if err != nil {
		data, err = fs.ReadFile(static, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		setStaticCache(w, "index.html")
		_, _ = w.Write(data)
		return
	}
	ext := path.Ext(p)
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		switch ext {
		case ".js", ".mjs":
			ct = "text/javascript; charset=utf-8"
		case ".css":
			ct = "text/css; charset=utf-8"
		case ".json":
			ct = "application/json; charset=utf-8"
		case ".svg":
			ct = "image/svg+xml"
		default:
			ct = "application/octet-stream"
		}
	}
	w.Header().Set("Content-Type", ct)
	setStaticCache(w, p)
	_, _ = w.Write(data)
}

func setStaticCache(w http.ResponseWriter, filePath string) {
	// Fingerprint-Dateien: ändern den Namen pro Release, lang cachen
	if strings.HasPrefix(filePath, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	// SPA-Shell: immer beim Server nachfragen, damit neue Script-Hashes zuverlässig ankommen
	if filePath == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=600")
}
