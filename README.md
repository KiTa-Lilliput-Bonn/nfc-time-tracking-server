# NFC Time Tracking Server

Zeiterfassung mit Go-Backend, SQLite, NFC-Stempeldaten (u.a. LAN-Polling) und eingebettetem Vue-Frontend.

## Voraussetzungen

- Go
- Node.js 24 LTS (oder neuer) und npm

## Projektueberblick

- Backend: Go HTTP-Server unter `cmd/server`
- Datenbank: SQLite-Datei unter `data/`
- Frontend: Vue/Vite unter `web/`
- Produktionsauslieferung: gebaute Frontend-Dateien werden nach `internal/web/dist/` kopiert und in das Go-Binary eingebettet (dieser Ordner ist lokal/CI-Build-Output, nicht Teil des Repos; siehe `.gitignore`)

## Konfiguration

Standardmaessig sucht der Server nach `config.yaml` im Projektverzeichnis. Als Vorlage dient `config.example.yaml` im Repo-Root.

Typischer Start (optional — ohne `config.yaml` startet der Server mit eingebauten Defaults, Port 8080):

```bash
cp config.example.yaml config.yaml
```

Danach die Werte in `config.yaml` an die eigene Umgebung anpassen, insbesondere:

- `database.path`
- `auth.jwt_secret`

Alternativ koennen wichtige Werte auch per Umgebungsvariablen gesetzt werden, z. B.:

- `NFC_SERVER_PORT`
- `NFC_SERVER_HOST`
- `NFC_DATABASE_PATH`
- `NFC_AUTH_JWT_SECRET`
- `NFC_AUTH_EXPIRY_HOURS`
- `NFC_LOGGING_FILE`
- `NFC_LOGGING_MAX_AGE_DAYS`
- `NFC_BACKUP_TARGET_PATH` (setzt `backup_target_path` beim ersten Start, wenn noch leer; Docker-Image: `/backup`)

### Docker

Container mit Volumes `/data` und `/backup`: siehe [`docker-compose.yml`](docker-compose.yml) und [`docs/entwicklung-und-release.md`](docs/entwicklung-und-release.md#docker-linux-amd64-ghcr).

### Logging

Logs gehen parallel nach **stderr** und in die Datei `logging.file` (Default `./data/server.log`, auch wenn `config.yaml` fehlt und die eingebauten Defaults greifen).

- HTTP-Request-Zeilen (chi `Logger`) sind im **Terminal farbig**, solange **stderr** ein TTY ist; in der **Logdatei** werden ANSI-Steuerzeichen entfernt, damit die Datei lesbar bleibt.

- Die aktive Logdatei rotiert beim **Wechsel des lokalen Kalendertags**: ein Hintergrundjob prueft etwa **jede Minute** die Wanduhr und benennt die bisherige `server.log` in eine Backup-Datei mit Zeitstempel um (z. B. `server-2026-05-12T00-00-00.000.log`), sobald ein neuer Tag erkannt wird; danach entsteht eine neue, leere `server.log`. So funktioniert die Tages-Rotation auch nach **Standby/Resume**, ohne dass der Server neu gestartet werden muss (typisch innerhalb von etwa einer Minute nach Mitternacht bzw. nach dem Aufwachen).
- Beim **Start** rotiert der Server zusaetzlich einmal, wenn die Logdatei schon existiert und ihr letztes Aenderungsdatum (**mtime**, lokaler Kalendertag) **vor** dem heutigen Tag liegt. So werden nach einem **Neustart ueber Mitternacht** keine neuen Eintraege mehr an die Datei des Vortags angehaengt.
- `logging.max_age_days` legt fest, wie viele Tage rotierte Dateien aufbewahrt werden (`0` = unbegrenzt). Default: `14`.
- `logging.max_backups` begrenzt zusaetzlich die Anzahl rotierter Dateien (`0` = unbegrenzt).
- `logging.max_size_mb` ist eine harte Groessen-Grenze pro Datei (Default `20`). Wird sie innerhalb eines Tages ueberschritten (z. B. bei einem Fehler-Sturm), rotiert die Datei zusaetzlich, sodass an diesem Tag mehrere Backups entstehen koennen.
- `logging.level` ist aktuell reserviert (das stdlib-`log` kennt keine Levels) und wird ignoriert.

### Leitung-Dashboard

Optional: Standard-Stichtag fuer die Team-Uebersicht im Leitung-Dashboard. App-Einstellung `dashboard.team_overview.as_of_default` im Format `YYYY-MM-DD`.

### LAN-Ziele (Android-Stamps / Mitarbeiter-Sync) und Login-Rate-Limit

- Einstellung **android_lan_targets** (Superadmin): JSON-Array von Zielen, jedes mit `id`, `host`, `port`, `api_client_id` (gepaarter Client), optional `label`. In der Verwaltung (Android-API / LAN-Pairing) sind Key und LAN-Adresse **pro Gerät in einer Zeile** gebündelt; beim Speichern gilt `id` = `api_client_id` = Client-ID. Der Server pollt `GET /v1/stamps` auf jedem Ziel im Intervall **stamps_poll_interval_seconds** und verwaltet pro Ziel ein eigenes Watermark. **Ist der Stempel-Poll aktiv** (`stamps_poll_interval_seconds` > 0), führt der Server nach jedem Poll-Zyklus zusätzlich den **Mitarbeitenden-Abgleich** auf alle konfigurierten LAN-Ziele aus (dieselbe Logik wie die manuellen Endpunkte `POST /api/v1/android-lan/sync-employee-ids(-all)`). Bei `stamps_poll_interval_seconds = 0` bleibt nur der manuelle Sync in der Verwaltung. Pro **host** gelten dieselben Regeln wie zuvor bei **android_lan_host**: nur **IPv4**; erlaubt sind **private RFC-1918-Adressen** und **Loopback** (`127.0.0.0/8`). **IPv6** und Hostnamen, die nur IPv6 liefern, werden abgelehnt. Der Bereich **169.254.169.0/24** (u.a. Cloud-Metadaten) ist gesperrt.
- **POST /api/v1/auth/login** ist auf **20 Anfragen pro Minute und Client-IP** begrenzt (Schutz vor Brute-Force). Hinter einem Reverse-Proxy muss **X-Forwarded-For** (oder vergleichbar) korrekt gesetzt werden, damit die IP-Ermittlung stimmt (vgl. chi `RealIP`).

## Lokale Entwicklung

### Backend im Dev-Modus starten

Im Dev-Modus werden Frontend-Dateien direkt aus `web/dist` gelesen:

```bash
go run -tags dev ./cmd/server
```

Oder ueber Make:

```bash
make run-dev
```

Wichtig: Auch im Dev-Modus sollte vorher das Frontend gebaut worden sein:

```bash
cd web && npm install && npm run build
```

### Frontend bauen

```bash
cd web
npm install
npm run build
```

### Frontend in die Go-Embed-Struktur kopieren

```bash
make sync-web-dist
```

## Produktions-Build

Ein kompletter Build inklusive Frontend-Build und Embed-Kopie:

```bash
make build-with-web
```

Das erzeugte Binary liegt danach unter:

```bash
bin/nfc-time-tracker-server
```

Wenn `web/dist` bereits gebaut wurde und schon nach `internal/web/dist` synchronisiert ist, reicht auch:

```bash
make build
```

## Cross-Compile

Linux:

```bash
make build-linux
```

Windows:

```bash
make build-windows
```

Windows-Installer (WiX-MSI, **nur unter Windows**: [dotnet SDK](https://dotnet.microsoft.com/download) + WixToolset.Sdk; auf dem Mac die MSI über GitHub Actions bauen oder Artefakt laden):

```bash
make build-windows-msi
```

Komplett-Check (eingebettetes Web + Linux- und Windows-Binaries, ohne MSI-Build):

```bash
make verify-release
```

### Windows: Autostart beim Anmelden

Skripte und Schritt-fuer-Schritt: [docs/entwicklung-und-release.md](docs/entwicklung-und-release.md) (Abschnitte **Windows: MSI (WiX)** und **Windows: Autostart beim Anmelden**). Dateien unter [`scripts/windows/`](scripts/windows/).

## Tests ausfuehren

### Gesamte Go-Test-Suite

```bash
make test
```

oder direkt:

```bash
go test ./... -v
```

### Frontend-Typcheck und Produktionsbuild

```bash
cd web
npm run build
```

### Automatischer Smoke-Test

Der Smoke-Test baut das Binary und prueft unter anderem:

- Health-Endpoint
- Auslieferung des SPA-Root
- SPA-Fallback fuer Deep Links
- Admin-Login mit initialem Einmalpasswort

Start:

```bash
make smoke-e2e
```

## Anwendung starten

### Direkt mit Go

```bash
go run ./cmd/server
```

### Mit gebautem Binary

```bash
./bin/nfc-time-tracker-server
```

Optional kann ein anderer Pfad zur Konfigurationsdatei uebergeben werden:

```bash
./bin/nfc-time-tracker-server /pfad/zu/config.yaml
```

## Erster Login nach dem Setup

Beim ersten Start mit leerer Datenbank legt der Server automatisch einen Superadmin an:

- Benutzername: `admin`
- Anzeigename: `Administrator`
- Rolle: `superadmin`

Das einmalige Initialpasswort wird beim Start ins Server-Log geschrieben.

Beispielhaft steht dort sinngemaess:

```text
bootstrap: created superadmin user "admin" with one-time password: <PASSWORT>
```

Dann:

1. Server starten
2. Browser oeffnen: `http://localhost:8080`
3. Mit Benutzer `admin` und dem im Log ausgegebenen Passwort anmelden
4. Direkt danach das Passwort aendern

## Hinweise zum ersten Setup

- Falls noch keine Feiertage fuer das aktuelle und naechste Jahr vorhanden sind, werden diese beim Start automatisch erzeugt.
- Wenn `auth.jwt_secret` leer ist, erzeugt der Server ein temporaeres Secret. Fuer produktive Nutzung sollte ein festes Secret gesetzt werden.
## Nuetzliche Make-Ziele

- `make build-frontend` - baut das Vue-Frontend
- `make sync-web-dist` - kopiert `web/dist` nach `internal/web/dist`
- `make build` - baut das Go-Binary aus der bereits synchronisierten Embed-Struktur
- `make build-with-web` - baut Frontend und Binary komplett
- `make build-linux` - Linux-Binary
- `make build-windows` - Windows-Binary
- `make verify-release` - kompletter Release-Check inkl. Cross-Compile
- `make test` - gesamte Go-Test-Suite
- `make smoke-e2e` - automatischer HTTP-Smoke-Test
- `make run` - Server lokal starten
- `make run-dev` - Dev-Start mit `-tags dev`

