# Compensation Days Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Ausgleichstag claims from weekend work, allow booking `compensation_day` absences only against open claims, and keep overtime accounting unchanged except for the normal target-hour debit when the day is taken.

**Architecture:** Add a dedicated `compensation_day_claims` table and store. Synchronize open claims from weekend work after work-period and correction changes. Extend absence validation so `compensation_day` consumes the oldest open claim and deleting that absence reopens the linked claim.

**Tech Stack:** Go HTTP handlers, SQLite stores and goose migrations, Vue 3 with PrimeVue, existing REST API client.

---

## File Structure

- Create `internal/model/compensation_day_claim.go` for claim model and status constants.
- Create `internal/store/sqlite/compensation_day_claims.go` for claim CRUD and synchronization helpers.
- Create `internal/store/sqlite/migrations/003_compensation_day_claims.sql` for schema changes.
- Modify `internal/model/absence.go` to add `AbsenceCompensationDay`.
- Modify `internal/store/interfaces.go` to add `CompensationDayClaimStore`.
- Modify `internal/api/router.go` and `cmd/server/main.go` to wire the new store.
- Modify `internal/api/handler/employees.go` to validate, consume, list, waive, and reopen claims.
- Modify `internal/service/daycalc/daycalc.go`, `internal/service/export/rows.go`, and `internal/service/teamoverview/overview.go` so `compensation_day` has no absence credit and is visible in outputs.
- Modify `web/src/types/api.ts`, `web/src/api/management.ts`, and `web/src/views/absences/AbsencesView.vue` for UI support.
- Extend tests in `internal/store/sqlite/stores_test.go`, `internal/api/handler/absence_vacation_test.go` or a new handler test file, and `internal/service/teamoverview/overview_test.go`.

---

### Task 1: Data Model And Migration

**Files:**
- Create: `internal/model/compensation_day_claim.go`
- Modify: `internal/model/absence.go`
- Modify: `internal/store/interfaces.go`
- Create: `internal/store/sqlite/migrations/003_compensation_day_claims.sql`

- [ ] **Step 1: Add model constants and structs**

Create `internal/model/compensation_day_claim.go`:

```go
package model

import "time"

type CompensationDayClaimStatus string

const (
	CompensationDayClaimOpen   CompensationDayClaimStatus = "open"
	CompensationDayClaimUsed   CompensationDayClaimStatus = "used"
	CompensationDayClaimWaived CompensationDayClaimStatus = "waived"
)

type CompensationDayClaim struct {
	ID            int                        `json:"id"`
	UserID        int                        `json:"user_id"`
	WorkDate      string                     `json:"work_date"`
	Status        CompensationDayClaimStatus `json:"status"`
	UsedAbsenceID *int                       `json:"used_absence_id"`
	CreatedAt     time.Time                  `json:"created_at"`
	UpdatedAt     time.Time                  `json:"updated_at"`
}
```

Modify `internal/model/absence.go`:

```go
const (
	AbsenceSick            AbsenceType = "sick"
	AbsenceVacation        AbsenceType = "vacation"
	AbsenceOther           AbsenceType = "other"
	AbsenceCompensationDay AbsenceType = "compensation_day"
)
```

- [ ] **Step 2: Add store interface**

Modify `internal/store/interfaces.go`:

```go
type CompensationDayClaimStore interface {
	EnsureForWorkDate(ctx context.Context, userID int, workDate string, hasEligibleWork bool) error
	GetOldestOpen(ctx context.Context, userID int) (*model.CompensationDayClaim, error)
	MarkUsed(ctx context.Context, claimID int, absenceID int) error
	ReopenByAbsenceID(ctx context.Context, absenceID int) error
	Waive(ctx context.Context, userID int, claimID int) error
	ListByUser(ctx context.Context, userID int, status *model.CompensationDayClaimStatus) ([]model.CompensationDayClaim, error)
	CountOpen(ctx context.Context, userID int) (int, error)
}
```

- [ ] **Step 3: Add migration**

Create `internal/store/sqlite/migrations/003_compensation_day_claims.sql`:

```sql
-- +goose Up
CREATE TABLE compensation_day_claims (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    work_date DATE NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('open', 'used', 'waived')),
    used_absence_id INTEGER REFERENCES absences(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, work_date)
);

CREATE INDEX idx_compensation_day_claims_user_status
    ON compensation_day_claims(user_id, status, work_date);

CREATE INDEX idx_compensation_day_claims_used_absence
    ON compensation_day_claims(used_absence_id);

PRAGMA foreign_keys=off;

CREATE TABLE absences_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    absence_date DATE NOT NULL,
    absence_type TEXT NOT NULL CHECK(absence_type IN ('sick', 'vacation', 'other', 'compensation_day')),
    half_day BOOLEAN NOT NULL DEFAULT 0,
    created_by INTEGER NOT NULL REFERENCES users(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, absence_date)
);

INSERT INTO absences_new (id, user_id, absence_date, absence_type, half_day, created_by, created_at)
SELECT id, user_id, absence_date, absence_type, half_day, created_by, created_at
FROM absences;

DROP TABLE absences;
ALTER TABLE absences_new RENAME TO absences;

PRAGMA foreign_keys=on;

-- +goose Down
PRAGMA foreign_keys=off;

CREATE TABLE absences_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id),
    absence_date DATE NOT NULL,
    absence_type TEXT NOT NULL CHECK(absence_type IN ('sick', 'vacation', 'other')),
    half_day BOOLEAN NOT NULL DEFAULT 0,
    created_by INTEGER NOT NULL REFERENCES users(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, absence_date)
);

INSERT INTO absences_old (id, user_id, absence_date, absence_type, half_day, created_by, created_at)
SELECT id, user_id, absence_date, absence_type, half_day, created_by, created_at
FROM absences
WHERE absence_type IN ('sick', 'vacation', 'other');

DROP TABLE absences;
ALTER TABLE absences_old RENAME TO absences;

DROP TABLE IF EXISTS compensation_day_claims;

PRAGMA foreign_keys=on;
```

- [ ] **Step 4: Run migration/store tests**

Run:

```bash
go test ./internal/store/sqlite -run TestAbsenceStore_CRUD -count=1
```

Expected: tests compile. A failure about the missing concrete store is acceptable until Task 2 is implemented.

---

### Task 2: SQLite Claim Store

**Files:**
- Create: `internal/store/sqlite/compensation_day_claims.go`
- Modify: `internal/store/sqlite/stores_test.go`

- [ ] **Step 1: Write store tests**

Add tests to `internal/store/sqlite/stores_test.go`:

```go
func TestCompensationDayClaimStore_EnsureForWorkDate(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	cs := NewCompensationDayClaimStore(db)

	u := &model.User{Username: "claim", PasswordHash: "x", DisplayName: "Claim", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}

	if err := cs.EnsureForWorkDate(ctx, u.ID, "2026-04-04", true); err != nil {
		t.Fatal(err)
	}
	if err := cs.EnsureForWorkDate(ctx, u.ID, "2026-04-04", true); err != nil {
		t.Fatal(err)
	}
	list, err := cs.ListByUser(ctx, u.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Status != model.CompensationDayClaimOpen {
		t.Fatalf("expected one open claim, got %+v", list)
	}

	if err := cs.EnsureForWorkDate(ctx, u.ID, "2026-04-04", false); err != nil {
		t.Fatal(err)
	}
	list, err = cs.ListByUser(ctx, u.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected open claim to be removed, got %+v", list)
	}
}

func TestCompensationDayClaimStore_UseReopenAndWaive(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	us := NewUserStore(db)
	as := NewAbsenceStore(db)
	cs := NewCompensationDayClaimStore(db)

	u := &model.User{Username: "claim2", PasswordHash: "x", DisplayName: "Claim 2", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := cs.EnsureForWorkDate(ctx, u.ID, "2026-04-04", true); err != nil {
		t.Fatal(err)
	}
	a := &model.Absence{UserID: u.ID, AbsenceDate: "2026-04-07", AbsenceType: model.AbsenceCompensationDay, HalfDay: false, CreatedBy: u.ID}
	if err := as.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	claim, err := cs.GetOldestOpen(ctx, u.ID)
	if err != nil || claim == nil {
		t.Fatalf("open claim: %v %+v", err, claim)
	}
	if err := cs.MarkUsed(ctx, claim.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	open, err := cs.CountOpen(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if open != 0 {
		t.Fatalf("open count want 0, got %d", open)
	}
	if err := cs.ReopenByAbsenceID(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	open, err = cs.CountOpen(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if open != 1 {
		t.Fatalf("open count after reopen want 1, got %d", open)
	}
	if err := cs.Waive(ctx, u.ID, claim.ID); err != nil {
		t.Fatal(err)
	}
	open, err = cs.CountOpen(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if open != 0 {
		t.Fatalf("open count after waive want 0, got %d", open)
	}
}
```

- [ ] **Step 2: Run tests and confirm failure**

Run:

```bash
go test ./internal/store/sqlite -run 'TestCompensationDayClaimStore' -count=1
```

Expected: FAIL because `NewCompensationDayClaimStore` does not exist.

- [ ] **Step 3: Implement store**

Create `internal/store/sqlite/compensation_day_claims.go`:

```go
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"nfc-time-tracking-server/internal/model"
)

type CompensationDayClaimStore struct {
	db *DB
}

func NewCompensationDayClaimStore(db *DB) *CompensationDayClaimStore {
	return &CompensationDayClaimStore{db: db}
}

func (s *CompensationDayClaimStore) EnsureForWorkDate(ctx context.Context, userID int, workDate string, hasEligibleWork bool) error {
	if hasEligibleWork {
		_, err := s.db.DB.ExecContext(ctx, `
INSERT INTO compensation_day_claims (user_id, work_date, status)
VALUES (?, ?, ?)
ON CONFLICT(user_id, work_date) DO NOTHING
`, userID, workDate, model.CompensationDayClaimOpen)
		return err
	}
	_, err := s.db.DB.ExecContext(ctx, `
DELETE FROM compensation_day_claims
WHERE user_id = ? AND work_date = ? AND status = ?
`, userID, workDate, model.CompensationDayClaimOpen)
	return err
}

func (s *CompensationDayClaimStore) GetOldestOpen(ctx context.Context, userID int) (*model.CompensationDayClaim, error) {
	row := s.db.DB.QueryRowContext(ctx, `
SELECT id, user_id, work_date, status, used_absence_id, created_at, updated_at
FROM compensation_day_claims
WHERE user_id = ? AND status = ?
ORDER BY work_date, id
LIMIT 1
`, userID, model.CompensationDayClaimOpen)
	return scanCompensationDayClaim(row)
}

func (s *CompensationDayClaimStore) MarkUsed(ctx context.Context, claimID int, absenceID int) error {
	res, err := s.db.DB.ExecContext(ctx, `
UPDATE compensation_day_claims
SET status = ?, used_absence_id = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND status = ?
`, model.CompensationDayClaimUsed, absenceID, claimID, model.CompensationDayClaimOpen)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("kein offener Ausgleichstag-Anspruch gefunden")
	}
	return nil
}

func (s *CompensationDayClaimStore) ReopenByAbsenceID(ctx context.Context, absenceID int) error {
	_, err := s.db.DB.ExecContext(ctx, `
UPDATE compensation_day_claims
SET status = ?, used_absence_id = NULL, updated_at = CURRENT_TIMESTAMP
WHERE used_absence_id = ? AND status = ?
`, model.CompensationDayClaimOpen, absenceID, model.CompensationDayClaimUsed)
	return err
}

func (s *CompensationDayClaimStore) Waive(ctx context.Context, userID int, claimID int) error {
	res, err := s.db.DB.ExecContext(ctx, `
UPDATE compensation_day_claims
SET status = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND user_id = ? AND status = ?
`, model.CompensationDayClaimWaived, claimID, userID, model.CompensationDayClaimOpen)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("offener Ausgleichstag-Anspruch nicht gefunden")
	}
	return nil
}

func (s *CompensationDayClaimStore) ListByUser(ctx context.Context, userID int, status *model.CompensationDayClaimStatus) ([]model.CompensationDayClaim, error) {
	query := `
SELECT id, user_id, work_date, status, used_absence_id, created_at, updated_at
FROM compensation_day_claims
WHERE user_id = ?`
	args := []interface{}{userID}
	if status != nil {
		query += ` AND status = ?`
		args = append(args, *status)
	}
	query += ` ORDER BY work_date DESC, id DESC`
	rows, err := s.db.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.CompensationDayClaim
	for rows.Next() {
		c, err := scanCompensationDayClaimRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *CompensationDayClaimStore) CountOpen(ctx context.Context, userID int) (int, error) {
	var n int
	err := s.db.DB.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM compensation_day_claims
WHERE user_id = ? AND status = ?
`, userID, model.CompensationDayClaimOpen).Scan(&n)
	return n, err
}

type compensationDayClaimScanner interface {
	Scan(dest ...interface{}) error
}

func scanCompensationDayClaim(row compensationDayClaimScanner) (*model.CompensationDayClaim, error) {
	var c model.CompensationDayClaim
	var used sql.NullInt64
	var created, updated string
	err := row.Scan(&c.ID, &c.UserID, &c.WorkDate, &c.Status, &used, &created, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if used.Valid {
		id := int(used.Int64)
		c.UsedAbsenceID = &id
	}
	c.CreatedAt = parseSQLiteTime(created)
	c.UpdatedAt = parseSQLiteTime(updated)
	return &c, nil
}

func scanCompensationDayClaimRows(rows *sql.Rows) (*model.CompensationDayClaim, error) {
	return scanCompensationDayClaim(rows)
}

func parseSQLiteTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
```

- [ ] **Step 4: Run store tests**

Run:

```bash
go test ./internal/store/sqlite -run 'TestCompensationDayClaimStore' -count=1
```

Expected: PASS.

---

### Task 3: Synchronize Claims From Weekend Work

**Files:**
- Modify: `internal/api/handler/employees.go`
- Modify: `internal/api/router.go`
- Modify: `cmd/server/main.go`
- Test: `internal/api/handler/compensation_days_test.go` for helper behavior and `internal/store/sqlite/stores_test.go` for store behavior

- [ ] **Step 1: Add dependency wiring**

Modify `internal/api/router.go` `Deps`:

```go
CompensationDayClaims store.CompensationDayClaimStore
```

Modify `EmployeeHandler` in `internal/api/handler/employees.go`:

```go
CompensationDayClaims store.CompensationDayClaimStore
```

Wire it in `api.NewRouter`:

```go
CompensationDayClaims: d.CompensationDayClaims,
```

Modify `cmd/server/main.go`:

```go
compensationDayClaims := sqlite.NewCompensationDayClaimStore(db)
```

and pass it into `api.NewRouter(api.Deps{...})`:

```go
CompensationDayClaims: compensationDayClaims,
```

- [ ] **Step 2: Add synchronization helper**

Add to `internal/api/handler/employees.go` near work-period handlers:

```go
func isWeekendDate(dateStr string) bool {
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return false
	}
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

func hasEligibleWeekendWork(periods []model.WorkPeriod) bool {
	for _, p := range periods {
		if p.IsBreak || p.PunchOut == nil {
			continue
		}
		if p.PunchOut.After(p.PunchIn) {
			return true
		}
	}
	return false
}

func (h *EmployeeHandler) syncCompensationDayClaimForDate(ctx context.Context, userID int, dateStr string) error {
	if h.CompensationDayClaims == nil || !isWeekendDate(dateStr) {
		return nil
	}
	periods, err := h.WorkPeriods.ListByUserDateRange(ctx, userID, dateStr, dateStr)
	if err != nil {
		return err
	}
	return h.CompensationDayClaims.EnsureForWorkDate(ctx, userID, dateStr, hasEligibleWeekendWork(periods))
}
```

- [ ] **Step 3: Call synchronization after manual work-period create/delete**

In `CreateWorkPeriod`, after `CreateManual` succeeds and before the JSON response:

```go
if err := h.syncCompensationDayClaimForDate(r.Context(), uid, body.WorkDate); err != nil {
	response.Error(w, http.StatusInternalServerError, "Ausgleichstag-Anspruch konnte nicht aktualisiert werden")
	return
}
```

In `DeleteWorkPeriod`, capture the deleted period date before deleting:

```go
var workDate string
for _, p := range periods {
	if p.ID == wpid {
		found = true
		workDate = p.WorkDate
		break
	}
}
```

After `DeleteManual` succeeds:

```go
if err := h.syncCompensationDayClaimForDate(r.Context(), uid, workDate); err != nil {
	response.Error(w, http.StatusInternalServerError, "Ausgleichstag-Anspruch konnte nicht aktualisiert werden")
	return
}
```

- [ ] **Step 4: Call synchronization after corrections**

After `h.Corrections.Create` succeeds in `CreateCorrection`:

```go
if err := h.syncCompensationDayClaimForDate(r.Context(), uid, day); err != nil {
	response.Error(w, http.StatusInternalServerError, "Ausgleichstag-Anspruch konnte nicht aktualisiert werden")
	return
}
```

- [ ] **Step 5: Backfill existing weekend work**

Add a small startup backfill helper in `cmd/server/main.go` after store construction:

```go
if err := bootstrapCompensationDayClaims(ctx, workPeriods, compensationDayClaims, users); err != nil {
	log.Printf("bootstrap compensation day claims: %v", err)
}
```

Create helper in the same file or a small service file if `main.go` grows too large:

```go
func bootstrapCompensationDayClaims(ctx context.Context, workPeriods store.WorkPeriodStore, claims store.CompensationDayClaimStore, users store.UserStore) error {
	list, err := users.List(ctx, true)
	if err != nil {
		return err
	}
	for _, u := range list {
		periods, err := workPeriods.ListByUserDateRange(ctx, u.ID, "1970-01-01", "2099-12-31")
		if err != nil {
			return err
		}
		byDate := map[string][]model.WorkPeriod{}
		for _, p := range periods {
			if !isWeekendDateForBootstrap(p.WorkDate) {
				continue
			}
			byDate[p.WorkDate] = append(byDate[p.WorkDate], p)
		}
		for date, dayPeriods := range byDate {
			if err := claims.EnsureForWorkDate(ctx, u.ID, date, hasEligibleWeekendWorkForBootstrap(dayPeriods)); err != nil {
				return err
			}
		}
	}
	return nil
}
```

Use local helper names in `main.go` to avoid importing `handler`.

- [ ] **Step 6: Run synchronization-focused tests**

Run:

```bash
go test ./internal/api/handler ./internal/store/sqlite -count=1
```

Expected: PASS after handler dependencies and helper functions compile.

---

### Task 4: Book, Delete, List, And Waive Compensation Days

**Files:**
- Modify: `internal/api/handler/employees.go`
- Modify: `internal/api/router.go`
- Test: create `internal/api/handler/compensation_days_test.go`

- [ ] **Step 1: Add validation helpers**

Add to `internal/api/handler/employees.go` near `validateVacationAbsenceDate`:

```go
func validateCompensationDayAbsenceDate(ctx context.Context, holidays store.HolidayStore, dateStr string, halfDay bool) error {
	if halfDay {
		return errors.New("Halbe Ausgleichstage sind nicht möglich.")
	}
	t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return errors.New("Ungültiges Datum: Bitte ein gültiges Kalenderdatum im Format JJJJ-MM-TT angeben.")
	}
	wd := t.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return fmt.Errorf(
			"Am %s, %s, kann kein Ausgleichstag gebucht werden — das ist ein Wochenende.",
			germanWeekdayName[wd], t.Format("02.01.2006"),
		)
	}
	h, err := holidays.GetForDate(ctx, dateStr)
	if err != nil {
		return err
	}
	if h != nil {
		return fmt.Errorf(
			"Am %s, %s, kann kein Ausgleichstag gebucht werden — „%s“ ist ein gesetzlicher Feiertag.",
			germanWeekdayName[wd], t.Format("02.01.2006"), h.Name,
		)
	}
	return nil
}
```

- [ ] **Step 2: Consume open claim when creating absence**

Modify `CreateAbsence`:

```go
if body.AbsenceType == model.AbsenceVacation {
	if err := validateVacationAbsenceDate(r.Context(), h.Holidays, body.AbsenceDate); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
}
var compensationDayClaim *model.CompensationDayClaim
if body.AbsenceType == model.AbsenceCompensationDay {
	if err := validateCompensationDayAbsenceDate(r.Context(), h.Holidays, body.AbsenceDate, body.HalfDay); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	claim, err := h.CompensationDayClaims.GetOldestOpen(r.Context(), uid)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Ausgleichstag-Anspruch konnte nicht geprüft werden")
		return
	}
	if claim == nil {
		response.Error(w, http.StatusBadRequest, "Für diesen Mitarbeiter ist kein offener Ausgleichstag-Anspruch vorhanden.")
		return
	}
	compensationDayClaim = claim
}
```

After `h.Absences.Create` succeeds:

```go
if compensationDayClaim != nil {
	if err := h.CompensationDayClaims.MarkUsed(r.Context(), compensationDayClaim.ID, a.ID); err != nil {
		_ = h.Absences.Delete(r.Context(), a.ID)
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
}
```

- [ ] **Step 3: Reopen claim when deleting compensation-day absence**

Modify `DeleteAbsence` before deleting:

```go
isCompensationDay := a.AbsenceType == model.AbsenceCompensationDay
```

After `h.Absences.Delete` succeeds:

```go
if isCompensationDay {
	if err := h.CompensationDayClaims.ReopenByAbsenceID(r.Context(), aid); err != nil {
		response.Error(w, http.StatusInternalServerError, "Ausgleichstag-Anspruch konnte nicht wieder geöffnet werden")
		return
	}
}
```

- [ ] **Step 4: Add list and waive endpoints**

Add handlers:

```go
func (h *EmployeeHandler) ListCompensationDayClaims(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeID(w, r)
	if !ok {
		return
	}
	var status *model.CompensationDayClaimStatus
	if raw := r.URL.Query().Get("status"); raw != "" {
		s := model.CompensationDayClaimStatus(raw)
		if s != model.CompensationDayClaimOpen && s != model.CompensationDayClaimUsed && s != model.CompensationDayClaimWaived {
			response.Error(w, http.StatusBadRequest, "invalid status")
			return
		}
		status = &s
	}
	list, err := h.CompensationDayClaims.ListByUser(r.Context(), uid, status)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	if list == nil {
		list = []model.CompensationDayClaim{}
	}
	response.JSON(w, http.StatusOK, map[string]interface{}{"compensation_day_claims": list})
}

func (h *EmployeeHandler) WaiveCompensationDayClaim(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.parseEmployeeID(w, r)
	if !ok {
		return
	}
	claimID, err := strconv.Atoi(chi.URLParam(r, "claimId"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid claim id")
		return
	}
	if err := h.CompensationDayClaims.Waive(r.Context(), uid, claimID); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

Add routes in `internal/api/router.go`:

```go
r.Get("/employees/{id}/compensation-day-claims", eh.ListCompensationDayClaims)
r.Post("/employees/{id}/compensation-day-claims/{claimId}/waive", eh.WaiveCompensationDayClaim)
```

- [ ] **Step 5: Write handler tests**

Create `internal/api/handler/compensation_days_test.go` with focused tests for the validation helper:

```go
func TestValidateCompensationDayAbsenceDate(t *testing.T) {
	ctx := context.Background()
	st := stubHolidayStore{}

	if err := validateCompensationDayAbsenceDate(ctx, st, "2026-04-07", true); err == nil {
		t.Fatal("expected half-day error")
	}
	if err := validateCompensationDayAbsenceDate(ctx, st, "2026-04-04", false); err == nil {
		t.Fatal("expected weekend error")
	}
	holidayStore := stubHolidayStore{
		getForDate: func(_ context.Context, date string) (*model.Holiday, error) {
			if date == "2026-04-06" {
				return &model.Holiday{HolidayDate: date, Name: "Ostermontag"}, nil
			}
			return nil, nil
		},
	}
	if err := validateCompensationDayAbsenceDate(ctx, holidayStore, "2026-04-06", false); err == nil {
		t.Fatal("expected holiday error")
	}
	if err := validateCompensationDayAbsenceDate(ctx, st, "2026-04-07", false); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 6: Run API tests**

Run:

```bash
go test ./internal/api/handler -run 'TestValidateCompensationDayAbsenceDate|TestValidateVacationAbsenceDate' -count=1
```

Expected: PASS.

---

### Task 5: Balance, Export, And Team Overview Semantics

**Files:**
- Modify: `internal/service/daycalc/daycalc.go`
- Modify: `internal/service/export/rows.go`
- Modify: `internal/service/teamoverview/overview.go`
- Modify: `internal/api/handler/dashboard.go`
- Modify: `internal/api/router.go`
- Modify: `internal/service/teamoverview/overview_test.go`

- [ ] **Step 1: Ensure compensation days have no absence credit**

Modify `internal/service/daycalc/daycalc.go`.

`DailyTarget` should keep returning `daily` for `AbsenceCompensationDay` on weekdays:

```go
case model.AbsenceSick, model.AbsenceVacation, model.AbsenceOther, model.AbsenceCompensationDay:
	if abs.HalfDay {
		return daily / 2
	}
	return daily
```

`AbsenceCreditHours` must not include `AbsenceCompensationDay`:

```go
case model.AbsenceSick, model.AbsenceVacation, model.AbsenceOther:
	if abs.HalfDay {
		return daily / 2
	}
	return daily
```

- [ ] **Step 2: Label compensation days in export**

Modify `internal/service/export/rows.go` absence notes:

```go
case model.AbsenceCompensationDay:
	notes = "Ausgleichstag"
```

- [ ] **Step 3: Add open claim count to team overview**

Modify `internal/service/teamoverview/overview.go` `Deps`:

```go
CompensationDayClaims store.CompensationDayClaimStore
```

Modify `Row`:

```go
CompensationDayClaimsOpen int `json:"compensation_day_claims_open"`
```

During row build:

```go
var openCompensationDayClaims int
if d.CompensationDayClaims != nil {
	if n, err := d.CompensationDayClaims.CountOpen(ctx, u.ID); err == nil {
		openCompensationDayClaims = n
	}
}
```

Set in row:

```go
CompensationDayClaimsOpen: openCompensationDayClaims,
```

Modify `internal/api/handler/dashboard.go` `DashboardHandler`:

```go
CompensationDayClaims store.CompensationDayClaimStore
```

Modify `teamDeps()`:

```go
CompensationDayClaims: h.CompensationDayClaims,
```

Modify `internal/api/router.go` `DashboardHandler` construction:

```go
CompensationDayClaims: d.CompensationDayClaims,
```

- [ ] **Step 4: Add team overview test**

Add to `internal/service/teamoverview/overview_test.go`:

```go
func TestOverview_IncludesOpenCompensationDayClaims(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	us := sqlite.NewUserStore(db)
	claims := sqlite.NewCompensationDayClaimStore(db)
	u := &model.User{Username: "claimoverview", PasswordHash: "x", DisplayName: "Claim Overview", Role: model.RoleUser, Active: true}
	if err := us.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := claims.EnsureForWorkDate(ctx, u.ID, "2026-04-04", true); err != nil {
		t.Fatal(err)
	}
	if err := claims.EnsureForWorkDate(ctx, u.ID, "2026-04-05", true); err != nil {
		t.Fatal(err)
	}

	rows, err := Build(ctx, Deps{
		Users:                 us,
		WorkPeriods:           sqlite.NewWorkPeriodStore(db),
		Corrections:           sqlite.NewCorrectionStore(db),
		Absences:              sqlite.NewAbsenceStore(db),
		Holidays:              sqlite.NewHolidayStore(db),
		Closures:              sqlite.NewClosureDayStore(db),
		WeeklyHours:           sqlite.NewWeeklyHoursStore(db),
		Settings:              sqlite.NewSettingsStore(db),
		VacationEnt:           sqlite.NewVacationEntitlementStore(db),
		CompensationDayClaims: claims,
	}, "2026-04-01", 2026, time.Date(2026, 4, 10, 10, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].CompensationDayClaimsOpen != 2 {
		t.Fatalf("open compensation day claims want 2, got %d", rows[0].CompensationDayClaimsOpen)
	}
}
```

- [ ] **Step 5: Run service tests**

Run:

```bash
go test ./internal/service/daycalc ./internal/service/export ./internal/service/teamoverview -count=1
```

Expected: PASS.

---

### Task 6: Frontend API And Absences UI

**Files:**
- Modify: `web/src/types/api.ts`
- Modify: `web/src/api/management.ts`
- Modify: `web/src/views/absences/AbsencesView.vue`
- Modify: `web/src/views/DashboardView.vue`

- [ ] **Step 1: Extend TypeScript types**

Modify `web/src/types/api.ts`:

```ts
export interface Absence {
  id: number
  user_id: number
  absence_date: string
  absence_type: 'sick' | 'vacation' | 'other' | 'compensation_day'
  half_day: boolean
  created_by: number
  created_at: string
}

export type CompensationDayClaimStatus = 'open' | 'used' | 'waived'

export interface CompensationDayClaim {
  id: number
  user_id: number
  work_date: string
  status: CompensationDayClaimStatus
  used_absence_id: number | null
  created_at: string
  updated_at: string
}
```

Extend `TeamOverviewRow`:

```ts
compensation_day_claims_open: number
```

- [ ] **Step 2: Add management API functions**

Modify imports in `web/src/api/management.ts` to include `CompensationDayClaim` and `CompensationDayClaimStatus`.

Add:

```ts
export async function fetchEmployeeCompensationDayClaims(
  employeeId: number,
  status?: CompensationDayClaimStatus,
) {
  const { data } = await api.get<{ compensation_day_claims: CompensationDayClaim[] | null }>(
    `/employees/${employeeId}/compensation-day-claims`,
    { params: status ? { status } : undefined },
  )
  return data.compensation_day_claims ?? []
}

export async function waiveEmployeeCompensationDayClaim(employeeId: number, claimId: number) {
  await api.post(`/employees/${employeeId}/compensation-day-claims/${claimId}/waive`)
}
```

- [ ] **Step 3: Update absence type options and labels**

Modify `web/src/views/absences/AbsencesView.vue`:

```ts
const typeOptions = [
  { label: 'Krank', value: 'sick' as const },
  { label: 'Urlaub', value: 'vacation' as const },
  { label: 'Ausgleichstag', value: 'compensation_day' as const },
  { label: 'Sonstiges', value: 'other' as const },
]
```

Update the add type ref:

```ts
const addType = ref<'sick' | 'vacation' | 'other' | 'compensation_day'>('vacation')
```

Update `typeLabel`:

```ts
if (t === 'compensation_day') return 'Ausgleichstag'
```

- [ ] **Step 4: Validate compensation day before submit**

Import `fetchEmployeeCompensationDayClaims`.

In `submitAdd`, after computing `iso`, add a branch:

```ts
if (addType.value === 'compensation_day') {
  const dow = addDate.value.getDay()
  const pretty = formatGermanDate(iso)
  const weekday = germanWeekdayLong(addDate.value)
  if (addHalf.value) {
    toast.add({
      severity: 'error',
      summary: 'Ausgleichstag nicht möglich',
      detail: 'Halbe Ausgleichstage sind nicht möglich.',
      life: 8000,
    })
    return
  }
  if (dow === 0 || dow === 6) {
    toast.add({
      severity: 'error',
      summary: 'Ausgleichstag nicht möglich',
      detail: `Am ${weekday}, ${pretty}, kann kein Ausgleichstag gebucht werden — das ist ein Wochenende.`,
      life: 8000,
    })
    return
  }
  try {
    const hol = await fetchHolidays(addDate.value.getFullYear())
    const hit = hol.find((h) => h.holiday_date === iso)
    if (hit) {
      toast.add({
        severity: 'error',
        summary: 'Ausgleichstag nicht möglich',
        detail: `Am ${weekday}, ${pretty}, kann kein Ausgleichstag gebucht werden — „${hit.name}“ ist ein gesetzlicher Feiertag.`,
        life: 8000,
      })
      return
    }
    const claims = await fetchEmployeeCompensationDayClaims(addEmpId.value, 'open')
    if (claims.length === 0) {
      toast.add({
        severity: 'error',
        summary: 'Ausgleichstag nicht möglich',
        detail: 'Für diesen Mitarbeiter ist kein offener Ausgleichstag-Anspruch vorhanden.',
        life: 8000,
      })
      return
    }
  } catch {
    toast.add({
      severity: 'error',
      summary: 'Ausgleichstag',
      detail: 'Die Ausgleichstag-Ansprüche konnten nicht geprüft werden. Bitte erneut versuchen.',
      life: 8000,
    })
    return
  }
}
```

Set the submit error summary:

```ts
const failureSummary =
  addType.value === 'vacation'
    ? 'Urlaub nicht möglich'
    : addType.value === 'compensation_day'
      ? 'Ausgleichstag nicht möglich'
      : 'Speichern fehlgeschlagen'
```

- [ ] **Step 5: Display open claim count**

In `AbsencesView.vue`, add refs and a watcher to load open claims when employee or type changes:

```ts
const openCompensationDayClaims = ref<CompensationDayClaim[]>([])

async function loadOpenCompensationDayClaims() {
  if (addType.value !== 'compensation_day' || addEmpId.value == null) {
    openCompensationDayClaims.value = []
    return
  }
  openCompensationDayClaims.value = await fetchEmployeeCompensationDayClaims(addEmpId.value, 'open')
}

watch([addEmpId, addType], () => {
  void loadOpenCompensationDayClaims()
})
```

Call `loadOpenCompensationDayClaims()` in `openAdd()` after setting defaults when the default type is changed to `compensation_day` in the future. Add the count below the type select:

```vue
<small v-if="addType === 'compensation_day'" class="hint">
  Offene Ausgleichstag-Ansprüche: {{ openCompensationDayClaims.length }}
</small>
```

Modify `web/src/views/DashboardView.vue` team overview columns to show the open count:

```vue
<Column field="compensation_day_claims_open" header="Ausgleich offen" sortable style="min-width: 8rem">
  <template #body="{ data }">{{ data.compensation_day_claims_open }}</template>
</Column>
```

- [ ] **Step 6: Run frontend build**

Run:

```bash
npm --prefix web run build
```

Expected: PASS.

---

### Task 7: Full Verification

**Files:**
- All changed files

- [ ] **Step 1: Run Go tests**

Run:

```bash
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 2: Run frontend build**

Run:

```bash
npm --prefix web run build
```

Expected: PASS.

- [ ] **Step 3: Run lint diagnostics**

Use Cursor diagnostics or `ReadLints` for:

- `internal/api/handler/employees.go`
- `internal/model/absence.go`
- `internal/model/compensation_day_claim.go`
- `internal/store/interfaces.go`
- `internal/store/sqlite/compensation_day_claims.go`
- `web/src/types/api.ts`
- `web/src/api/management.ts`
- `web/src/views/absences/AbsencesView.vue`

Expected: no new diagnostics.

- [ ] **Step 4: Manual smoke test**

Use the app as Leitung/Superadmin:

1. Create or import a completed one-hour work period on a Saturday for an employee.
2. Open that employee's compensation-day claims endpoint or UI and confirm one open claim.
3. Book an `Ausgleichstag` on a normal weekday.
4. Confirm the claim becomes `used`.
5. Confirm the hours balance drops by the employee's day target.
6. Delete the Ausgleichstag absence.
7. Confirm the claim is open again.
8. Waive the open claim.
9. Confirm the claim becomes `waived` and the hours balance does not change.

---

## Self-Review Checklist

- The API value is consistently `compensation_day`.
- The claim table and store use `compensation_day_claims`.
- Weekend work hours remain in the normal hours balance.
- Taking a compensation day adds no absence credit and therefore consumes the daily target from overtime.
- Vacation calculations still count only `AbsenceVacation`.
- Open claims can be used, reopened after deletion, or waived.
- Tests cover weekend generation, booking validation, balance behavior, and waiver behavior.
