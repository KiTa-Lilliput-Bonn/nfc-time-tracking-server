# NFC Time Tracking Server – Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a cross-platform Go server with embedded Vue.js SPA for NFC-based employee time tracking, importing data from a Fritz!Box via FTP.

**Architecture:** Go backend with chi router, SQLite (pure Go, no CGO) for persistence, and a Vue 3 + PrimeVue frontend embedded into a single binary via `embed.FS`. The system imports NFC punch timestamps from CSV files on a Fritz!Box NAS, pairs them into work periods, and computes balances with rounding, break rules, holiday credits, and rolling carryover.

**Tech Stack:** Go 1.22+, chi (router), modernc.org/sqlite, goose (migrations), golang-jwt/jwt, Vue 3 + Vite + PrimeVue + Pinia, jlaffaye/ftp, jung-kurt/gofpdf

**Spec:** `docs/superpowers/specs/2026-03-26-nfc-time-tracking-server-design.md`

---

## File Structure

```
nfc-time-tracking-server/
├── cmd/server/
│   └── main.go                           # Entrypoint: config load, DB init, server start
├── internal/
│   ├── config/config.go                  # YAML+env config struct & loader
│   ├── model/                            # Pure data structs (no DB deps)
│   │   ├── user.go                       # User, Role constants
│   │   ├── punch.go                      # RawPunch
│   │   ├── workperiod.go                 # WorkPeriod, TimeCorrection
│   │   ├── schedule.go                   # Schedule
│   │   ├── absence.go                    # Absence, AbsenceType constants
│   │   ├── holiday.go                    # Holiday, ClosureDay
│   │   ├── nfctag.go                     # NFCTag
│   │   ├── settings.go                   # Setting, BreakRule
│   │   └── balance.go                    # DayResult, MonthBalance, VacationBalance
│   ├── store/                            # Repository interfaces
│   │   ├── interfaces.go                 # All store interfaces
│   │   └── sqlite/                       # SQLite implementations
│   │       ├── db.go                     # Open, migrate, close
│   │       ├── users.go
│   │       ├── punches.go
│   │       ├── workperiods.go
│   │       ├── schedules.go
│   │       ├── absences.go
│   │       ├── holidays.go
│   │       ├── nfctags.go
│   │       ├── settings.go
│   │       └── migrations/
│   │           └── 001_initial_schema.sql
│   ├── service/
│   │   ├── auth/auth.go                  # Login, JWT issue/verify, password hashing
│   │   ├── importer/
│   │   │   ├── csv.go                    # CSV parser with delimiter auto-detection
│   │   │   ├── pairing.go               # Toggle-logic: punches → work_periods
│   │   │   └── ftp.go                   # FTP client, scheduled + manual import
│   │   ├── timecalc/
│   │   │   ├── daily.go                  # Single-day net work time calculation
│   │   │   ├── breaks.go                # Break deduction logic
│   │   │   ├── rounding.go              # Floor-rounding to configured unit
│   │   │   ├── holidays.go             # Gauss Easter formula, NRW holidays
│   │   │   └── balance.go               # Monthly/yearly hour + vacation balance
│   │   └── export/
│   │       ├── csv.go                    # CSV export
│   │       └── pdf.go                    # PDF monthly report
│   ├── web/                              # Embedded frontend
│   │   ├── embed.go                      # Production: embed.FS for web/dist
│   │   ├── embed_dev.go                  # Development: os.DirFS for web/dist
│   │   └── dist/                         # Copied from web/dist by Makefile
│   └── api/
│       ├── router.go                     # chi router setup, route registration
│       ├── middleware/auth.go            # JWT middleware, role check
│       ├── response/json.go             # JSON response helpers
│       └── handler/
│           ├── auth.go                   # POST login, refresh, change-password
│           ├── me.go                     # GET /me/* (own data)
│           ├── employees.go              # CRUD employees, times, corrections, absences
│           ├── schedules.go              # CRUD schedules
│           ├── holidays.go              # CRUD holidays, generate
│           ├── closuredays.go           # CRUD closure days
│           ├── settings.go              # GET/PUT settings
│           ├── importhandler.go         # POST trigger, GET status
│           └── export.go                # GET csv/pdf
├── web/                                  # Vue 3 frontend
│   ├── package.json
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── index.html
│   └── src/
│       ├── main.ts
│       ├── App.vue
│       ├── api/client.ts                 # Axios instance with JWT interceptor
│       ├── router/index.ts               # Vue Router with role guards
│       ├── stores/
│       │   ├── auth.ts                   # Auth store (login, token, user)
│       │   └── app.ts                    # General app state
│       ├── layouts/
│       │   ├── AppLayout.vue             # Main layout with sidebar
│       │   └── AuthLayout.vue            # Login layout (no sidebar)
│       ├── views/
│       │   ├── LoginView.vue
│       │   ├── DashboardView.vue
│       │   ├── my/
│       │   │   ├── MyTimesView.vue
│       │   │   ├── MyBalanceView.vue
│       │   │   ├── MyVacationView.vue
│       │   │   └── MyScheduleView.vue
│       │   ├── employees/
│       │   │   ├── EmployeeListView.vue
│       │   │   ├── EmployeeDetailView.vue
│       │   │   └── EmployeeEditView.vue
│       │   ├── schedule/ScheduleEditorView.vue
│       │   ├── absences/AbsencesView.vue
│       │   ├── corrections/CorrectionsView.vue
│       │   ├── closuredays/ClosureDaysView.vue
│       │   ├── import/ImportView.vue
│       │   ├── reports/ReportsView.vue
│       │   └── admin/
│       │       ├── UsersView.vue
│       │       ├── HolidaysView.vue
│       │       └── SettingsView.vue
│       └── components/
│           ├── TimeTable.vue             # Reusable time entries table
│           ├── BalanceCard.vue           # Month/year balance display
│           └── ScheduleGrid.vue          # Week schedule grid editor
├── config.yaml.example
├── Makefile
├── go.mod
└── go.sum
```

---

## Task 1: Project Scaffolding & Configuration

**Files:**
- Create: `go.mod`
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `config.yaml.example`
- Create: `cmd/server/main.go` (minimal placeholder)
- Create: `Makefile`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/Apps.Associates/nfc-time-tracking-server
go mod init nfc-time-tracking-server
```

- [ ] **Step 2: Install core dependencies**

```bash
go get github.com/go-chi/chi/v5
go get modernc.org/sqlite
go get github.com/pressly/goose/v3
go get github.com/golang-jwt/jwt/v5
go get golang.org/x/crypto/bcrypt
go get gopkg.in/yaml.v3
go get github.com/jlaffaye/ftp
go get github.com/jung-kurt/gofpdf
```

- [ ] **Step 3: Write config test**

Create `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromFile(t *testing.T) {
	content := []byte(`
server:
  port: 9090
  host: "127.0.0.1"
database:
  path: "./test.db"
ftp:
  host: "192.168.1.1"
  port: 21
  user: "admin"
  password: "secret"
  path: "/data/"
  interval_seconds: 60
auth:
  jwt_secret: "testsecret"
  token_expiry_hours: 4
logging:
  level: "debug"
  file: "./test.log"
`)
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yaml")
	os.WriteFile(f, content, 0644)

	cfg, err := Load(f)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.FTP.Host != "192.168.1.1" {
		t.Errorf("expected FTP host 192.168.1.1, got %s", cfg.FTP.Host)
	}
	if cfg.Auth.TokenExpiryHours != 4 {
		t.Errorf("expected expiry 4h, got %d", cfg.Auth.TokenExpiryHours)
	}
}

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Database.Path != "./data/timetracking.db" {
		t.Errorf("expected default db path, got %s", cfg.Database.Path)
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv("NFC_SERVER_PORT", "3000")
	t.Setenv("NFC_FTP_HOST", "10.0.0.1")
	cfg := Defaults()
	cfg.ApplyEnv()
	if cfg.Server.Port != 3000 {
		t.Errorf("expected port 3000 from env, got %d", cfg.Server.Port)
	}
	if cfg.FTP.Host != "10.0.0.1" {
		t.Errorf("expected FTP host from env, got %s", cfg.FTP.Host)
	}
}
```

- [ ] **Step 4: Run test – verify it fails**

```bash
go test ./internal/config/ -v
```

Expected: compilation error (package doesn't exist yet).

- [ ] **Step 5: Implement config**

Create `internal/config/config.go`:

```go
package config

import (
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	FTP      FTPConfig      `yaml:"ftp"`
	Auth     AuthConfig     `yaml:"auth"`
	Logging  LoggingConfig  `yaml:"logging"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
	TLS  struct {
		Enabled  bool   `yaml:"enabled"`
		CertFile string `yaml:"cert_file"`
		KeyFile  string `yaml:"key_file"`
	} `yaml:"tls"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type FTPConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	User            string `yaml:"user"`
	Password        string `yaml:"password"`
	Path            string `yaml:"path"`
	IntervalSeconds int    `yaml:"interval_seconds"`
}

type AuthConfig struct {
	JWTSecret        string `yaml:"jwt_secret"`
	TokenExpiryHours int    `yaml:"token_expiry_hours"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

func Defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 8080,
			Host: "0.0.0.0",
		},
		Database: DatabaseConfig{
			Path: "./data/timetracking.db",
		},
		FTP: FTPConfig{
			Port:            21,
			IntervalSeconds: 300,
		},
		Auth: AuthConfig{
			TokenExpiryHours: 8,
		},
		Logging: LoggingConfig{
			Level: "info",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) ApplyEnv() {
	if v := os.Getenv("NFC_SERVER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.Server.Port = p
		}
	}
	if v := os.Getenv("NFC_SERVER_HOST"); v != "" {
		c.Server.Host = v
	}
	if v := os.Getenv("NFC_DATABASE_PATH"); v != "" {
		c.Database.Path = v
	}
	if v := os.Getenv("NFC_FTP_HOST"); v != "" {
		c.FTP.Host = v
	}
	if v := os.Getenv("NFC_FTP_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.FTP.Port = p
		}
	}
	if v := os.Getenv("NFC_FTP_USER"); v != "" {
		c.FTP.User = v
	}
	if v := os.Getenv("NFC_FTP_PASSWORD"); v != "" {
		c.FTP.Password = v
	}
	if v := os.Getenv("NFC_FTP_PATH"); v != "" {
		c.FTP.Path = v
	}
	if v := os.Getenv("NFC_AUTH_JWT_SECRET"); v != "" {
		c.Auth.JWTSecret = v
	}
	if v := os.Getenv("NFC_AUTH_EXPIRY_HOURS"); v != "" {
		if h, err := strconv.Atoi(v); err == nil {
			c.Auth.TokenExpiryHours = h
		}
	}
}
```

- [ ] **Step 6: Run test – verify it passes**

```bash
go test ./internal/config/ -v
```

Expected: 3 tests PASS.

- [ ] **Step 7: Create config.yaml.example**

Create `config.yaml.example` with all documented fields (copy from spec section 8.2).

- [ ] **Step 8: Create minimal main.go**

Create `cmd/server/main.go`:

```go
package main

import (
	"fmt"
	"log"
	"os"

	"nfc-time-tracking-server/internal/config"
)

func main() {
	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Printf("No config file at %s, using defaults: %v", cfgPath, err)
		cfg = config.Defaults()
	}
	cfg.ApplyEnv()

	fmt.Printf("NFC Time Tracking Server starting on %s:%d\n", cfg.Server.Host, cfg.Server.Port)
}
```

- [ ] **Step 9: Create Makefile**

Create `Makefile`:

```makefile
.PHONY: build-frontend build-linux build-windows build test run clean

build-frontend:
	cd web && npm ci && npm run build

build-linux: build-frontend
	GOOS=linux GOARCH=amd64 go build -o bin/nfc-time-tracker-server ./cmd/server

build-windows: build-frontend
	GOOS=windows GOARCH=amd64 go build -o bin/nfc-time-tracker-server.exe ./cmd/server

build: build-frontend
	go build -o bin/nfc-time-tracker-server ./cmd/server

test:
	go test ./... -v

run:
	go run ./cmd/server

clean:
	rm -rf bin/ web/dist/
```

- [ ] **Step 10: Create .gitignore**

Create `.gitignore`:

```
bin/
web/node_modules/
web/dist/
data/
*.db
config.yaml
```

- [ ] **Step 11: Verify build compiles**

```bash
go build ./cmd/server
```

Expected: compiles without errors.

- [ ] **Step 12: Commit**

```bash
git add -A
git commit -m "feat: project scaffolding with config, main entrypoint, Makefile"
```

---

## Task 2: Data Models

**Files:**
- Create: `internal/model/user.go`
- Create: `internal/model/punch.go`
- Create: `internal/model/workperiod.go`
- Create: `internal/model/schedule.go`
- Create: `internal/model/absence.go`
- Create: `internal/model/holiday.go`
- Create: `internal/model/nfctag.go`
- Create: `internal/model/settings.go`
- Create: `internal/model/balance.go`

- [ ] **Step 1: Create user.go**

```go
package model

import "time"

type Role string

const (
	RoleUser       Role = "user"
	RoleLeitung    Role = "leitung"
	RoleSuperadmin Role = "superadmin"
)

type User struct {
	ID                 int       `json:"id"`
	Username           string    `json:"username"`
	PasswordHash       string    `json:"-"`
	DisplayName        string    `json:"display_name"`
	Role               Role      `json:"role"`
	Active             bool      `json:"active"`
	MustChangePassword bool      `json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
```

- [ ] **Step 2: Create punch.go**

```go
package model

import "time"

type RawPunch struct {
	ID         int       `json:"id"`
	PunchTime  time.Time `json:"punch_time"`
	NFCTagUID  string    `json:"nfc_tag_uid"`
	SourceFile string    `json:"source_file"`
	DeviceName string    `json:"device_name"`
	ImportedAt time.Time `json:"imported_at"`
}
```

- [ ] **Step 3: Create workperiod.go**

```go
package model

import "time"

type WorkPeriod struct {
	ID       int        `json:"id"`
	UserID   int        `json:"user_id"`
	WorkDate string     `json:"work_date"`
	PunchIn  time.Time  `json:"punch_in"`
	PunchOut *time.Time `json:"punch_out"`
	IsBreak  bool       `json:"is_break"`
	Source   string     `json:"source"`
}

type TimeCorrection struct {
	ID           int       `json:"id"`
	WorkPeriodID int       `json:"work_period_id"`
	CorrectedIn  time.Time `json:"corrected_in"`
	CorrectedOut time.Time `json:"corrected_out"`
	Reason       string    `json:"reason"`
	CorrectedBy  int       `json:"corrected_by"`
	CreatedAt    time.Time `json:"created_at"`
}
```

- [ ] **Step 4: Create schedule.go**

```go
package model

type Schedule struct {
	ID           int    `json:"id"`
	UserID       int    `json:"user_id"`
	ScheduleDate string `json:"schedule_date"`
	ShiftStart   string `json:"shift_start"`
	ShiftEnd     string `json:"shift_end"`
}
```

- [ ] **Step 5: Create absence.go**

```go
package model

import "time"

type AbsenceType string

const (
	AbsenceSick     AbsenceType = "sick"
	AbsenceVacation AbsenceType = "vacation"
	AbsenceOther    AbsenceType = "other"
)

type Absence struct {
	ID          int         `json:"id"`
	UserID      int         `json:"user_id"`
	AbsenceDate string      `json:"absence_date"`
	AbsenceType AbsenceType `json:"absence_type"`
	HalfDay     bool        `json:"half_day"`
	CreatedBy   int         `json:"created_by"`
	CreatedAt   time.Time   `json:"created_at"`
}
```

- [ ] **Step 6: Create holiday.go**

```go
package model

type Holiday struct {
	ID            int    `json:"id"`
	HolidayDate   string `json:"holiday_date"`
	Name          string `json:"name"`
	AutoGenerated bool   `json:"auto_generated"`
}

type ClosureDay struct {
	ID          int    `json:"id"`
	ClosureDate string `json:"closure_date"`
	Name        string `json:"name"`
	CreatedBy   int    `json:"created_by"`
}
```

- [ ] **Step 7: Create nfctag.go**

```go
package model

type NFCTag struct {
	ID            int     `json:"id"`
	TagUID        string  `json:"tag_uid"`
	UserID        int     `json:"user_id"`
	AssignedFrom  string  `json:"assigned_from"`
	AssignedUntil *string `json:"assigned_until"`
}
```

- [ ] **Step 8: Create settings.go**

```go
package model

type Setting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type BreakRule struct {
	MinWorkHours float64 `json:"min_work_hours"`
	BreakMinutes int     `json:"break_minutes"`
}

type WeeklyHours struct {
	ID           int     `json:"id"`
	UserID       int     `json:"user_id"`
	HoursPerWeek float64 `json:"hours_per_week"`
	ValidFrom    string  `json:"valid_from"`
	ValidUntil   *string `json:"valid_until"`
}

type VacationEntitlement struct {
	ID          int     `json:"id"`
	UserID      int     `json:"user_id"`
	DaysPerYear float64 `json:"days_per_year"`
	ValidFrom   string  `json:"valid_from"`
	ValidUntil  *string `json:"valid_until"`
}
```

- [ ] **Step 9: Create balance.go**

```go
package model

type DayResult struct {
	Date         string  `json:"date"`
	NetWorkHours float64 `json:"net_work_hours"`
	TargetHours  float64 `json:"target_hours"`
	IsHoliday    bool    `json:"is_holiday"`
	IsClosureDay bool    `json:"is_closure_day"`
	AbsenceType  *string `json:"absence_type"`
	HalfDay      bool    `json:"half_day"`
	IsWeekend    bool    `json:"is_weekend"`
}

type MonthBalance struct {
	Year         int     `json:"year"`
	Month        int     `json:"month"`
	WorkedHours  float64 `json:"worked_hours"`
	TargetHours  float64 `json:"target_hours"`
	BalanceHours float64 `json:"balance_hours"`
	Carryover    float64 `json:"carryover"`
	TotalBalance float64 `json:"total_balance"`
}

type VacationBalance struct {
	Year           int     `json:"year"`
	Entitlement    float64 `json:"entitlement"`
	Taken          float64 `json:"taken"`
	Remaining      float64 `json:"remaining"`
	CarriedOver    float64 `json:"carried_over"`
}
```

- [ ] **Step 10: Verify compilation**

```bash
go build ./internal/model/
```

Expected: compiles without errors.

- [ ] **Step 11: Commit**

```bash
git add -A
git commit -m "feat: add all data model structs"
```

---

## Task 3: Database Layer – Schema & Connection

**Files:**
- Create: `internal/store/interfaces.go`
- Create: `internal/store/sqlite/db.go`
- Create: `internal/store/sqlite/db_test.go`
- Create: `internal/store/sqlite/migrations/001_initial_schema.sql`

- [ ] **Step 1: Create migration SQL**

Create `internal/store/sqlite/migrations/001_initial_schema.sql`:

```sql
-- +goose Up
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name TEXT NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('user', 'leitung', 'superadmin')),
    active BOOLEAN NOT NULL DEFAULT 1,
    must_change_password BOOLEAN NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE nfc_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tag_uid TEXT NOT NULL,
    user_id INTEGER NOT NULL REFERENCES users(id),
    assigned_from DATE NOT NULL,
    assigned_until DATE,
    UNIQUE(tag_uid, assigned_from)
);

CREATE TABLE raw_punches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    punch_time TIMESTAMP NOT NULL,
    nfc_tag_uid TEXT NOT NULL,
    source_file TEXT NOT NULL,
    device_name TEXT NOT NULL DEFAULT '',
    imported_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(punch_time, nfc_tag_uid)
);

CREATE TABLE work_periods (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    work_date DATE NOT NULL,
    punch_in TIMESTAMP NOT NULL,
    punch_out TIMESTAMP,
    is_break BOOLEAN NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT 'imported' CHECK(source IN ('imported', 'manual'))
);
CREATE INDEX idx_work_periods_user_date ON work_periods(user_id, work_date);

CREATE TABLE time_corrections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    work_period_id INTEGER NOT NULL REFERENCES work_periods(id),
    corrected_in TIMESTAMP NOT NULL,
    corrected_out TIMESTAMP NOT NULL,
    reason TEXT NOT NULL,
    corrected_by INTEGER NOT NULL REFERENCES users(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE weekly_hours (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    hours_per_week REAL NOT NULL,
    valid_from DATE NOT NULL,
    valid_until DATE
);
CREATE INDEX idx_weekly_hours_user ON weekly_hours(user_id);

CREATE TABLE vacation_entitlements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    days_per_year REAL NOT NULL,
    valid_from DATE NOT NULL,
    valid_until DATE
);
CREATE INDEX idx_vacation_ent_user ON vacation_entitlements(user_id);

CREATE TABLE schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    schedule_date DATE NOT NULL,
    shift_start TEXT NOT NULL,
    shift_end TEXT NOT NULL,
    UNIQUE(user_id, schedule_date)
);

CREATE TABLE absences (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    absence_date DATE NOT NULL,
    absence_type TEXT NOT NULL CHECK(absence_type IN ('sick', 'vacation', 'other')),
    half_day BOOLEAN NOT NULL DEFAULT 0,
    created_by INTEGER NOT NULL REFERENCES users(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, absence_date)
);

CREATE TABLE holidays (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    holiday_date DATE NOT NULL UNIQUE,
    name TEXT NOT NULL,
    auto_generated BOOLEAN NOT NULL DEFAULT 0
);

CREATE TABLE closure_days (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    closure_date DATE NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_by INTEGER NOT NULL REFERENCES users(id)
);

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Seed default settings
INSERT INTO settings (key, value) VALUES ('rounding_minutes', '15');
INSERT INTO settings (key, value) VALUES ('break_rules', '[{"min_work_hours":6.0,"break_minutes":30},{"min_work_hours":9.0,"break_minutes":45}]');
INSERT INTO settings (key, value) VALUES ('csv_delimiter', ';');

-- +goose Down
DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS closure_days;
DROP TABLE IF EXISTS holidays;
DROP TABLE IF EXISTS absences;
DROP TABLE IF EXISTS schedules;
DROP TABLE IF EXISTS vacation_entitlements;
DROP TABLE IF EXISTS weekly_hours;
DROP TABLE IF EXISTS time_corrections;
DROP TABLE IF EXISTS work_periods;
DROP TABLE IF EXISTS raw_punches;
DROP TABLE IF EXISTS nfc_tags;
DROP TABLE IF EXISTS users;
```

- [ ] **Step 2: Write DB connection test**

Create `internal/store/sqlite/db_test.go`:

```go
package sqlite

import (
	"testing"
)

func TestOpenAndMigrate(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Verify tables exist by querying sqlite_master
	rows, err := db.DB.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		tables = append(tables, name)
	}

	expected := []string{"absences", "closure_days", "holidays", "nfc_tags",
		"raw_punches", "schedules", "settings", "time_corrections",
		"users", "vacation_entitlements", "weekly_hours", "work_periods"}

	for _, exp := range expected {
		found := false
		for _, t2 := range tables {
			if t2 == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected table %q not found in %v", exp, tables)
		}
	}
}

func TestDefaultSettings(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	var val string
	err = db.DB.QueryRow("SELECT value FROM settings WHERE key = 'rounding_minutes'").Scan(&val)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if val != "15" {
		t.Errorf("expected rounding_minutes=15, got %s", val)
	}
}
```

- [ ] **Step 3: Run test – verify it fails**

```bash
go test ./internal/store/sqlite/ -v
```

Expected: compilation error.

- [ ] **Step 4: Implement DB connection & migration**

Create `internal/store/sqlite/db.go`:

```go
package sqlite

import (
	"database/sql"
	"embed"
	"fmt"

	_ "modernc.org/sqlite"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

type DB struct {
	DB *sql.DB
}

func Open(dsn string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Enable WAL mode and foreign keys
	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := sqlDB.Exec("PRAGMA foreign_keys=ON"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &DB{DB: sqlDB}, nil
}

func (d *DB) Close() error {
	return d.DB.Close()
}
```

- [ ] **Step 5: Run test – verify it passes**

```bash
go test ./internal/store/sqlite/ -v
```

Expected: 2 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: SQLite database layer with migrations and schema"
```

---

## Task 4: Store Interfaces & User Repository

**Files:**
- Create: `internal/store/interfaces.go`
- Create: `internal/store/sqlite/users.go`
- Create: `internal/store/sqlite/users_test.go`

- [ ] **Step 1: Define store interfaces**

Create `internal/store/interfaces.go` with all repository interfaces:

```go
package store

import (
	"context"
	"time"

	"nfc-time-tracking-server/internal/model"
)

type UserStore interface {
	Create(ctx context.Context, u *model.User) error
	GetByID(ctx context.Context, id int) (*model.User, error)
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	List(ctx context.Context, activeOnly bool) ([]model.User, error)
	Update(ctx context.Context, u *model.User) error
}

type NFCTagStore interface {
	Assign(ctx context.Context, tag *model.NFCTag) error
	Unassign(ctx context.Context, tagUID string, until string) error
	GetActiveByTagUID(ctx context.Context, tagUID string) (*model.NFCTag, error)
	ListByUser(ctx context.Context, userID int) ([]model.NFCTag, error)
	ResolveUserID(ctx context.Context, tagUID string, at time.Time) (int, error)
}

type PunchStore interface {
	InsertBatch(ctx context.Context, punches []model.RawPunch) (int, error)
	ListByUserAndDate(ctx context.Context, userID int, date string) ([]model.RawPunch, error)
}

type WorkPeriodStore interface {
	ReplaceForUserDate(ctx context.Context, userID int, date string, periods []model.WorkPeriod) error
	ListByUserDateRange(ctx context.Context, userID int, from, to string) ([]model.WorkPeriod, error)
	CreateManual(ctx context.Context, wp *model.WorkPeriod) error
	DeleteManual(ctx context.Context, id int) error
}

type CorrectionStore interface {
	Create(ctx context.Context, c *model.TimeCorrection) error
	GetLatestForPeriod(ctx context.Context, workPeriodID int) (*model.TimeCorrection, error)
	ListByUser(ctx context.Context, userID int, from, to string) ([]model.TimeCorrection, error)
}

type WeeklyHoursStore interface {
	Set(ctx context.Context, wh *model.WeeklyHours) error
	GetForDate(ctx context.Context, userID int, date string) (*model.WeeklyHours, error)
	ListByUser(ctx context.Context, userID int) ([]model.WeeklyHours, error)
}

type VacationEntitlementStore interface {
	Set(ctx context.Context, ve *model.VacationEntitlement) error
	GetForDate(ctx context.Context, userID int, date string) (*model.VacationEntitlement, error)
	ListByUser(ctx context.Context, userID int) ([]model.VacationEntitlement, error)
}

type ScheduleStore interface {
	Set(ctx context.Context, s *model.Schedule) error
	GetForUserDate(ctx context.Context, userID int, date string) (*model.Schedule, error)
	ListByWeek(ctx context.Context, year, week int) ([]model.Schedule, error)
	Delete(ctx context.Context, id int) error
}

type AbsenceStore interface {
	Create(ctx context.Context, a *model.Absence) error
	Delete(ctx context.Context, id int) error
	GetForUserDate(ctx context.Context, userID int, date string) (*model.Absence, error)
	ListByUserDateRange(ctx context.Context, userID int, from, to string) ([]model.Absence, error)
}

type HolidayStore interface {
	Create(ctx context.Context, h *model.Holiday) error
	Delete(ctx context.Context, id int) error
	ListByYear(ctx context.Context, year int) ([]model.Holiday, error)
	GetForDate(ctx context.Context, date string) (*model.Holiday, error)
	DeleteAutoGenerated(ctx context.Context, year int) error
}

type ClosureDayStore interface {
	Create(ctx context.Context, c *model.ClosureDay) error
	Delete(ctx context.Context, id int) error
	List(ctx context.Context) ([]model.ClosureDay, error)
	GetForDate(ctx context.Context, date string) (*model.ClosureDay, error)
}

type SettingsStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	GetAll(ctx context.Context) ([]model.Setting, error)
}
```

- [ ] **Step 2: Write user store test**

Create `internal/store/sqlite/users_test.go`:

```go
package sqlite

import (
	"context"
	"testing"

	"nfc-time-tracking-server/internal/model"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUserStore_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	s := NewUserStore(db)
	ctx := context.Background()

	u := &model.User{
		Username:     "testuser",
		PasswordHash: "$2a$10$fakehash",
		DisplayName:  "Test User",
		Role:         model.RoleUser,
		Active:       true,
	}
	if err := s.Create(ctx, u); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if u.ID == 0 {
		t.Error("expected ID to be set after create")
	}

	got, err := s.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Username != "testuser" {
		t.Errorf("expected username testuser, got %s", got.Username)
	}

	got2, err := s.GetByUsername(ctx, "testuser")
	if err != nil {
		t.Fatalf("GetByUsername failed: %v", err)
	}
	if got2.ID != u.ID {
		t.Errorf("expected ID %d, got %d", u.ID, got2.ID)
	}
}

func TestUserStore_List(t *testing.T) {
	db := setupTestDB(t)
	s := NewUserStore(db)
	ctx := context.Background()

	s.Create(ctx, &model.User{Username: "active", PasswordHash: "x", DisplayName: "A", Role: model.RoleUser, Active: true})
	s.Create(ctx, &model.User{Username: "inactive", PasswordHash: "x", DisplayName: "B", Role: model.RoleUser, Active: false})

	all, _ := s.List(ctx, false)
	if len(all) != 2 {
		t.Errorf("expected 2 users, got %d", len(all))
	}

	active, _ := s.List(ctx, true)
	if len(active) != 1 {
		t.Errorf("expected 1 active user, got %d", len(active))
	}
}

func TestUserStore_Update(t *testing.T) {
	db := setupTestDB(t)
	s := NewUserStore(db)
	ctx := context.Background()

	u := &model.User{Username: "updatable", PasswordHash: "x", DisplayName: "Before", Role: model.RoleUser, Active: true}
	s.Create(ctx, u)

	u.DisplayName = "After"
	u.Active = false
	if err := s.Update(ctx, u); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, _ := s.GetByID(ctx, u.ID)
	if got.DisplayName != "After" {
		t.Errorf("expected After, got %s", got.DisplayName)
	}
	if got.Active {
		t.Error("expected inactive")
	}
}
```

- [ ] **Step 3: Run test – verify it fails**

```bash
go test ./internal/store/sqlite/ -run TestUserStore -v
```

Expected: compilation error (NewUserStore not defined).

- [ ] **Step 4: Implement user store**

Create `internal/store/sqlite/users.go`:

```go
package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"nfc-time-tracking-server/internal/model"
)

type UserStore struct {
	db *DB
}

func NewUserStore(db *DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) Create(ctx context.Context, u *model.User) error {
	res, err := s.db.DB.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, display_name, role, active, must_change_password) VALUES (?, ?, ?, ?, ?, ?)`,
		u.Username, u.PasswordHash, u.DisplayName, u.Role, u.Active, u.MustChangePassword)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	id, _ := res.LastInsertId()
	u.ID = int(id)
	return nil
}

func (s *UserStore) GetByID(ctx context.Context, id int) (*model.User, error) {
	u := &model.User{}
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, username, password_hash, display_name, role, active, must_change_password, created_at, updated_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Role, &u.Active, &u.MustChangePassword, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %d", id)
	}
	return u, err
}

func (s *UserStore) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	u := &model.User{}
	err := s.db.DB.QueryRowContext(ctx,
		`SELECT id, username, password_hash, display_name, role, active, must_change_password, created_at, updated_at FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Role, &u.Active, &u.MustChangePassword, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	return u, err
}

func (s *UserStore) List(ctx context.Context, activeOnly bool) ([]model.User, error) {
	query := `SELECT id, username, password_hash, display_name, role, active, must_change_password, created_at, updated_at FROM users`
	if activeOnly {
		query += ` WHERE active = 1`
	}
	query += ` ORDER BY display_name`

	rows, err := s.db.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Role, &u.Active, &u.MustChangePassword, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (s *UserStore) Update(ctx context.Context, u *model.User) error {
	_, err := s.db.DB.ExecContext(ctx,
		`UPDATE users SET display_name = ?, role = ?, active = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		u.DisplayName, u.Role, u.Active, u.ID)
	return err
}
```

- [ ] **Step 5: Run test – verify it passes**

```bash
go test ./internal/store/sqlite/ -run TestUserStore -v
```

Expected: all TestUserStore tests PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: store interfaces and SQLite user repository"
```

---

## Task 5: Remaining Store Implementations

**Files:**
- Create: `internal/store/sqlite/punches.go`
- Create: `internal/store/sqlite/workperiods.go`
- Create: `internal/store/sqlite/schedules.go`
- Create: `internal/store/sqlite/absences.go`
- Create: `internal/store/sqlite/holidays.go`
- Create: `internal/store/sqlite/nfctags.go`
- Create: `internal/store/sqlite/settings.go`
- Create: `internal/store/sqlite/stores_test.go`

Each store follows the same pattern as UserStore. Implement all remaining stores with corresponding tests.

- [ ] **Step 1: Write tests for all remaining stores**

Create `internal/store/sqlite/stores_test.go` covering:
- `TestPunchStore_InsertBatchAndDedup` – Insert punches, insert same again, verify count unchanged
- `TestNFCTagStore_AssignAndResolve` – Assign tag, resolve user by tag UID + time
- `TestWorkPeriodStore_ReplaceForUserDate` – Replace imported periods, verify manual preserved
- `TestScheduleStore_SetAndGet` – Set schedule, get by user+date
- `TestAbsenceStore_CRUD` – Create, get, delete absence
- `TestHolidayStore_CRUD` – Create, list by year, delete
- `TestClosureDayStore_CRUD` – Create, list, delete
- `TestSettingsStore_GetAndSet` – Get default, set new value, get again
- `TestWeeklyHoursStore_SetAndGetForDate` – Set multiple periods, query by date
- `TestVacationEntitlementStore_SetAndGetForDate`
- `TestCorrectionStore_CreateAndGetLatest` – Multiple corrections, latest returned

- [ ] **Step 2: Run tests – verify they fail**

```bash
go test ./internal/store/sqlite/ -run "TestPunch|TestNFC|TestWorkPeriod|TestSchedule|TestAbsence|TestHoliday|TestClosure|TestSettings|TestWeekly|TestVacation|TestCorrection" -v
```

- [ ] **Step 3: Implement punches.go**

`NewPunchStore`, `InsertBatch` (INSERT OR IGNORE for dedup), `ListByUserAndDate` (join via nfc_tags to resolve user).

- [ ] **Step 4: Implement nfctags.go**

`NewNFCTagStore`, `Assign` (validates user role = `user` before assigning – rejects leitung/superadmin), `Unassign` (sets assigned_until), `GetActiveByTagUID`, `ListByUser`, `ResolveUserID` (finds user for tag UID at a given time using assigned_from/until range). The role check requires a JOIN with users table or a separate query before insert.

- [ ] **Step 5: Implement workperiods.go**

`NewWorkPeriodStore`, `ReplaceForUserDate` (DELETE WHERE source='imported' then INSERT), `ListByUserDateRange`, `CreateManual`, `DeleteManual` (only source='manual').

- [ ] **Step 6: Implement schedules.go, absences.go, holidays.go, settings.go**

Each store: constructor + methods matching the interface. Use INSERT OR REPLACE for schedule's unique constraint. Settings uses INSERT OR REPLACE.

- [ ] **Step 7: Implement remaining: closuredays, weekly_hours, vacation_entitlements, corrections**

WeeklyHoursStore.GetForDate: `WHERE user_id = ? AND valid_from <= ? AND (valid_until IS NULL OR valid_until >= ?) ORDER BY valid_from DESC LIMIT 1`.

CorrectionStore.GetLatestForPeriod: `ORDER BY created_at DESC LIMIT 1`.

- [ ] **Step 8: Run all store tests – verify they pass**

```bash
go test ./internal/store/sqlite/ -v
```

Expected: all tests PASS.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat: complete SQLite store implementations for all entities"
```

---

## Task 6: Auth Service

**Files:**
- Create: `internal/service/auth/auth.go`
- Create: `internal/service/auth/auth_test.go`

- [ ] **Step 1: Write auth tests**

```go
package auth

import (
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	svc := New("test-secret", 8)
	hash, err := svc.HashPassword("mypassword")
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	if !svc.CheckPassword("mypassword", hash) {
		t.Error("expected password to match")
	}
	if svc.CheckPassword("wrongpassword", hash) {
		t.Error("expected wrong password to fail")
	}
}

func TestJWTIssueAndVerify(t *testing.T) {
	svc := New("test-jwt-secret", 8)
	token, err := svc.IssueToken(42, "admin", "superadmin")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	claims, err := svc.VerifyToken(token)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if claims.UserID != 42 {
		t.Errorf("expected user ID 42, got %d", claims.UserID)
	}
	if claims.Role != "superadmin" {
		t.Errorf("expected role superadmin, got %s", claims.Role)
	}
}

func TestJWTInvalidSecret(t *testing.T) {
	svc1 := New("secret-1", 8)
	svc2 := New("secret-2", 8)
	token, _ := svc1.IssueToken(1, "user", "user")
	_, err := svc2.VerifyToken(token)
	if err == nil {
		t.Error("expected verification to fail with wrong secret")
	}
}

func TestGenerateRandomPassword(t *testing.T) {
	p := GenerateRandomPassword(16)
	if len(p) != 16 {
		t.Errorf("expected length 16, got %d", len(p))
	}
}
```

- [ ] **Step 2: Run test – verify it fails**

```bash
go test ./internal/service/auth/ -v
```

- [ ] **Step 3: Implement auth service**

Create `internal/service/auth/auth.go`:

```go
package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

type Service struct {
	jwtSecret   []byte
	expiryHours int
}

func New(jwtSecret string, expiryHours int) *Service {
	return &Service{
		jwtSecret:   []byte(jwtSecret),
		expiryHours: expiryHours,
	}
}

func (s *Service) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func (s *Service) CheckPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *Service) IssueToken(userID int, username, role string) (string, error) {
	claims := &Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(s.expiryHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *Service) VerifyToken(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

func GenerateRandomPassword(length int) string {
	const charset = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	result := make([]byte, length)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[n.Int64()]
	}
	return string(result)
}
```

- [ ] **Step 4: Run test – verify it passes**

```bash
go test ./internal/service/auth/ -v
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: auth service with bcrypt hashing and JWT tokens"
```

---

## Task 7: Holiday Calculator (NRW)

**Files:**
- Create: `internal/service/timecalc/holidays.go`
- Create: `internal/service/timecalc/holidays_test.go`

- [ ] **Step 1: Write holiday tests**

```go
package timecalc

import (
	"testing"
)

func TestEasterDate(t *testing.T) {
	tests := []struct {
		year  int
		month int
		day   int
	}{
		{2024, 3, 31},
		{2025, 4, 20},
		{2026, 4, 5},
		{2027, 3, 28},
	}
	for _, tt := range tests {
		m, d := easterDate(tt.year)
		if m != tt.month || d != tt.day {
			t.Errorf("Easter %d: expected %d-%02d, got %d-%02d", tt.year, tt.month, tt.day, m, d)
		}
	}
}

func TestGenerateNRWHolidays(t *testing.T) {
	holidays := GenerateNRWHolidays(2026)
	if len(holidays) != 11 {
		t.Fatalf("expected 11 NRW holidays, got %d", len(holidays))
	}

	expected := map[string]string{
		"2026-01-01": "Neujahr",
		"2026-04-03": "Karfreitag",
		"2026-04-06": "Ostermontag",
		"2026-05-01": "Tag der Arbeit",
		"2026-05-14": "Christi Himmelfahrt",
		"2026-05-25": "Pfingstmontag",
		"2026-06-04": "Fronleichnam",
		"2026-10-03": "Tag der Deutschen Einheit",
		"2026-11-01": "Allerheiligen",
		"2026-12-25": "1. Weihnachtstag",
		"2026-12-26": "2. Weihnachtstag",
	}

	for _, h := range holidays {
		exp, ok := expected[h.HolidayDate]
		if !ok {
			t.Errorf("unexpected holiday: %s %s", h.HolidayDate, h.Name)
			continue
		}
		if h.Name != exp {
			t.Errorf("for %s: expected %q, got %q", h.HolidayDate, exp, h.Name)
		}
		if !h.AutoGenerated {
			t.Errorf("expected auto_generated=true for %s", h.HolidayDate)
		}
	}
}
```

- [ ] **Step 2: Run test – verify it fails**

```bash
go test ./internal/service/timecalc/ -run TestEaster -v
go test ./internal/service/timecalc/ -run TestGenerateNRW -v
```

- [ ] **Step 3: Implement holiday calculator**

Create `internal/service/timecalc/holidays.go` with Gauss Easter formula and NRW holiday list (11 holidays as per spec).

- [ ] **Step 4: Run tests – verify they pass**

```bash
go test ./internal/service/timecalc/ -v
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: NRW holiday calculator with Gauss Easter formula"
```

---

## Task 8: CSV Parser & Punch Pairing

**Files:**
- Create: `internal/service/importer/csv.go`
- Create: `internal/service/importer/csv_test.go`
- Create: `internal/service/importer/pairing.go`
- Create: `internal/service/importer/pairing_test.go`

- [ ] **Step 1: Write CSV parser test**

```go
package importer

import (
	"strings"
	"testing"
)

func TestParseCSV_Semicolon(t *testing.T) {
	input := "1711234567890;ABC123\n1711234590000;DEF456\n"
	punches, err := ParseCSV(strings.NewReader(input), "device1.csv")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(punches) != 2 {
		t.Fatalf("expected 2 punches, got %d", len(punches))
	}
	if punches[0].NFCTagUID != "ABC123" {
		t.Errorf("expected ABC123, got %s", punches[0].NFCTagUID)
	}
	if punches[0].SourceFile != "device1.csv" {
		t.Errorf("expected device1.csv, got %s", punches[0].SourceFile)
	}
}

func TestParseCSV_AutoDetect_Comma(t *testing.T) {
	input := "1711234567890,ABC123\n"
	punches, err := ParseCSV(strings.NewReader(input), "test.csv")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(punches) != 1 {
		t.Fatalf("expected 1 punch, got %d", len(punches))
	}
}

func TestParseCSV_EmptyLines(t *testing.T) {
	input := "1711234567890;ABC123\n\n\n1711234590000;DEF456\n"
	punches, err := ParseCSV(strings.NewReader(input), "test.csv")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(punches) != 2 {
		t.Fatalf("expected 2 punches (skipping empty lines), got %d", len(punches))
	}
}
```

- [ ] **Step 2: Write pairing test**

```go
package importer

import (
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
)

func TestPairPunches_Normal(t *testing.T) {
	day := "2026-03-26"
	punches := []model.RawPunch{
		{PunchTime: time.Date(2026, 3, 26, 7, 55, 0, 0, time.Local)},
		{PunchTime: time.Date(2026, 3, 26, 12, 0, 0, 0, time.Local)},
		{PunchTime: time.Date(2026, 3, 26, 12, 30, 0, 0, time.Local)},
		{PunchTime: time.Date(2026, 3, 26, 16, 5, 0, 0, time.Local)},
	}

	periods := PairPunches(1, day, punches)
	if len(periods) != 3 {
		t.Fatalf("expected 3 periods, got %d", len(periods))
	}

	// Period 1: work 07:55-12:00
	if periods[0].IsBreak {
		t.Error("period 0 should not be break")
	}
	// Period 2: break 12:00-12:30
	if !periods[1].IsBreak {
		t.Error("period 1 should be break")
	}
	// Period 3: work 12:30-16:05
	if periods[2].IsBreak {
		t.Error("period 2 should not be break")
	}
	if periods[2].PunchOut == nil {
		t.Error("period 2 should have punch_out")
	}
}

func TestPairPunches_OddCount(t *testing.T) {
	day := "2026-03-26"
	punches := []model.RawPunch{
		{PunchTime: time.Date(2026, 3, 26, 8, 0, 0, 0, time.Local)},
		{PunchTime: time.Date(2026, 3, 26, 12, 0, 0, 0, time.Local)},
		{PunchTime: time.Date(2026, 3, 26, 12, 30, 0, 0, time.Local)},
	}

	periods := PairPunches(1, day, punches)
	if len(periods) != 3 {
		t.Fatalf("expected 3 periods, got %d", len(periods))
	}
	// Last period should be open (punch_out = nil)
	if periods[2].PunchOut != nil {
		t.Error("last period should have nil punch_out (still clocked in)")
	}
}

func TestPairPunches_SinglePunch(t *testing.T) {
	punches := []model.RawPunch{
		{PunchTime: time.Date(2026, 3, 26, 8, 0, 0, 0, time.Local)},
	}
	periods := PairPunches(1, "2026-03-26", punches)
	if len(periods) != 1 {
		t.Fatalf("expected 1 period, got %d", len(periods))
	}
	if periods[0].PunchOut != nil {
		t.Error("single punch should have nil punch_out")
	}
}
```

- [ ] **Step 3: Run tests – verify they fail**

```bash
go test ./internal/service/importer/ -v
```

- [ ] **Step 4: Implement CSV parser**

Create `internal/service/importer/csv.go`: Read lines, auto-detect delimiter (try `;`, `,`, `\t`), parse timestamp (milliseconds since epoch) + NFC tag UID, return `[]model.RawPunch`.

- [ ] **Step 5: Implement pairing logic**

Create `internal/service/importer/pairing.go`: Sort punches by time, create work periods using toggle logic. Even-indexed pairs (0→1, 2→3) are work, odd-indexed gaps (1→2, 3→4) are breaks. Last odd punch gets `PunchOut = nil`.

- [ ] **Step 6: Run tests – verify they pass**

```bash
go test ./internal/service/importer/ -v
```

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat: CSV parser with auto-detection and punch-to-work-period pairing"
```

---

## Task 9: Time Calculation Engine

**Files:**
- Create: `internal/service/timecalc/rounding.go`
- Create: `internal/service/timecalc/rounding_test.go`
- Create: `internal/service/timecalc/breaks.go`
- Create: `internal/service/timecalc/breaks_test.go`
- Create: `internal/service/timecalc/daily.go`
- Create: `internal/service/timecalc/daily_test.go`
- Create: `internal/service/timecalc/balance.go`
- Create: `internal/service/timecalc/balance_test.go`

- [ ] **Step 1: Write rounding tests**

```go
package timecalc

import (
	"testing"
	"time"
)

func TestRoundDown(t *testing.T) {
	tests := []struct {
		minutes  float64
		unit     int
		expected float64
	}{
		{443, 15, 435},  // 7h23m → 7h15m
		{60, 15, 60},    // exact
		{29, 15, 15},    // 29m → 15m
		{14, 15, 0},     // below unit
		{480, 5, 480},   // 8h exact
		{483, 5, 480},   // 8h03m → 8h00m
	}
	for _, tt := range tests {
		got := RoundDown(time.Duration(tt.minutes)*time.Minute, tt.unit)
		expected := time.Duration(tt.expected) * time.Minute
		if got != expected {
			t.Errorf("RoundDown(%v, %d): expected %v, got %v", tt.minutes, tt.unit, expected, got)
		}
	}
}
```

- [ ] **Step 2: Implement rounding**

Create `internal/service/timecalc/rounding.go`:

```go
package timecalc

import "time"

func RoundDown(d time.Duration, unitMinutes int) time.Duration {
	unit := time.Duration(unitMinutes) * time.Minute
	return (d / unit) * unit
}
```

- [ ] **Step 3: Write break deduction tests**

```go
package timecalc

import (
	"testing"
	"time"

	"nfc-time-tracking-server/internal/model"
)

func TestCalcBreakDeduction_NoStampedBreak_Over6h(t *testing.T) {
	rules := []model.BreakRule{
		{MinWorkHours: 6.0, BreakMinutes: 30},
		{MinWorkHours: 9.0, BreakMinutes: 45},
	}
	gross := 7 * time.Hour
	stamped := time.Duration(0)
	deduction := CalcBreakDeduction(gross, stamped, rules)
	if deduction != 30*time.Minute {
		t.Errorf("expected 30m deduction, got %v", deduction)
	}
}

func TestCalcBreakDeduction_ShortStampedBreak(t *testing.T) {
	rules := []model.BreakRule{
		{MinWorkHours: 6.0, BreakMinutes: 30},
	}
	gross := 7 * time.Hour
	stamped := 20 * time.Minute
	deduction := CalcBreakDeduction(gross, stamped, rules)
	// Stamped 20min, required 30min → deduct 10min difference
	if deduction != 10*time.Minute {
		t.Errorf("expected 10m deduction, got %v", deduction)
	}
}

func TestCalcBreakDeduction_SufficientStampedBreak(t *testing.T) {
	rules := []model.BreakRule{
		{MinWorkHours: 6.0, BreakMinutes: 30},
	}
	gross := 7 * time.Hour
	stamped := 35 * time.Minute
	deduction := CalcBreakDeduction(gross, stamped, rules)
	if deduction != 0 {
		t.Errorf("expected 0 deduction, got %v", deduction)
	}
}

func TestCalcBreakDeduction_Under6h(t *testing.T) {
	rules := []model.BreakRule{
		{MinWorkHours: 6.0, BreakMinutes: 30},
	}
	gross := 5 * time.Hour
	stamped := time.Duration(0)
	deduction := CalcBreakDeduction(gross, stamped, rules)
	if deduction != 0 {
		t.Errorf("expected 0 deduction for <6h, got %v", deduction)
	}
}
```

- [ ] **Step 4: Implement break deduction**

Create `internal/service/timecalc/breaks.go`: Sort rules by MinWorkHours descending, find highest matching threshold, compare stamped break against required, return difference or zero.

- [ ] **Step 5: Write daily calculation tests**

Test cases:
- Normal work day with schedule: punch before shift start → clipped to shift_start
- Normal work day without schedule: actual punches used
- Holiday → credit 1/5 weekly hours
- Sick absence → credit 1/5
- Half-day vacation → credit 1/10
- Closure day → credit 1/5
- Weekend → zero target, zero work
- With time correction → corrected values used
- Timestamps with milliseconds → truncated to minutes before calculation (08:07:45.123 → 08:07)

- [ ] **Step 6: Implement daily calculation**

Create `internal/service/timecalc/daily.go` with `CalcDay` function that takes work periods, schedule, absence, holiday/closure info, weekly hours, break rules, rounding unit and returns `model.DayResult`. **Important:** First step in CalcDay is to truncate all punch timestamps to minute precision (discard seconds and milliseconds) before any further calculation.

- [ ] **Step 7: Write balance tests**

Test: Given a list of DayResults for a month + carryover from previous month → MonthBalance with correct worked, target, balance, total.

Test: VacationBalance for a year given entitlement + taken days.

- [ ] **Step 8: Implement balance calculation**

Create `internal/service/timecalc/balance.go`: `CalcMonthBalance` sums DayResults, `CalcVacationBalance` counts absences of type vacation.

- [ ] **Step 9: Run all timecalc tests**

```bash
go test ./internal/service/timecalc/ -v
```

Expected: all tests PASS.

- [ ] **Step 10: Commit**

```bash
git add -A
git commit -m "feat: time calculation engine with rounding, breaks, daily calc, balance"
```

---

## Task 10: FTP Import Service

**Files:**
- Create: `internal/service/importer/ftp.go`
- Create: `internal/service/importer/ftp_test.go`
- Create: `internal/service/importer/service.go`
- Create: `internal/service/importer/service_test.go`

- [ ] **Step 1: Write import service test (integration with store)**

Test using in-memory SQLite: Create test CSV content, run import service, verify raw_punches and work_periods are created in DB. Test dedup: run same import again, verify no duplicates.

- [ ] **Step 2: Implement FTP client wrapper**

Create `internal/service/importer/ftp.go`: `FTPClient` struct with `Connect`, `ListFiles`, `ReadFile`, `Close` methods wrapping `jlaffaye/ftp`. This is a thin wrapper for testability.

- [ ] **Step 3: Implement import service**

Create `internal/service/importer/service.go`: `ImportService` struct with dependencies (FTPClient, PunchStore, WorkPeriodStore, NFCTagStore). Methods:
- `RunImport(ctx)`: fetch all CSV files from FTP, parse, store punches, recompute work periods
- `StartScheduler(ctx, intervalSeconds)`: ticker-based goroutine
- `Status()`: returns last import time, error, count

- [ ] **Step 4: Run tests**

```bash
go test ./internal/service/importer/ -v
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: FTP import service with scheduler and dedup"
```

---

## Task 11: API Layer – Router, Middleware, Auth Endpoints

**Files:**
- Create: `internal/api/router.go`
- Create: `internal/api/middleware/auth.go`
- Create: `internal/api/response/json.go`
- Create: `internal/api/handler/auth.go`
- Create: `internal/api/handler/auth_test.go`

- [ ] **Step 1: Create JSON response helpers**

Create `internal/api/response/json.go`:

```go
package response

import (
	"encoding/json"
	"net/http"
)

func JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]string{"error": message})
}
```

- [ ] **Step 2: Create auth middleware**

Create `internal/api/middleware/auth.go`: Extract JWT from `Authorization: Bearer <token>` header, verify, inject claims into context. Role-check middleware `RequireRole(roles ...string)`.

- [ ] **Step 3: Write auth handler test**

Use `httptest` to test `POST /api/v1/auth/login` with valid and invalid credentials.

- [ ] **Step 4: Implement auth handler**

Create `internal/api/handler/auth.go`: Login (validate credentials, return JWT), Refresh, ChangePassword.

- [ ] **Step 5: Create router**

Create `internal/api/router.go`: chi router setup, mount auth routes, apply middleware. Structure all route groups with role checks.

- [ ] **Step 6: Run auth tests**

```bash
go test ./internal/api/handler/ -run TestAuth -v
```

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat: API router, auth middleware, login/refresh/change-password endpoints"
```

---

## Task 12: API Handlers – Employee & Management

**Files:**
- Create: `internal/api/handler/me.go`
- Create: `internal/api/handler/employees.go`
- Create: `internal/api/handler/schedules.go`
- Create: `internal/api/handler/absences.go`
- Create: `internal/api/handler/closuredays.go`
- Create: `internal/api/handler/corrections.go`
- Create: `internal/api/handler/employees_test.go`

- [ ] **Step 1: Implement /me handlers**

`GET /me/times`, `/me/balance`, `/me/vacation`, `/me/schedule` – extract user ID from JWT claims context, delegate to stores + timecalc service.

- [ ] **Step 2: Implement employee CRUD handlers**

`GET /employees`, `POST /employees`, `PATCH /employees/:id` – list, create, update. Include weekly-hours, vacation-entitlement, nfc-tags sub-routes.

- [ ] **Step 3: Implement employee times/balance handlers**

`GET /employees/:id/times`, `/employees/:id/balance` – delegate to timecalc engine.

- [ ] **Step 4: Implement corrections handler**

`POST /employees/:id/corrections`, `GET /employees/:id/corrections`. Validate reason is non-empty.

- [ ] **Step 5: Implement work period manual entry handler**

`POST /employees/:id/work-periods`, `DELETE /employees/:id/work-periods/:wpId`.

- [ ] **Step 6: Implement schedule, absence, closure day handlers**

Schedule: `GET /schedules?week=&year=`, `POST`, `PUT/:id`, `DELETE/:id`.
Absence: `POST /employees/:id/absences`, `GET`, `DELETE`.
Closure days: `GET /closure-days`, `POST`, `DELETE/:id`.

- [ ] **Step 7: Write key integration test**

Test creating employee, adding weekly hours, creating schedule, punching time, querying balance – via HTTP endpoints.

- [ ] **Step 8: Run tests**

```bash
go test ./internal/api/handler/ -v
```

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat: API handlers for employees, schedules, absences, corrections"
```

---

## Task 13: API Handlers – Admin, Import, Export

**Files:**
- Create: `internal/api/handler/holidays.go`
- Create: `internal/api/handler/settings.go`
- Create: `internal/api/handler/importhandler.go`
- Create: `internal/api/handler/export.go`
- Create: `internal/service/export/csv.go`
- Create: `internal/service/export/pdf.go`

- [ ] **Step 1: Implement holiday handler**

`GET /holidays?year=`, `POST /holidays`, `DELETE /holidays/:id`, `POST /holidays/generate?year=`. Generate calls `timecalc.GenerateNRWHolidays`.

- [ ] **Step 2: Implement settings handler**

`GET /settings`, `PUT /settings/:key`. Validate known keys.

- [ ] **Step 3: Implement user management handler (superadmin)**

`GET /users`, `POST /users` (can create leitung), `PATCH /users/:id`.

- [ ] **Step 4: Implement import handler**

`POST /import/trigger` – calls ImportService.RunImport. `GET /import/status` – returns ImportService.Status().

- [ ] **Step 5: Implement CSV export**

Create `internal/service/export/csv.go`: Generate CSV from DayResults for a date range. Columns: Date, Weekday, ShiftStart, ShiftEnd, PunchIn, PunchOut, GrossHours, NetHours, Target, Balance, Notes (holiday/sick/vacation).

- [ ] **Step 6: Implement PDF export**

Create `internal/service/export/pdf.go`: Monthly report with header (employee name, month, year), table of daily values, summary row with totals and balance.

- [ ] **Step 7: Implement export handler**

`GET /export/csv?employee=&from=&to=` – returns CSV file. `GET /export/pdf?employee=&month=&year=` – returns PDF.

- [ ] **Step 8: Register all routes in router**

Update `internal/api/router.go` to mount all handler groups with correct middleware.

- [ ] **Step 9: Run all tests**

```bash
go test ./... -v
```

- [ ] **Step 10: Commit**

```bash
git add -A
git commit -m "feat: admin, import, export API handlers with CSV/PDF generation"
```

---

## Task 14: Server Bootstrap & First-Start Logic

**Files:**
- Modify: `cmd/server/main.go`
- Create: `internal/web/embed.go`
- Create: `internal/web/embed_dev.go`

- [ ] **Step 1: Create embed files**

Create `cmd/server/embed.go` (production build – embeds compiled frontend):

```go
//go:build !dev

package main

import "embed"

//go:embed all:web_dist
var webFS embed.FS
```

Note: This requires a symlink or copy step in the Makefile so that `web/dist` content is available at build time under `cmd/server/web_dist/`. Alternatively, use a dedicated `internal/web` package with the embed directive.

A cleaner approach: create `internal/web/embed.go`:

```go
//go:build !dev

package web

import "embed"
import "io/fs"

//go:embed dist
var embedFS embed.FS

func FS() (fs.FS, error) {
	return fs.Sub(embedFS, "dist")
}
```

And `internal/web/embed_dev.go`:

```go
//go:build dev

package web

import (
	"io/fs"
	"os"
)

func FS() (fs.FS, error) {
	return os.DirFS("web/dist"), nil
}
```

Place the built frontend at `internal/web/dist/` (Makefile copies `web/dist/*` → `internal/web/dist/`).

Update the file structure accordingly:
- `internal/web/embed.go` and `internal/web/embed_dev.go`
- Makefile copies `web/dist/` → `internal/web/dist/` before Go build

Create `web/dist/.gitkeep` and `internal/web/dist/.gitkeep` as placeholders so the directories exist for development builds.

- [ ] **Step 2: Wire up main.go**

Update `cmd/server/main.go`:
1. Load config
2. Open database (auto-migrate)
3. Initialize all stores
4. Initialize services (auth, importer, timecalc)
5. First-start check: if no users exist, create superadmin with random password, log to console
6. Generate holidays for current + next year if none exist
7. Start FTP import scheduler
8. Create router with all handlers
9. Serve embedded SPA for non-API routes
10. Start HTTP server

- [ ] **Step 3: Test server starts**

```bash
go run -tags dev ./cmd/server
```

Expected: server starts, prints superadmin credentials, listens on :8080. Ctrl+C to stop.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat: server bootstrap with first-start logic and auto-migration"
```

---

## Task 15: Vue.js Frontend Scaffolding

**Files:**
- Create: `web/` (entire Vue 3 project)

- [ ] **Step 1: Scaffold Vue project**

```bash
cd /Users/Apps.Associates/nfc-time-tracking-server
npm create vite@latest web -- --template vue-ts
cd web
npm install
npm install primevue @primevue/themes primeicons
npm install pinia
npm install vue-router@4
npm install axios
```

- [ ] **Step 2: Configure PrimeVue in main.ts**

Set up PrimeVue with Aura theme, install router and Pinia.

- [ ] **Step 3: Create API client**

Create `web/src/api/client.ts`: Axios instance with base URL `/api/v1/`, JWT interceptor that adds `Authorization: Bearer` header from auth store, 401 interceptor that redirects to login.

- [ ] **Step 4: Create auth store**

Create `web/src/stores/auth.ts`: Pinia store with `login`, `logout`, `refreshToken` actions. Stores JWT in localStorage. Exposes `user`, `role`, `isAuthenticated` getters.

- [ ] **Step 5: Create router with guards**

Create `web/src/router/index.ts`: All routes from spec section 7.2. Navigation guard checks auth state and role. Redirect to `/login` if unauthenticated. Redirect to `/dashboard` after login.

- [ ] **Step 6: Create layouts**

Create `web/src/layouts/AppLayout.vue`: Sidebar with navigation (role-filtered menu items), top bar with user info + logout. Responsive.

Create `web/src/layouts/AuthLayout.vue`: Centered card layout for login page.

- [ ] **Step 7: Create LoginView**

Create `web/src/views/LoginView.vue`: Username + password form with PrimeVue InputText + Password + Button. Error message on failed login. Force password change modal on first login.

- [ ] **Step 8: Verify frontend builds**

```bash
cd web && npm run build
```

Expected: `web/dist/` created successfully.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat: Vue.js frontend scaffolding with PrimeVue, auth, router, layouts"
```

---

## Task 16: Frontend – Dashboard & User Views

**Files:**
- Create: `web/src/views/DashboardView.vue`
- Create: `web/src/views/my/MyTimesView.vue`
- Create: `web/src/views/my/MyBalanceView.vue`
- Create: `web/src/views/my/MyVacationView.vue`
- Create: `web/src/views/my/MyScheduleView.vue`
- Create: `web/src/components/TimeTable.vue`
- Create: `web/src/components/BalanceCard.vue`

- [ ] **Step 1: Create TimeTable component**

Reusable DataTable (PrimeVue) showing work periods for a date range. Columns: Date, PunchIn, PunchOut, Gross, Net, Notes. Color-coded rows for holidays, absences.

- [ ] **Step 2: Create BalanceCard component**

Card showing month balance: Worked, Target, Balance, Carryover. Green/red color for positive/negative.

- [ ] **Step 3: Implement DashboardView**

- Today's punches (current status: clocked in/out)
- Today's net hours so far
- Current month balance
- Remaining vacation days
- Next scheduled shift
- Leitung: attendance overview table (who's in/out)

- [ ] **Step 4: Implement MyTimesView**

Date range picker (week/month selector). TimeTable component. PrimeVue Calendar for date navigation.

- [ ] **Step 5: Implement MyBalanceView**

Month/year selector. Monthly balance cards. Yearly summary. Rolling carryover visualization.

- [ ] **Step 6: Implement MyVacationView**

Vacation balance card (entitled, taken, remaining, carried over). List of vacation absences for current year.

- [ ] **Step 7: Implement MyScheduleView**

Week view of own schedule. PrimeVue DataTable with days as columns.

- [ ] **Step 8: Verify build**

```bash
cd web && npm run build
```

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat: frontend dashboard and user views (times, balance, vacation, schedule)"
```

---

## Task 17: Frontend – Management Views

**Files:**
- Create: `web/src/views/employees/EmployeeListView.vue`
- Create: `web/src/views/employees/EmployeeDetailView.vue`
- Create: `web/src/views/employees/EmployeeEditView.vue`
- Create: `web/src/views/schedule/ScheduleEditorView.vue`
- Create: `web/src/views/absences/AbsencesView.vue`
- Create: `web/src/views/corrections/CorrectionsView.vue`
- Create: `web/src/views/closuredays/ClosureDaysView.vue`
- Create: `web/src/views/import/ImportView.vue`
- Create: `web/src/components/ScheduleGrid.vue`

- [ ] **Step 1: Implement EmployeeListView**

DataTable of employees. Columns: Name, Role, Active, Weekly Hours. Filter by active/inactive. "New Employee" button.

- [ ] **Step 2: Implement EmployeeDetailView**

Tabs: Times, Balance, Absences, Corrections, Settings. Reuses TimeTable and BalanceCard components.

- [ ] **Step 3: Implement EmployeeEditView**

Form for: Display name, active status, weekly hours (with valid_from), vacation entitlement (with valid_from), NFC tag assignment.

- [ ] **Step 4: Implement ScheduleGrid component**

Week grid: rows = employees, columns = Mo–Fr. Each cell: two time inputs (shift start, shift end). "Copy from previous week" button.

- [ ] **Step 5: Implement ScheduleEditorView**

Week picker + ScheduleGrid. Save button persists all changes.

- [ ] **Step 6: Implement AbsencesView**

Calendar-style view or DataTable. Filter by employee. "Add Absence" dialog with employee picker, date, type (sick/vacation/other), half-day toggle.

- [ ] **Step 7: Implement CorrectionsView**

List of work periods for selected employee + date range. Each row shows original + corrected values. "Correct" button opens dialog with pre-filled original values and mandatory reason field.

Also: "Manual Entry" button for cases where no punch exists (NFC tag forgotten).

- [ ] **Step 8: Implement ClosureDaysView**

DataTable of closure days. "Add" dialog with date + name.

- [ ] **Step 9: Implement ImportView**

Import status card (last import time, success/error). "Import Now" button. Import log.

- [ ] **Step 10: Verify build**

```bash
cd web && npm run build
```

- [ ] **Step 11: Commit**

```bash
git add -A
git commit -m "feat: frontend management views (employees, schedule, absences, corrections, import)"
```

---

## Task 18: Frontend – Admin Views & Reports

**Files:**
- Create: `web/src/views/admin/UsersView.vue`
- Create: `web/src/views/admin/HolidaysView.vue`
- Create: `web/src/views/admin/SettingsView.vue`
- Create: `web/src/views/reports/ReportsView.vue`

- [ ] **Step 1: Implement UsersView**

DataTable of all users (incl. leitung). "Create Leitung User" button. Edit role, active status.

- [ ] **Step 2: Implement HolidaysView**

Year picker. DataTable of holidays with auto-generated badge. "Generate for Year" button. "Add" dialog. Delete button (with confirmation).

- [ ] **Step 3: Implement SettingsView**

Form with:
- Rounding (dropdown: 5, 10, 15, 30 minutes)
- Break rules (editable table: min hours → break minutes)
- FTP configuration (host, port, user, password, path, interval)
- CSV delimiter

- [ ] **Step 4: Implement ReportsView**

Employee picker (or "all"). Date range (month/year). Preview table. "Export CSV" + "Export PDF" buttons that trigger downloads.

- [ ] **Step 5: Verify build**

```bash
cd web && npm run build
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: frontend admin views (users, holidays, settings) and reports"
```

---

## Task 19: Embedding & Full Integration

**Files:**
- Modify: `internal/web/embed.go`
- Modify: `cmd/server/main.go`
- Modify: `Makefile`

- [ ] **Step 1: Finalize internal/web/embed.go**

Ensure `internal/web/embed.go` correctly embeds `internal/web/dist/` and the `FS()` function returns a proper `fs.FS` for serving the SPA. Update Makefile to copy `web/dist/*` → `internal/web/dist/` before Go build.

- [ ] **Step 2: Add SPA serving to router**

In `internal/api/router.go`, add a catch-all handler that serves static files from the embedded FS, falling back to `index.html` for any unmatched route (SPA routing).

- [ ] **Step 3: Build full binary**

```bash
cd web && npm run build && cd ..
go build -o bin/nfc-time-tracker-server ./cmd/server
```

- [ ] **Step 4: Test full binary**

```bash
./bin/nfc-time-tracker-server
```

Expected: Server starts, superadmin credentials printed. Open `http://localhost:8080` → Vue.js SPA loads. Login with superadmin → dashboard.

- [ ] **Step 5: Test cross-compilation**

```bash
GOOS=linux GOARCH=amd64 go build -o bin/nfc-time-tracker-server-linux ./cmd/server
GOOS=windows GOARCH=amd64 go build -o bin/nfc-time-tracker-server.exe ./cmd/server
```

Expected: both binaries compile without errors.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: full integration with embedded SPA and cross-platform build"
```

---

## Task 20: End-to-End Smoke Test

- [ ] **Step 1: Start server**

```bash
./bin/nfc-time-tracker-server
```

Note the superadmin password from console output.

- [ ] **Step 2: Login as superadmin**

Open `http://localhost:8080`. Login with admin credentials. Change password.

- [ ] **Step 3: Configure FTP**

Navigate to Settings. Enter Fritz!Box FTP credentials (or skip if no Fritz!Box available – test with mock data).

- [ ] **Step 4: Create a test employee**

Navigate to Employees → New. Create user "testmitarbeiter" with 40h/week, 30 vacation days.

- [ ] **Step 5: Assign NFC tag**

In employee edit, assign NFC tag "AABBCCDD".

- [ ] **Step 6: Create schedule**

Navigate to Schedule Editor. Create a schedule for testmitarbeiter: Mo–Fr 08:00–16:30.

- [ ] **Step 7: Verify holiday generation**

Navigate to Holidays. Verify 2026 + 2027 NRW holidays are listed.

- [ ] **Step 8: Verify balance calculation**

Navigate to Employee Detail → Balance. Verify monthly balance shows correct target hours.

- [ ] **Step 9: Test export**

Navigate to Reports. Select employee + month. Download CSV and PDF. Verify content.

- [ ] **Step 10: Commit any fixes**

```bash
git add -A
git commit -m "fix: smoke test fixes"
```
