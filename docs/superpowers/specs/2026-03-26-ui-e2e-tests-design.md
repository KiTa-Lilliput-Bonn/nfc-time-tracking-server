# UI E2E Tests (Playwright) - Design

## Kontext
Ziel ist es, UI-Regressionen fuer die Kern-Funktionen schnell und reproduzierbar zu erkennen.
Aktuell existieren Go-Tests und ein HTTP-Smoketest (`make smoke-e2e`), aber keine UI-E2E-Abdeckung.

## Ziele
- Headless UI-E2E-Tests (CI-first) fuer:
  - Dienstplan
  - Korrekturen
  - Zeitsaldo
  - Abwesenheiten
- Frische, isolierte DB pro Testlauf.
- Deterministischer Admin-Login ohne Log-Parsing.
- Klare, wartbare Seed-Strategie fuer Testdaten.

## Nicht-Ziele
- Vollstaendige UI-Abdeckung aller Seiten.
- Visuelle Snapshot-Tests.
- End-to-end FTP/Import Tests (separat behandelbar).

## Ansatz (empfohlen)
**Option 2: deterministischer Test-Bootstrap**

### Test-Modus (Backend)
Ein expliziter Testmodus stellt reproduzierbare Logins und saubere DBs sicher:
- `NFC_TEST_MODE=1` aktiviert Test-Setup.
- `NFC_TEST_ADMIN_PASSWORD` setzt ein fixes Admin-Passwort beim Bootstrap.
- **MustChangePassword wird im Testmodus auf `false` gesetzt**, damit der UI-Login ohne Pflichtdialog moeglich ist.
- Testlauf startet den Server mit einer temporaeren SQLite-DB (neue Datei pro Run).

**Sicherheitsregeln fuer Testmodus (pflicht)**
- Testmodus nur aktivieren, wenn `NFC_DATABASE_PATH` unter `os.TempDir()` liegt.
- Testmodus nur erlauben, wenn `NFC_SERVER_HOST=127.0.0.1`.

### Testdaten-Seed (API-basiert)
Testdaten werden ueber vorhandene Admin-APIs erstellt, um echte Pfade abzudecken:
- Mitarbeiter anlegen
- WorkPeriods (manuell) anlegen
- Korrekturen erstellen
- Dienstplan-Schichten erstellen
- Abwesenheiten erstellen
- WeeklyHours setzen fuer Saldo-Berechnungen

## Komponenten
1. **Playwright-Konfiguration**
   - `@playwright/test` als Runner.
   - `webServer` startet den Go-Server mit Test-Config und temp DB.
   - `workers`: initial 1 (stabil), spaeter parallelisierbar.
   - Headless by default; optional headed lokal via ENV.
   - Server-Ready-Check via `/api/v1/health` (Timeout explizit setzen).
   - Base-URL aus `NFC_SERVER_PORT` ableiten.

2. **Test-Helper**
   - `loginAsAdmin()` (feste Credentials aus Test-Env)
   - `seedEmployee()`, `seedWorkPeriod()`, `seedCorrection()`, `seedAbsence()`, `seedSchedule()`, `seedWeeklyHours()`
   - Auth fuer Seed via `POST /api/v1/auth/login` und JWT im Authorization-Header
   - Date-Helper fuer stabile Zeitraeume/ISO-Wochen

3. **Test-Flows**
   - Dienstplan speichern und nach Reload sichtbar
   - Korrektur anlegen und in Tabelle sichtbar
   - Zeitsaldo berechnen (WorkPeriods + WeeklyHours)
   - Abwesenheit anlegen und in Liste sichtbar

## Datenfluss pro Testlauf
1. Temp-DB erstellen
2. Server mit Test-Config starten (`NFC_TEST_MODE=1`)
3. Admin-Login (fixes Passwort)
4. Seed via API
5. UI-Flow ausfuehren + Assertions

## Fehlerbehandlung & Stabilitaet
- Retry bei flaky UI-Elementen (Playwright retries)
- Screenshots/Tracing bei Fehlern
- Deterministische Daten (keine Abhaengigkeit von echter Uhrzeit)
- Optional: `TZ=UTC` in CI, um Datumsgrenzen stabil zu halten
- Testmodus soll Holiday-Seeding deterministisch machen (z. B. Seed nur fuer ein fixes Jahr
  oder Holiday-Seeding im Testmodus deaktivieren).
- Testlauf initial serial (ein Worker), um DB-Interferenz zu vermeiden; spaeter pro-Worker DB.

## Tests (erster Wurf)
1. **Dienstplan**
   - Mitarbeiter erstellen
   - Schicht eintragen + speichern
   - Reload -> Schicht sichtbar

2. **Korrekturen**
   - WorkPeriod erstellen
   - Korrektur anlegen
   - Tabelle zeigt Korrekturwerte

3. **Zeitsaldo**
   - WeeklyHours setzen
   - WorkPeriods im Monat anlegen
   - Balance-Karte zeigt erwarteten Saldo (explizit gesetzter Monat/Jahr)

4. **Abwesenheiten**
   - Abwesenheit anlegen
   - Liste zeigt Eintrag

## Konfiguration/ENV (Vorschlag)
- `NFC_TEST_MODE=1`
- `NFC_TEST_ADMIN_PASSWORD=...`
- `NFC_AUTH_JWT_SECRET=...`
- `NFC_DATABASE_PATH=<tempfile>`
- `NFC_SERVER_PORT=<freier port>`
 - `NFC_SERVER_HOST=127.0.0.1`

## Akzeptanzkriterien
- Tests laufen headless lokal und in CI.
- Keine Log-Parsing-Abhaengigkeit fuer Login.
- Jeder der vier Flows ist durch mindestens einen E2E-Test abgedeckt.
- Ein gescheiterter Test liefert Screenshot/Trace.

## CI-Notizen (Kurz)
- Linux-Runner mit Go + Node.
- `npm ci` im Frontend, Playwright Browser installieren.
- Playwright System-Dependencies installieren (`playwright install --with-deps` oder
  eine CI-Action/Container mit vorinstallierten Dependencies).
- Test-Report (HTML/Trace) als Artefakt bei Fehlern.

## SPA-Serving fuer E2E
- Tests starten den Go-Server und serven die SPA ueber das Go-Handler-Frontend.
- Vor dem Serverstart: `cd web && npm run build` (oder `make build-with-web`).
- Dev-Tag (`go run -tags dev`) ist moeglich, wenn `web/dist` vorhanden ist.

## Port/Startup-Strategie
- Fester, freier Port via ENV (z. B. 8091) fuer stabilen `baseURL`.
- Wenn Port belegt, Testlauf abbrechen (oder im Runner freien Port ermitteln).

## Selektoren (UI-Stabilitaet)
- Bevorzugt `data-testid` fuer kritische Assertions (falls vorhanden/neu eingefuehrt).

## Repo-Layout (Vorschlag)
- `web/` enthaelt die Playwright-Tests (z. B. `web/e2e/`).
- `package.json` im `web/` fuehrt `@playwright/test`.
- Optional: `make e2e-ui` fuer lokalen Start der Suite.

