# Entwicklung und Release (Kurzrezept)

Dieses Projekt führt den API-Server als Go-Binary und die Oberfläche als Vite/Vue-App unter `web/`. Für lokale Web-Entwicklung werden üblicherweise **zwei Prozesse** parallel betrieben: Vite liefert die UI mit Hot-Reload, der Go-Server liefert `/api` und (je nach Build-Tag) statische Dateien.

Für den Frontend-Build ist **Node.js 24 LTS** (oder neuer) empfohlen; siehe `engines` in `web/package.json`.

## Tägliches Arbeiten (Frontend + API)

### Einmalig bzw. nach größeren Abhängigkeits-Änderungen

- Abhängigkeiten installieren:

```bash
cd web && npm ci
```

- `go run -tags dev` liest in der Dev-Variante statische Dateien aus `web/dist` (siehe `internal/web/embed_dev.go`). Damit der Go-Server nicht in leere/veraltete Assets läuft, `web/dist` zumindest **einmal** bauen:

```bash
cd web && npm run build
```

### Laufend: zwei Terminals (empfohlen)

**Terminal 1 (Vite / UI, Hot Reload):**

```bash
cd web && npm run dev
```

Vite startet lokal (typisch `http://127.0.0.1:5173/`) und proxyt API-Requests unter `/api` an den Go-Server. Das Proxy-Ziel steht in `web/vite.config.ts` (derzeit `http://127.0.0.1:8080`).

**Terminal 2 (API-Server, Dev-Build-Tag):**

Im Repo-Root (dort, wo `config.yaml` liegen kann):

```bash
make run-dev
# entspricht: go run -tags dev ./cmd/server
```

**Wichtig (Port-Alignment):** Ohne `config.yaml` fällt der Server auf `internal/config.Defaults()` zurück (Port 8080). Wenn im Repo-Root eine `config.yaml` existiert, setzt deren `server.port` den Port. In diesem Fall muss Vites Proxy-Ziel dazu passen, z. B.:

- Entweder den Server-Port an Vite anpassen (einfachste fachliche Lösung: Vite-Proxy in `web/vite.config.ts` auf euren `server.port` setzen), **oder**
- per Umgebung den API-Port auf 8080 ziehen, damit es mit dem aktuellen Vite-Proxy übereinstimmt:

```bash
NFC_SERVER_PORT=8080 make run-dev
```

### Verifikation während der Entwicklung (optional, aber sinnvoll)

- Go-Tests:
  - normal: `make test` (bzw. `go test ./...`)
  - in agent-/Sandbox-Setups, falls Go-Modul-Caches Probleme machen: `make test-agent` (siehe `scripts/go-test-agent.sh`)
- Frontend-Build-Qualität: `cd web && npm run build` (führt u. a. `vue-tsc --noEmit` aus)

## „Release-Tag vX“ (Production-Binary mit eingebetteter Web-UI)

Ziel: **eine** auslieferbare Server-Binary mit eingebetteten Assets aus `internal/web/dist`, ohne dass ihr bei jeder lokalen Iteration riesige Hash-Diffs in Git „mitzieht“.

### 1) Release-Binary bauen

Im Repo-Root:

```bash
make build-with-web
```

Was das macht (siehe `Makefile`): `npm ci` + `npm run build` in `web/`, Kopieren nach `internal/web/dist/`, danach `go build` in `bin/`.

- Ausgabe-Binary: `bin/nfc-time-tracker-server` (steht in `.gitignore` und ist **nicht** gemeint als Git-Inhalt, sondern als Release-Artefakt)
- `internal/web/dist/` steht in `.gitignore` und wird **nicht** versioniert; `make build-with-web` legt es bei Bedarf lokal/auf der CI-Runner-Maschine an

### 2) Integrität (Checksumme) für Arbeitsanweisung/IT-Austausch

Auf macOS z. B.:

```bash
shasum -a 256 bin/nfc-time-tracker-server
```

### 3) Quellcode-Tagging (optional, getrennt vom Binary)

Viele Teams taggen **Quell-Commits** (nicht die Binary):

```bash
git tag -a vX.Y.Z -m "Release vX.Y.Z"
# danach: git push origin vX.Y.Z
```

Nach `git push` eines Tags `vX.Y.Z` erzeugt der Workflow [`.github/workflows/release.yml`](../.github/workflows/release.yml) automatisch ein GitHub Release mit Linux-, macOS- (Apple Silicon/arm64) und Windows-Artefakten sowie einem **Docker-Image** auf GHCR (siehe Abschnitt **Docker** und **CI** unter Windows/MSI).

Wenn ihr Release-Binaries extern ablegt, verknüpft ihr diese Version/Checksum mit `vX.Y.Z` in eurer Release-Notiz (GitHub Release, internes Fileshare, …).

## Docker (Linux amd64, GHCR)

Ziel: Server als Container mit zwei persistenten Volumes — **`/data`** (SQLite, JWT-Secret, Logs) und **`/backup`** (geplante Backups / Restic-Repo).

- **Image:** `ghcr.io/waynenani/nfc-time-tracking-server:<tag>` (bei Release-Tag `vX.Y.Z` zusätzlich `:latest`).
- **restic:** nur als Alpine-Paket im Image (`PATH`), kein `tools/restic` wie bei der Windows-MSI.
- **Backup-Ziel:** im Image `NFC_BACKUP_TARGET_PATH=/backup`; beim ersten Start wird `backup_target_path` in der DB gesetzt, wenn noch leer (Backup selbst bleibt aus bis zur Aktivierung in der Admin-UI).
- **Healthcheck:** `GET /api/v1/health` (öffentlich, ohne JWT) — nicht `android-lan/health-status`.

### Lokal bauen und starten

```bash
make docker-build
# oder: docker build -t nfc-time-tracking-server:local .

docker compose up -d
docker compose logs -f
```

UI: `http://127.0.0.1:8080/` — beim ersten Start steht das einmalige Admin-Passwort in den Logs (`bootstrap: created superadmin …`).

Konfiguration im Image: [`config.docker.yaml`](../config.docker.yaml) als `/app/config.yaml`. Weitere Overrides per Umgebung (siehe README), z. B. `NFC_AUTH_JWT_SECRET`.

Der Entrypoint [`docker-entrypoint.sh`](../docker-entrypoint.sh) setzt beim Start (als root) die Besitzerrechte auf `/data` und `/backup` für den Container-User `app` (UID 1000), damit gemountete Volumes beschreibbar sind.

### Upgrade

Neues Image ziehen, Container neu erstellen, **dieselben** Named Volumes `nfc-data` und `nfc-backup` beibehalten.

## Eingebettetes Web-UI (ohne Git-Rauschen)

Viele Vite-Assets tragen **Content-Hashes in Dateinamen** — daraus würden sich bei jedem Build massive Git-Diffs ergeben. Deshalb: `internal/web/dist/` **nicht** im Repository halten, sondern per `make build-with-web` bzw. `cd web && npm run build && make sync-web-dist` erzeugen, bevor `go build` (ohne `dev`) die UI einbettet.

## Windows: MSI (WiX)

Ziel: Eine **`.msi`** (Windows Installer), die **ohne Administratorrechte** installiert (`Scope="perUser"`), Standardpfad **`%LOCALAPPDATA%\NfcTimeTracking`** (voll beschreibbar für den angemeldeten Benutzer), die Windows-Launcher-Skripte mitliefert und ein **Feature „Autostart“** (Startup-Verknüpfung auf `start-nfc-time-tracking.vbs`) optional per Installation steuerbar macht.

Quellen: [WiX Toolset](https://wixtoolset.org/) (hier **WixToolset.Sdk 5.x** im Projekt [installer/windows/wix/NfcTimeTracking.wixproj](installer/windows/wix/NfcTimeTracking.wixproj) und [installer/windows/wix/Package.wxs](installer/windows/wix/Package.wxs)).

**Startmenü-Icon und Anzeigename der EXE:** Die Startmenü- und Autostart-Verknüpfungen in der MSI referenzieren in `Package.wxs` ein explizites **WiX-`Icon`** (`cmd/server/tray.ico`), damit Windows nicht automatisch das Symbol für `.vbs`/`.bat` wählt. Die Windows-**EXE** enthält per [**go-winres**](https://github.com/tc-hib/go-winres) (Konfiguration [`cmd/server/winres/winres.json`](../cmd/server/winres/winres.json)) das gleiche **Anwendungs-Icon** sowie **VERSIONINFO** (`FileDescription` / `ProductName`: „NFC Time Tracking Server“) für Explorer, Suche und Task-Manager. `make build-windows` (und die CI-Windows-Jobs) führen vor `go build` `go-winres make` aus; die erzeugte `cmd/server/rsrc_windows_amd64.syso` wird nicht eingecheckt (`.gitignore`). Release-Builds setzen die vierstellige Datei-/Produktversion aus dem Git-Tag.

### Voraussetzungen

- [.NET SDK](https://dotnet.microsoft.com/download) (für `dotnet build` des WiX-Projekts)
- Repo-Root: zuerst eingebettetes Web + Windows-EXE bauen (siehe `Makefile`)

### Lokal bauen (nur Windows)

**WixToolset.Sdk** startet `wix.exe`, das derzeit **nur unter Windows** unterstützt wird (auf macOS/Linux bricht `dotnet build` mit Hinweis „WiX Toolset only supports Windows“ ab).

```bat
make build-windows-msi
```

Ausgabe: `installer/windows/dist/NFC-Time-Tracking-<Version>.msi` (Dateiname/Produktversion stehen in [installer/windows/wix/NfcTimeTracking.wixproj](installer/windows/wix/NfcTimeTracking.wixproj) bzw. `Package`-`Version` in `Package.wxs` — bei Release beide anheben).

**Mitgeliefertes restic:** Vor dem WiX-Build lädt [`scripts/ci/fetch-restic-redist.sh`](../scripts/ci/fetch-restic-redist.sh) das offizielle **restic windows_amd64** (ZIP aus GitHub Releases, SHA256 gegen `SHA256SUMS`) sowie die **originale LICENSE** nach [`installer/windows/redist/`](../installer/windows/redist/). Die gebündelte Semver steht in [`restic-version.txt`](../installer/windows/redist/restic-version.txt). Nach Installation unter **`%LOCALAPPDATA%\NfcTimeTracking\tools\`** liegen `restic.exe` und `restic-LICENSE.txt`. **Zur Laufzeit** nutzt der Backup-Dienst [`resticpath.ResolveRestic()`](../internal/resticpath/resticpath.go): zuerst die gebündelte Binary unter `tools/` neben der Server-EXE, sonst **`restic` aus dem `PATH`** (sinnvoll für Linux/macOS ohne MSI). [`BundledRestic()`](../internal/resticpath/resticpath.go) liefert nur den gebündelten Pfad.

**Auf macOS/Linux:** MSI über den Workflow [windows-installer](../.github/workflows/windows-installer.yml) manuell starten (**Actions → windows-installer → Run workflow**) und das **Artifact** herunterladen (oder auf einer Windows-Maschine `make build-windows-msi` ausführen).

### CI

- **Pull Requests und Push auf `main`:** Workflow [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) auf `ubuntu-latest`: `go test ./...` und `cd web && npm ci && npm run build` (inkl. `vue-tsc`). Läuft bei jedem PR und bei jedem Push auf `main`.
- **GitHub Release (Linux + macOS + Windows + Docker):** Workflow [`.github/workflows/release.yml`](../.github/workflows/release.yml) läuft beim Push eines Tags `vX.Y.Z` (nur numerische Semver-Teile). Nach Tag-Validierung bauen **`build-linux`** und **`build-macos`** parallel auf `ubuntu-latest` (jeweils Frontend-Embed + Cross-Compile), **`build-windows`** parallel auf `windows-latest`, **`build-docker`** pusht das Image nach **`ghcr.io/<repository>`** (`:vX.Y.Z` und `:latest`). Artefakte: Linux-`.tar.gz` und macOS-`.tar.gz` (jeweils Binary + `config.example.yaml`; macOS nur **darwin/arm64**, kein Intel-amd64), Windows-`.exe` und `.msi`. Die **WiX-Paketversion** und der MSI-Dateiname kommen aus dem Tag (Skript [`scripts/ci/patch-wix-version.sh`](../scripts/ci/patch-wix-version.sh)). Der Job **`release`** erzeugt SHA256-Dateien und legt ein **GitHub Release** mit allen Assets an. Auf macOS ohne MSI liegt **restic** wie bei Linux über den **`PATH`** (siehe Abschnitt restic oben). Dafür braucht das Repository unter **Settings → Actions → General → Workflow permissions** typischerweise **Read and write** (damit `GITHUB_TOKEN` Releases und GHCR-Packages schreiben darf).
- **Nur MSI (manuell):** [`.github/workflows/windows-installer.yml`](../.github/workflows/windows-installer.yml) nur per `workflow_dispatch`. Optional kann beim Start eine **Semver ohne `v`** angegeben werden; dann werden `Package.wxs` / `OutputName` im Runner vor dem Build angepasst. Ohne Eingabe gelten die im Repo eingecheckten WiX-Defaults.

### Silent / IT (`msiexec`)

```bat
msiexec /i NFC-Time-Tracking-1.0.0.msi /qn
```

- **Ohne Autostart-Feature (nur Hauptprogramm):** `msiexec /i ... /qn ADDLOCAL=Main` (darin enthalten: gebündeltes **restic** unter `tools\` samt `restic-LICENSE.txt`)

Vor Installation läuft optional ein **CloseApplication** (WiX Util) für `nfc-time-tracker-server.exe`, damit Dateien ersetzt werden können.

### Konfiguration nach Installation

Liegt noch keine `config.yaml` im Installationsordner, legt die MSI sie beim ersten Install aus **`config.example.yaml`** an (Komponente mit `NeverOverwrite`). Upgrades überschreiben eine vorhandene `config.yaml` nicht.

### Code-Signing

Ohne Signatur kann Windows SmartScreen warnen. Für breitere Ausrollung: **`.msi`** mit einem gültigen Zertifikat signieren (externer Schritt, nicht im Makefile).

## Windows: Autostart beim Anmelden

Ziel: Server startet nach der Benutzer-Anmeldung **ohne sichtbares Konsolenfenster**, mit korrektem Arbeitsverzeichnis (damit `config.yaml` und relative Pfade wie `./data/` stimmen).

### Infobereich (Systemtray, nur Windows)

Die Windows-`.exe` legt ein Symbol im Infobereich an. Menüeinträge:

- **Im Browser öffnen** — öffnet die lokale Web-Oberfläche (bei typischer Bindung an alle Interfaces z. B. `http://127.0.0.1:<port>/`).
- **Ordner öffnen** — öffnet den **Ordner der Server-Programmdatei** im Explorer (hilfreich, wenn `config.yaml` und `data\` neben der EXE liegen).
- **Beenden** — beendet den HTTP-Server und schließt den Prozess (sodass z. B. Datenbank- und Log-`defer` in `main` ausgeführt werden).

Linux- und macOS-Binaries werden **ohne** Tray-Code und ohne Systray-Abhängigkeit gebaut (`//go:build`-Trennung).

**Gleiches Markenbild wie das Tab-Favicon:** Kreis wie in [`web/public/app-icon.svg`](../web/public/app-icon.svg); die eingebettete Windows-`tray.ico` folgt derselben Geometrie (Farbe `#42b883`). Nach Änderung des SVG: `python3 scripts/gen-tray-ico.py` ausführen und die erzeugte [`cmd/server/tray.ico`](../cmd/server/tray.ico) mit versionieren.

### 1) Installationsordner und Dateien

Lege einen Ordner an (Beispiel `C:\Programme\NfcTimeTracking\`) und kopiere dorthin:

- `nfc-time-tracker-server.exe` (z. B. nach `make build-with-web` und `make build-windows` erzeugt: `bin/nfc-time-tracker-server.exe`)
- `config.yaml` (aus Vorlage `config.example.yaml` im Repo-Root)
- die Launcher-Skripte aus dem Repo: [`scripts/windows/start-nfc-time-tracking.vbs`](../scripts/windows/start-nfc-time-tracking.vbs), optional [`scripts/windows/start-nfc-time-tracking-console.bat`](../scripts/windows/start-nfc-time-tracking-console.bat) und optional [`scripts/windows/install-startup-shortcut.ps1`](../scripts/windows/install-startup-shortcut.ps1)

Das **VBScript** setzt das Arbeitsverzeichnis auf den Ordner der `.vbs`-Datei und startet die EXE mit verstecktem Fenster (`Run` mit Fensterstil `0`). Die **Startup-Verknüpfung** muss auf die `.vbs` zeigen, **nicht** direkt auf die `.exe` (sonst riskiert ihr falsches cwd und damit eine falsche oder zweite Datenbank).

### 2) Autostart einrichten

**Manuell:** Verknüpfung im Benutzer-Startup anlegen, Ziel = `start-nfc-time-tracking.vbs` im Installationsordner.

- Ordner: `%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup`

**Per PowerShell (optional):** Im Installationsordner die Skripte aus dem Repo liegen lassen, dann z. B.:

```powershell
powershell -ExecutionPolicy Bypass -File "C:\Programme\NfcTimeTracking\install-startup-shortcut.ps1" -InstallDir "C:\Programme\NfcTimeTracking"
```

Das Skript [`scripts/windows/install-startup-shortcut.ps1`](../scripts/windows/install-startup-shortcut.ps1) legt eine `.lnk` an, die `wscript.exe` mit dem VBS-Launcher aufruft (und prüft, dass VBS und EXE vorhanden sind).

### 3) Support / sichtbare Konsole

Zum Debuggen mit sichtbarem Terminal-Log: **`start-nfc-time-tracking-console.bat`** per Doppelklick oder aus `cmd` starten. **Nicht** ins Startup legen.

### 4) Logs ohne Konsole

Ohne Konsole erscheinen `log`-Ausgaben nicht im Fenster. Nutzt die in `config.yaml` konfigurierte **Logdatei** (`logging.file`, siehe README) oder stoppt den Prozess im Task-Manager und startet kurz mit dem Support-BAT.

### 5) Upgrade

Laufenden `nfc-time-tracker-server.exe` beenden, Backup von `data\` (und ggf. `config.yaml`), neue **EXE** über die alte kopieren. VBS, BAT und Startup-Verknüpfung bleiben unverändert, solange EXE-Name und Installationsordner gleich bleiben.