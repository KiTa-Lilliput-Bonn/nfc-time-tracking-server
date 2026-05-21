.PHONY: help build-frontend build-linux build-darwin build-windows fetch-restic-redist build-windows-msi build check-web-embed test test-agent run run-dev clean sync-web-dist smoke-e2e e2e-ui verify-release docker-build

# WiX MSI (dotnet + WixToolset.Sdk): siehe installer/windows/wix/
WINDOWS_WIXPROJ := installer/windows/wix/NfcTimeTracking.wixproj

# First target must not be build-frontend (web/ may not exist yet).
.DEFAULT_GOAL := build

help:
	@echo "NFC Time Tracking – Arbeitsabläufe (Vollversion): docs/entwicklung-und-release.md"
	@echo
	@echo "Kurz: Alltag  = Terminal 1: cd web && npm run dev  |  Terminal 2: make run-dev"
	@echo "Kurz: Release = make build-with-web  (Binary: bin/nfc-time-tracker-server, gitignored)"
	@echo "Hinweis: make run-dev lädt die UI aus web/dist (sie wird vom aktuellen Verzeichnis aus gesucht —"
	@echo "         bei Problemen: im Repo-Root starten oder NFC_WEB_DIST_DIR setzen). Vite: UI unter :5173."
	@echo "Docker: make docker-build  |  docker compose up -d  (Volumes /data, /backup)"
	@echo "Windows-Installer (WiX MSI): make build-windows-msi  (nur Windows: dotnet + WixToolset.Sdk; lädt vor dem MSI-Pack restic redist via scripts/ci/fetch-restic-redist.sh)"
	@echo "Hinweis: Wenn config.yaml fehlt, ist der Server-Default Port 8080; dieses Repo enthält meist config.yaml (oft 8081)."
	@echo "        Vite proxyt /api auf 127.0.0.1:8080 — bei Abweichung: NFC_SERVER_PORT=8080 vor make run-dev oder vite.config.ts anpassen."

build-frontend:
	cd web && npm ci && npm run build

# After `npm run build` in web/, copy Vite output into the Go embed tree (required for release builds).
sync-web-dist:
	test -d web/dist
	rm -rf internal/web/dist
	mkdir -p internal/web/dist
	cp -R web/dist/. internal/web/dist/

# Clear error if embed tree is missing (Go embed would fail with an obscure message).
check-web-embed:
	@test -f internal/web/dist/index.html || (echo "internal/web/dist/index.html fehlt — zuerst: cd web && npm run build && make sync-web-dist" && exit 1)

# Tray/Systray: nur bei GOOS=windows (siehe cmd/server/run_ui_*.go mit //go:build).
# Linux/macOS und jede GOARCH: ohne Tray-Code im Binary — kein extra -tags im Makefile nötig.
# Windows: Icon + VERSIONINFO (Explorer/Start): go-winres → cmd/server/rsrc_windows_amd64.syso (gitignored).
WINRES_GO := github.com/tc-hib/go-winres@v0.3.3

build-linux:
	mkdir -p bin
	GOOS=linux GOARCH=amd64 go build -o bin/nfc-time-tracker-server-linux ./cmd/server

build-darwin:
	mkdir -p bin
	GOOS=darwin GOARCH=arm64 go build -o bin/nfc-time-tracker-server-darwin ./cmd/server

build-windows:
	mkdir -p bin
	cd cmd/server && go run $(WINRES_GO) make --arch amd64
	GOOS=windows GOARCH=amd64 go build -o bin/nfc-time-tracker-server.exe ./cmd/server

# Vor WiX: offizielles restic windows_amd64 + LICENSE nach installer/windows/redist/
fetch-restic-redist:
	bash "$(CURDIR)/scripts/ci/fetch-restic-redist.sh"

# WiX-MSI: WixToolset.Sdk ruft wix.exe auf — nur unter Windows unterstützt. MSI -> installer/windows/dist/
ifeq ($(OS),Windows_NT)
build-windows-msi: build-with-web build-windows fetch-restic-redist
	@command -v dotnet >/dev/null 2>&1 || (echo "build-windows-msi: dotnet SDK nicht gefunden." && exit 1)
	dotnet build "$(CURDIR)/$(WINDOWS_WIXPROJ)" -c Release
	@mkdir -p "$(CURDIR)/installer/windows/dist"
	@MSI=$$(find "$(CURDIR)/installer/windows/wix" -name "*.msi" -type f 2>/dev/null | head -1); \
	  if [ -z "$$MSI" ]; then echo "build-windows-msi: keine MSI unter installer/windows/wix gefunden." && exit 1; fi; \
	  cp -f "$$MSI" "$(CURDIR)/installer/windows/dist/"
else
build-windows-msi:
	@echo "build-windows-msi: WixToolset.Sdk erzeugt MSI nur unter Windows (wix.exe)."
	@echo "        Auf macOS/Linux: Workflow windows-installer in GitHub Actions, Artefakt .msi herunterladen."
	@exit 1
endif

build: check-web-embed
	mkdir -p bin
	go build -o bin/nfc-time-tracker-server ./cmd/server

# Production binary: build Vue, copy into internal/web/dist, then compile Go embed.
build-with-web: build-frontend sync-web-dist build
	@echo "Eingebetteter Web-Entry: $$(grep -oE 'src=\"/assets/[^\"]+\"' internal/web/dist/index.html | head -1)"

# Task 19: release binary + cross-compilation smoke (no run).
verify-release: build-with-web build-linux build-darwin build-windows

# Task 20: HTTP smoke (fresh temp DB, needs built binary + embedded UI).
smoke-e2e: build
	./scripts/smoke-e2e.sh

e2e-ui: build-with-web
	chmod +x scripts/e2e-web-server.sh
	cd web && npm ci && npx playwright install --with-deps chromium
	cd web && npm run e2e

test:
	go test ./... -v

# Agent-friendly test entrypoint: avoids flaky Cursor sandbox module cache by using the user's Go caches.
test-agent:
	./scripts/go-test-agent.sh ./...

run:
	go run ./cmd/server

run-dev:
	go run -tags dev ./cmd/server

docker-build:
	docker build -t nfc-time-tracking-server:local .

clean:
	rm -rf bin/ web/dist/
