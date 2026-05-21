//go:build !dev

package web

import (
	"embed"
	"io/fs"
)

// Embedded UI: run `cd web && npm run build && make sync-web-dist` so internal/web/dist exists before `go build`.
// all: includes Vite chunks named _plugin-vue_* (Go’s default directory embed skips names starting with _).
//
//go:embed all:dist
var embedded embed.FS

// FS returns the embedded Vite output (subtree dist/).
func FS() (fs.FS, error) {
	return fs.Sub(embedded, "dist")
}
