# NFC Time Tracking Server – Design Spec

## Übersicht

Server-Anwendung mit Web-UI für die Android-App "nfc-time-tracking". Erfasst Arbeitszeiten von Mitarbeitern über NFC-Stempel, importiert diese per FTP von einer Fritz!Box, und bietet eine Web-Oberfläche zur Auswertung, Verwaltung und Saldierung.

**Zielplattformen:** Windows, Linux
**Erwartete Größe:** 20–100 Mitarbeiter
**Deployment:** Single Binary (Go + eingebettetes Vue.js-Frontend + SQLite)

---

## 1. Architektur

### 1.1 Tech-Stack

| Komponente | Technologie |
|---|---|
| Backend | Go (net/http, chi Router) |
| Datenbank | SQLite (modernc.org/sqlite, pure Go, kein CGO) |
| Migrationen | goose |
| Frontend | Vue 3 + Composition API, Pinia, Vue Router, PrimeVue |
| Authentifizierung | JWT + bcrypt |
| PDF-Export | Go-basierte PDF-Bibliothek |
| Konfiguration | YAML-Datei + Umgebungsvariablen |
| Embedding | Go embed.FS für Frontend-Assets |

### 1.2 Komponenten

- **FTP-Importer** – Holt CSV-Dateien periodisch (konfigurierbares Intervall, z.B. alle 5 Min) und auf manuellen Trigger. Parst Zeitstempel (Millisekunden-Präzision) + NFC-Tag-ID. Duplikat-Erkennung über Unique Constraint.
- **REST API** – JSON-basierte API, versioniert (`/api/v1/...`). Dient sowohl der Web-UI als auch einer zukünftigen direkten Android-App-Anbindung.
- **Berechnungs-Engine** – Zentrale Business-Logik: Arbeitszeitberechnung mit Rundung, Pausenabzug, Schichtbeginn-Begrenzung, Feiertags-/Krankheits-/Urlaubs-Gutschriften, Monats- und Jahressaldierung.
- **Auth Middleware** – JWT-Tokens, bcrypt-gehashte Passwörter, rollenbasierte Zugriffskontrolle.
- **Embedded Vue.js SPA** – Frontend wird via `embed.FS` in die Go-Binary eingebettet.
- **SQLite** – Eine Datenbankdatei. Migration beim Start via goose.

### 1.3 Datenfluss

```
Fritz!Box (CSV per FTP)
        │
        ▼
  FTP-Importer ──► raw_punches (DB)
        │
        ▼
  Pairing-Logik ──► work_periods (DB)
        │
        ▼
  Berechnungs-Engine ──► Tages-/Monats-/Jahressaldo
        │
        ▼
  REST API ──► Vue.js SPA (Browser)
                  │
                  ▼
             Export (CSV / PDF)
```

---

## 2. Datenmodell

### 2.1 users

| Spalte | Typ | Beschreibung |
|---|---|---|
| id | INTEGER PK | Auto-Increment |
| username | TEXT UNIQUE | Login-Name |
| password_hash | TEXT | bcrypt-Hash |
| display_name | TEXT | Anzeigename |
| role | TEXT | `user`, `leitung`, `superadmin` |
| active | BOOLEAN | Aktiv/Inaktiv |
| created_at | TIMESTAMP | Erstellungszeitpunkt |
| updated_at | TIMESTAMP | Letzte Änderung |

### 2.2 nfc_tags

| Spalte | Typ | Beschreibung |
|---|---|---|
| id | INTEGER PK | Auto-Increment |
| tag_uid | TEXT UNIQUE | NFC-Tag-ID |
| user_id | INTEGER FK | Zugewiesener Mitarbeiter |
| assigned_from | DATE | Zuweisungsbeginn |
| assigned_until | DATE NULL | Zuweisungsende (NULL = aktiv) |

Ein NFC-Tag kann über die Zeit verschiedenen Mitarbeitern zugewiesen werden. NFC-Tags dürfen nur an User mit Rolle `user` zugewiesen werden (nicht an Leitung).

### 2.3 raw_punches

| Spalte | Typ | Beschreibung |
|---|---|---|
| id | INTEGER PK | Auto-Increment |
| punch_time | TIMESTAMP | Volle Millisekunden-Präzision |
| nfc_tag_uid | TEXT | NFC-Tag-ID aus CSV |
| source_file | TEXT | Quelldatei (Gerätename) |
| device_name | TEXT | Name des NFC-Lesegeräts |
| imported_at | TIMESTAMP | Import-Zeitpunkt |

**Unique Constraint:** `(punch_time, nfc_tag_uid)` für Duplikat-Erkennung.

Rohe Stempeldaten werden unveränderlich gespeichert (Audit-Trail).

### 2.4 work_periods

| Spalte | Typ | Beschreibung |
|---|---|---|
| id | INTEGER PK | Auto-Increment |
| user_id | INTEGER FK | Mitarbeiter |
| work_date | DATE | Arbeitstag |
| punch_in | TIMESTAMP | Kommen-Zeitpunkt |
| punch_out | TIMESTAMP NULL | Gehen-Zeitpunkt (NULL = noch anwesend) |
| is_break | BOOLEAN | Pause-Periode |
| source | TEXT | `imported` oder `manual` |

Abgeleitet aus raw_punches via Toggle-Logik. Werden bei jedem Import für betroffene User/Tage neu berechnet. Manuelle Einträge (`source = manual`) können durch Leitung/Superadmin direkt angelegt werden (z.B. wenn ein Mitarbeiter den NFC-Tag vergessen hat).

### 2.5 time_corrections

| Spalte | Typ | Beschreibung |
|---|---|---|
| id | INTEGER PK | Auto-Increment |
| work_period_id | INTEGER FK | Korrigiertes Work Period |
| corrected_in | TIMESTAMP | Korrigierter Kommen-Zeitpunkt |
| corrected_out | TIMESTAMP | Korrigierter Gehen-Zeitpunkt |
| reason | TEXT | Pflicht-Begründung |
| corrected_by | INTEGER FK | User der korrigiert hat |
| created_at | TIMESTAMP | Korrektur-Zeitpunkt |

Originalwerte bleiben in work_periods erhalten. Bei Berechnung gilt die letzte Korrektur. Mehrere Korrekturen pro work_period möglich. Wenn nur ein Zeitstempel korrigiert werden soll, wird der andere vom Original übernommen.

### 2.6 weekly_hours

| Spalte | Typ | Beschreibung |
|---|---|---|
| id | INTEGER PK | Auto-Increment |
| user_id | INTEGER FK | Mitarbeiter |
| hours_per_week | REAL | Wochenstunden (z.B. 40.0) |
| valid_from | DATE | Gültig ab |
| valid_until | DATE NULL | Gültig bis (NULL = aktuell) |

Erlaubt historische Änderungen der Wochenarbeitszeit. Der Tageswert ist `hours_per_week / 5`.

### 2.7 vacation_entitlements

| Spalte | Typ | Beschreibung |
|---|---|---|
| id | INTEGER PK | Auto-Increment |
| user_id | INTEGER FK | Mitarbeiter |
| days_per_year | REAL | Urlaubstage pro Jahr |
| valid_from | DATE | Gültig ab |
| valid_until | DATE NULL | Gültig bis (NULL = aktuell) |

### 2.8 schedules

| Spalte | Typ | Beschreibung |
|---|---|---|
| id | INTEGER PK | Auto-Increment |
| user_id | INTEGER FK | Mitarbeiter |
| schedule_date | DATE | Datum |
| shift_start | TIME | Schichtbeginn |
| shift_end | TIME | Schichtende |

Pro Person und Tag. Wird wöchentlich geplant. Kein Dienstplan-Eintrag = erfasste Zeit zählt trotzdem.

### 2.9 absences

| Spalte | Typ | Beschreibung |
|---|---|---|
| id | INTEGER PK | Auto-Increment |
| user_id | INTEGER FK | Mitarbeiter |
| absence_date | DATE | Datum |
| absence_type | TEXT | `sick`, `vacation`, `other` |
| half_day | BOOLEAN | Halber Tag |
| created_by | INTEGER FK | Eingetragen durch |
| created_at | TIMESTAMP | Erstellungszeitpunkt |

Halbe Urlaubstage möglich (0.5 Tage Abzug, 1/10 Wochenarbeitszeit Gutschrift).

### 2.10 holidays

| Spalte | Typ | Beschreibung |
|---|---|---|
| id | INTEGER PK | Auto-Increment |
| holiday_date | DATE UNIQUE | Datum |
| name | TEXT | Feiertagsname |
| auto_generated | BOOLEAN | Automatisch berechnet (NRW) |

### 2.11 closure_days

| Spalte | Typ | Beschreibung |
|---|---|---|
| id | INTEGER PK | Auto-Increment |
| closure_date | DATE UNIQUE | Datum |
| name | TEXT | Bezeichnung |
| created_by | INTEGER FK | Erstellt durch |

### 2.12 settings

| Spalte | Typ | Beschreibung |
|---|---|---|
| key | TEXT PK | Einstellungsname |
| value | TEXT | JSON-codierter Wert |

Globale Einstellungen: Rundung, Pausenregeln, FTP-Konfiguration.

---

## 3. Datenquelle und Import

### 3.1 CSV-Format

- Quelle: Fritz!Box NAS via FTP
- Eine CSV-Datei pro NFC-Lesegerät
- Zwei Spalten: Zeitstempel (Millisekunden-Präzision) + NFC-Tag-ID
- Trennzeichen: Konfigurierbar in `settings` (Default: Semikolon). Der Parser unterstützt Auto-Detection (Komma, Semikolon, Tab) als Fallback.

### 3.2 Import-Ablauf

1. FTP-Verbindung zur Fritz!Box aufbauen
2. Alle CSV-Dateien im konfigurierten Verzeichnis auflisten
3. Jede Datei lesen und parsen
4. Duplikat-Check über Unique Constraint `(punch_time, nfc_tag_uid)`
5. Neue Stempel in `raw_punches` einfügen
6. Betroffene User/Tage identifizieren
7. `work_periods` für betroffene User/Tage neu berechnen (nur `source = imported`; manuelle Einträge bleiben unverändert)

### 3.3 Trigger

- **Automatisch:** Konfigurierbares Intervall (Default: 300 Sekunden)
- **Manuell:** Button in der Web-UI (Leitung / Superadmin)
- **Import-Status:** Letzter erfolgreicher Import, Fehlermeldungen – über API abrufbar

### 3.4 Toggle-Logik (Stempel → Work Periods)

Alle raw_punches eines Users für einen Tag werden chronologisch sortiert:
- 1. Stempel = Kommen
- 2. Stempel = Gehen
- 3. Stempel = Kommen (Pause-Ende)
- 4. Stempel = Gehen
- usw.

Ergebnis:
- Work Period 1: Stempel 1 → Stempel 2 (Arbeit)
- Work Period 2: Stempel 2 → Stempel 3 (Pause, `is_break = true`)
- Work Period 3: Stempel 3 → Stempel 4 (Arbeit)
- Ungerade Anzahl: letzter Stempel offen (`punch_out = NULL`)

---

## 4. Berechnungs-Engine

### 4.1 Tagesberechnung

Für jeden Tag und Mitarbeiter:

1. **Abwesenheit prüfen:** Krankheit/Urlaub → Gutschrift 1/5 Wochenarbeitszeit (halber Tag: 1/10)
2. **Feiertag prüfen:** → Gutschrift 1/5 Wochenarbeitszeit
3. **Schließtag prüfen:** → Gutschrift 1/5 Wochenarbeitszeit
4. **Normaler Arbeitstag:**
   a. Work Periods laden (mit Korrekturen: letzte Korrektur überschreibt Original)
   b. Schichtbeginn anwenden: `effektiver_start = max(punch_in, shift_start)` (nur wenn Dienstplan existiert)
   c. Brutto-Arbeitszeit berechnen (Summe aller Nicht-Pausen-Perioden)
   d. Pausenabzug: gestempelte Pausen abziehen. Falls keine/zu kurze gestempelte Pause UND Brutto > konfigurierter Schwellwert → Differenz zum konfigurierten Mindestabzug
   e. Rundung: Netto-Arbeitszeit auf konfigurierte Einheit abrunden (z.B. 15 Min → 7h23m wird 7h15m)
   f. Ergebnis: Netto-Arbeitszeit des Tages

### 4.2 Zeitstempel-Verarbeitung

- raw_punches speichern Millisekunden-Präzision
- Bei Berechnung: Trunkierung auf Minutengenauigkeit (08:07:45.123 → 08:07)
- Rundung der Arbeitszeit ist ein separater Schritt (Abrundung auf konfigurierte Einheit)

### 4.3 Pausenregeln

Konfigurierbar durch Leitung/Superadmin, gespeichert in `settings`:

```json
{
  "break_rules": [
    {"min_work_hours": 6.0, "break_minutes": 30},
    {"min_work_hours": 9.0, "break_minutes": 45}
  ]
}
```

Logik: Der höchste zutreffende Schwellwert bestimmt die geforderte Gesamtpause. Gestempelte Pausen haben Vorrang. Auto-Abzug greift nur, wenn die Summe der gestempelten Pausen kürzer als die geforderte Gesamtpause ist – dann wird die Differenz abgezogen.

### 4.4 Monats-Saldierung

Arbeitstage = Montag bis Freitag, unabhängig von Feiertagen. Feiertage, Schließtage, Krankheit und Urlaub erzeugen Ist-Gutschriften, die das Soll ausgleichen.

```
Monats-Soll = Anzahl Werktage (Mo-Fr) × (Wochenarbeitszeit / 5)

Monats-Ist = Summe aller Tages-Netto-Arbeitszeiten
             (inkl. Gutschriften für Feiertage, Krankheit, Urlaub, Schließtage)

Monats-Saldo = Monats-Ist - Monats-Soll
Übertrag = Vormonats-Übertrag + Monats-Saldo
```

Rollierender Übertrag: Plus-/Minusstunden werden jeden Monat fortgeschrieben.

### 4.5 Urlaubs-Saldierung

```
Jahres-Urlaubsanspruch (aus vacation_entitlements)
- Genommene Urlaubstage (ganze: 1.0, halbe: 0.5)
= Rest-Urlaub

Vorjahresübertrag wird separat geführt.
```

### 4.6 Feiertags-Berechnung (NRW)

Automatische Berechnung beim Jahresstart oder auf Anforderung:

- Neujahr (01.01.)
- Karfreitag (Osterdatum - 2 Tage)
- Ostermontag (Osterdatum + 1 Tag)
- Tag der Arbeit (01.05.)
- Christi Himmelfahrt (Osterdatum + 39 Tage)
- Pfingstmontag (Osterdatum + 50 Tage)
- Fronleichnam (Osterdatum + 60 Tage)
- Tag der Deutschen Einheit (03.10.)
- Allerheiligen (01.11.)
- 1. Weihnachtstag (25.12.)
- 2. Weihnachtstag (26.12.)

Osterdatum via Gaußsche Osterformel. Auto-generierte Feiertage mit `auto_generated = true` markiert. Superadmin kann löschen oder manuell hinzufügen.

---

## 5. Rollen und Berechtigungen

### 5.1 Einfacher User

- Eigene Zeiterfassungen einsehen
- Eigene Stunden-Saldierung (Monat/Jahr)
- Eigene Urlaubstage einsehen
- Eigenen Dienstplan einsehen (nur lesen)

### 5.2 Leitung (mehrere User möglich)

Alles was User kann, plus:
- Wochenarbeitszeit pro Mitarbeiter und Zeitraum pflegen
- Urlaubsanspruch pro Mitarbeiter und Zeitraum pflegen
- Dienstplan erstellen und bearbeiten (pro Person, pro Woche)
- Krankheit und Urlaub eintragen
- Zeiten korrigieren (mit Begründung, Original bleibt erhalten)
- Mitarbeiter erstellen und deaktivieren
- NFC-Tags an User (nicht Leitung) zuweisen
- Schließtage verwalten
- FTP-Import manuell auslösen
- CSV- und PDF-Exporte erstellen
- Zeiten aller Mitarbeiter einsehen

### 5.3 Superadmin

Alles was Leitung kann, plus:
- Leitungs-User erstellen und verwalten
- Feiertage verwalten (auto-generierte NRW + manuell)
- Globale Einstellungen (Rundung, Pausenregeln, FTP-Konfiguration)

### 5.4 Authentifizierung

- Login: Benutzername + Passwort (bcrypt-Hash)
- JWT-Token bei Login (konfigurierbare Laufzeit, Default: 8h)
- Refresh-Token für Session-Verlängerung
- Erster Start: Superadmin-Account automatisch erstellt, zufälliges Passwort in Konsole ausgegeben, Passwortänderung beim ersten Login erzwungen
- Vorbereitung für spätere LDAP-Anbindung: Auth-Logik als Interface abstrahiert

---

## 6. REST API

Basis-Pfad: `/api/v1/`

### 6.1 Authentifizierung

```
POST /api/v1/auth/login
POST /api/v1/auth/refresh
POST /api/v1/auth/change-password
```

### 6.2 Eigene Daten (User)

```
GET /api/v1/me/times?from=...&to=...
GET /api/v1/me/balance?month=...&year=...
GET /api/v1/me/vacation
GET /api/v1/me/schedule?from=...&to=...
```

### 6.3 Mitarbeiter-Verwaltung (Leitung)

```
GET    /api/v1/employees
POST   /api/v1/employees
PATCH  /api/v1/employees/:id
GET    /api/v1/employees/:id/times?from=...&to=...
GET    /api/v1/employees/:id/balance?month=...&year=...
POST   /api/v1/employees/:id/work-periods    (manueller Eintrag)
DELETE /api/v1/employees/:id/work-periods/:wpId  (nur manuelle)
POST   /api/v1/employees/:id/corrections
GET    /api/v1/employees/:id/corrections
POST   /api/v1/employees/:id/absences
GET    /api/v1/employees/:id/absences
DELETE /api/v1/employees/:id/absences/:absenceId
PUT    /api/v1/employees/:id/weekly-hours
GET    /api/v1/employees/:id/weekly-hours
PUT    /api/v1/employees/:id/vacation-entitlement
GET    /api/v1/employees/:id/vacation-entitlement
POST   /api/v1/employees/:id/nfc-tags
GET    /api/v1/employees/:id/nfc-tags
```

### 6.4 Dienstplan (Leitung)

```
GET    /api/v1/schedules?week=...&year=...
POST   /api/v1/schedules
PUT    /api/v1/schedules/:id
DELETE /api/v1/schedules/:id
```

### 6.5 Abwesenheiten und Schließtage (Leitung)

```
GET    /api/v1/closure-days
POST   /api/v1/closure-days
DELETE /api/v1/closure-days/:id
```

### 6.6 Import (Leitung)

```
POST   /api/v1/import/trigger
GET    /api/v1/import/status
```

### 6.7 Administration (Superadmin)

```
GET    /api/v1/users
POST   /api/v1/users
PATCH  /api/v1/users/:id
GET    /api/v1/holidays?year=...
POST   /api/v1/holidays
DELETE /api/v1/holidays/:id
POST   /api/v1/holidays/generate?year=...
GET    /api/v1/settings
PUT    /api/v1/settings/:key
```

### 6.8 Export (Leitung + Superadmin)

```
GET /api/v1/export/csv?employee=...&from=...&to=...
GET /api/v1/export/pdf?employee=...&from=...&to=...
```

---

## 7. Web-UI

### 7.1 Technologie

Vue 3 + Composition API, Pinia (State), Vue Router, PrimeVue (Komponenten).

### 7.2 Seitenstruktur

**Alle Rollen:**
- `/login` – Login-Seite
- `/dashboard` – Persönliches Dashboard

**User-Ansichten:**
- `/my/times` – Eigene Zeiterfassungen (Tages-/Wochenansicht)
- `/my/balance` – Stunden-Saldierung (Monat/Jahr)
- `/my/vacation` – Urlaubsübersicht + Resttage
- `/my/schedule` – Eigener Dienstplan (nur lesen)

**Leitungs-Ansichten:**
- `/employees` – Mitarbeiterliste (aktiv/inaktiv)
- `/employees/:id` – Mitarbeiter-Detail (Zeiten, Saldo, Korrekturen)
- `/employees/:id/edit` – Stammdaten bearbeiten (Arbeitszeit, Urlaub, NFC)
- `/schedule` – Dienstplan-Editor (Wochenansicht, alle Mitarbeiter)
- `/absences` – Abwesenheiten verwalten
- `/corrections` – Zeitkorrekturen + manuelle Einträge (z.B. NFC-Tag vergessen)
- `/closure-days` – Schließtage
- `/import` – FTP-Import Status + manueller Trigger
- `/reports` – Auswertungen + Export

**Superadmin-Ansichten:**
- `/admin/users` – Alle User verwalten (inkl. Leitung erstellen)
- `/admin/holidays` – Feiertage verwalten
- `/admin/settings` – Globale Einstellungen

### 7.3 Schlüssel-UI-Elemente

- **Dashboard:** Tagesübersicht (aktuelle Stempel, heutiger Saldo, Resturlaub, nächste Schicht). Leitung sieht Anwesenheitsübersicht aller Mitarbeiter.
- **Dienstplan-Editor:** Wochenkalender. Zeilen = Mitarbeiter, Spalten = Wochentage. Schichtbeginn + Schichtende pro Zelle. Kopier-Funktion für wiederkehrende Pläne.
- **Zeitkorrekturen:** Original und Korrektur nebeneinander. Pflichtfeld Begründung. Audit-Log sichtbar.
- **Monatsübersicht:** Tageszeilen mit Soll/Ist/Saldo. Feiertage, Krankheit, Urlaub farblich markiert. Kumulierter Saldo.
- **Export:** Filter nach Mitarbeiter und Zeitraum. CSV für Weiterverarbeitung, PDF als druckbarer Monatsbericht.

---

## 8. Deployment und Konfiguration

### 8.1 Auslieferung

```
nfc-time-tracker-server(.exe)    # Single Binary
config.yaml                      # Konfigurationsdatei
data/                            # Automatisch erstellt
  └── timetracking.db            # SQLite-Datenbank
```

### 8.2 config.yaml

```yaml
server:
  port: 8080
  host: "0.0.0.0"
  tls:
    enabled: false
    cert_file: ""
    key_file: ""

database:
  path: "./data/timetracking.db"

ftp:
  host: "192.168.178.1"
  port: 21
  user: "ftpuser"
  password: "***"
  path: "/nfc-data/"
  interval_seconds: 300

auth:
  jwt_secret: "auto-generated-on-first-start"
  token_expiry_hours: 8

logging:
  level: "info"
  file: "./data/server.log"
```

Alle Werte auch per Umgebungsvariablen setzbar (z.B. `NFC_SERVER_PORT=8080`).

### 8.3 Erster Start

1. Datenbank automatisch erstellen + Migrationen ausführen
2. Superadmin-Account generieren (zufälliges Passwort in Konsole)
3. Feiertage für aktuelles + nächstes Jahr generieren
4. FTP-Import-Timer starten

### 8.4 Build

```bash
# Frontend bauen
cd web && npm run build && cd ..

# Windows
GOOS=windows GOARCH=amd64 go build -o nfc-time-tracker-server.exe ./cmd/server

# Linux
GOOS=linux GOARCH=amd64 go build -o nfc-time-tracker-server ./cmd/server
```

---

## 9. Projektstruktur

```
nfc-time-tracking-server/
├── cmd/
│   └── server/
│       └── main.go                 # Einstiegspunkt
├── internal/
│   ├── api/                        # HTTP-Handler & Routen
│   │   ├── router.go
│   │   ├── middleware/
│   │   │   └── auth.go
│   │   ├── handler/
│   │   │   ├── auth.go
│   │   │   ├── employees.go
│   │   │   ├── schedules.go
│   │   │   ├── absences.go
│   │   │   ├── corrections.go
│   │   │   ├── holidays.go
│   │   │   ├── settings.go
│   │   │   ├── import.go
│   │   │   └── export.go
│   │   └── response/
│   │       └── json.go
│   ├── model/                      # Datenmodelle (Structs)
│   │   ├── user.go
│   │   ├── punch.go
│   │   ├── workperiod.go
│   │   ├── schedule.go
│   │   ├── absence.go
│   │   ├── holiday.go
│   │   └── settings.go
│   ├── store/                      # Datenbank-Zugriff (Repository)
│   │   ├── store.go                # Interface-Definitionen
│   │   ├── sqlite/
│   │   │   ├── sqlite.go
│   │   │   ├── users.go
│   │   │   ├── punches.go
│   │   │   ├── workperiods.go
│   │   │   ├── schedules.go
│   │   │   ├── absences.go
│   │   │   ├── holidays.go
│   │   │   └── settings.go
│   │   └── migrations/             # SQL-Migrationen (goose)
│   │       └── *.sql
│   ├── service/                    # Business-Logik
│   │   ├── timecalc/              # Berechnungs-Engine
│   │   │   ├── daily.go
│   │   │   ├── balance.go
│   │   │   ├── rounding.go
│   │   │   ├── breaks.go
│   │   │   └── holidays.go
│   │   ├── importer/              # FTP-Import
│   │   │   ├── ftp.go
│   │   │   ├── csv.go
│   │   │   └── pairing.go
│   │   ├── auth/                  # Authentifizierung
│   │   │   └── auth.go
│   │   └── export/                # CSV/PDF-Export
│   │       ├── csv.go
│   │       └── pdf.go
│   └── config/                    # Konfiguration
│       └── config.go
├── web/                           # Vue.js Frontend
│   ├── src/
│   │   ├── main.ts
│   │   ├── router/
│   │   ├── stores/
│   │   ├── views/
│   │   ├── components/
│   │   └── api/
│   ├── package.json
│   └── vite.config.ts
├── embed.go                       # embed.FS für Frontend
├── config.yaml.example
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## 10. Offene Punkte / Spätere Erweiterungen

- **Direkte Android-API:** REST-API ist vorbereitet, Endpunkte für Stempel-Eingang können ergänzt werden
- **LDAP-Anbindung:** Auth-Interface erlaubt Austausch des Providers
- **Benachrichtigungen:** E-Mail-Benachrichtigungen bei fehlenden Stempeln
- **Mehrere Standorte/Bundesländer:** Aktuell nur NRW, erweiterbar
