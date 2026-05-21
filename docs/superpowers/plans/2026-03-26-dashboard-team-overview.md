# Dashboard Team‑Uebersicht (Leitung) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eine schnelle, leitungs‑spezifische Dashboard‑Uebersicht mit Stundensaldo (seit Stichtag) und Urlaubssalden pro aktivem Mitarbeiter.

**Architecture:** Neuer Leitung‑Endpoint liefert aggregierte Team‑Rows (Stunden + Urlaub) in einem Call. Backend nutzt Export‑tageslogik (Ziel/Soll inkl. Feiertage/Schliessungen) plus Korrekturen. UI rendert eine kompakte Tabelle mit Suche/Sortierung und einem Stichtag‑Picker (nur lokal).

**Tech Stack:** Go (Chi), SQLite, Vue 3 + PrimeVue, existing stores/services.

---

## File Structure (geplante Aenderungen)
- Modify: `internal/api/router.go` (Route fuer Team‑Overview)
- Modify: `internal/api/handler/settings.go` (neuer Settings‑Key erlauben)
- Create: `internal/api/handler/dashboard.go` (Team‑Overview Handler)
- Create: `internal/service/teamoverview/overview.go` (Aggregation)
- Create: `internal/service/teamoverview/overview_test.go`
- Modify: `internal/service/export/rows.go` (shared helpers extrahieren)
- Create: `internal/service/daycalc/daycalc.go` (gemeinsame Logik: DailyTarget + NetHours)
- Create: `internal/service/daycalc/daycalc_test.go`
- Modify: `internal/store/interfaces.go` (falls neuer Store‑Interface Typ gebraucht wird)
- Modify: `web/src/api/management.ts` (API call fuer Leitung)
- Modify: `web/src/views/DashboardView.vue` (neuer Abschnitt + Suche + Stichtag)
- Optional: `README.md` (kurzer Hinweis zum neuen Setting)

---

### Task 1: Shared Tageslogik extrahieren (TDD)

**Files:**
- Create: `internal/service/daycalc/daycalc.go`
- Create: `internal/service/daycalc/daycalc_test.go`
- Modify: `internal/service/export/rows.go`

- [ ] **Step 1: Write failing tests for daycalc**

```go
func TestDailyTarget_AbsenceAndHalfDay(t *testing.T) {
	daily := 8.0
	day := time.Date(2026, 3, 10, 0, 0, 0, 0, time.Local)
	abs := &model.Absence{AbsenceType: model.AbsenceVacation, HalfDay: true}
	got := daycalc.DailyTarget(day, daily, nil, abs, nil)
	if got != 4.0 {
		t.Fatalf("expected 4.0, got %v", got)
	}
}

func TestNetHours_RoundingAndBreaks(t *testing.T) {
	wps := []model.WorkPeriod{
		{PunchIn: t("2026-03-10T08:00:00Z"), PunchOut: ptr(t("2026-03-10T12:00:00Z")), IsBreak: false},
		{PunchIn: t("2026-03-10T12:00:00Z"), PunchOut: ptr(t("2026-03-10T12:30:00Z")), IsBreak: true},
	}
	breakRules := []model.BreakRule{{AfterHours: 6, Minutes: 30}}
	got := daycalc.NetHours(wps, breakRules, 15)
	if got <= 0 {
		t.Fatalf("expected positive net hours, got %v", got)
	}
}

func t(s string) time.Time { tt, _ := time.Parse(time.RFC3339, s); return tt }
func ptr(t time.Time) *time.Time { return &t }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/daycalc -v`  
Expected: FAIL (package missing)

- [ ] **Step 3: Implement daycalc helpers**

```go
package daycalc

func DailyTarget(day time.Time, daily float64, hol *model.Holiday, abs *model.Absence, clo *model.ClosureDay) float64 { ... }

// NetHours must mirror export: gross - deduction via timecalc.CalcBreakDeduction, then RoundDown.
func NetHours(wps []model.WorkPeriod, breakRules []model.BreakRule, roundMin int) float64 { ... }
```

- [ ] **Step 4: Wire into export**

Replace `dailyTarget(...)` and inline net calculation in `export.BuildDayRows` with `daycalc.DailyTarget` and `daycalc.NetHours` to keep logic identical.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/service/daycalc -v`  
Run: `go test ./internal/service/export -v`  
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/service/daycalc internal/service/export/rows.go
git commit -m "refactor: extract shared daycalc helpers"
```

---

### Task 2: Team‑Overview Aggregation Service (TDD)

**Files:**
- Create: `internal/service/teamoverview/overview.go`
- Create: `internal/service/teamoverview/overview_test.go`

- [ ] **Step 1: Write failing tests for aggregation**

```go
func TestOverview_HoursBalanceSinceAsOf(t *testing.T) {
	t.Fatal("implement real case: hours balance since as_of to yesterday")
}
func TestOverview_VacationPlannedAndFree(t *testing.T) {
	t.Fatal("implement real case: planned (> today) vs taken (<= today) with half days")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/teamoverview -v`  
Expected: FAIL (package missing)

- [ ] **Step 3: Implement aggregation**

Key steps:
- Determine `today` and `yesterday` in `time.Local`.
- If `as_of` > yesterday → **hours_balance = 0**, vacation still computed for `vacation_year`.
- Fetch active users via `UserStore.List(ctx, true)` and filter `role != superadmin`.
- Work periods: `ListByUserDateRange` once per user (as_of → yesterday).
- Corrections: `CorrectionStore.ListByUser` (same range) and map latest per `work_period_id`.
- Apply corrections by replacing `PunchIn`/`PunchOut` on a copy of each work period.
- Load settings (`rounding_minutes`, `break_rules`) via `SettingsStore`.
- For each day in range:
  - Get absences, holidays, closures, weekly hours.
  - `net := daycalc.NetHours(correctedPeriodsForDay, breakRules, roundMin)`
  - `target := daycalc.DailyTarget(...)`
  - accumulate `balance += net - target`
- Vacation:
  - `vacation_year` default = current year (local)
  - entitlement: `VacationEntitlementStore.GetForDate(userID, fmt.Sprintf("%d-06-15", year))`
  - list absences `year-01-01` → `year-12-31`
  - `taken` = vacation days where `absence_date <= today` (half days = 0.5)
  - `planned` = vacation days where `absence_date > today` (half days = 0.5)
  - `remaining_total = entitlement - taken + carryover(0)`
  - `free = remaining_total - planned`

- [ ] **Step 4: Run tests**

Run: `go test ./internal/service/teamoverview -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/teamoverview
git commit -m "feat: add team overview aggregation service"
```

---

### Task 3: API Handler + Route (TDD)

**Files:**
- Create: `internal/api/handler/dashboard.go`
- Modify: `internal/api/router.go`
- Modify: `internal/api/handler/settings.go`

- [ ] **Step 1: Write failing handler tests**

```go
func TestTeamOverview_RequiresLeitung(t *testing.T) {
	t.Fatal("implement: requires leitung/superadmin")
}
func TestTeamOverview_ResponseShape(t *testing.T) {
	t.Fatal("implement: response envelope + row fields")
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/api/handler -v`  
Expected: FAIL

- [ ] **Step 3: Implement handler + route**

Handler:
- Parse `as_of` (YYYY-MM-DD), default from setting `dashboard.team_overview.as_of_default`.
- If setting missing: default `YYYY-01-01` (current year, local).
- Parse `vacation_year` (int, default current year).
- Call `teamoverview.Build(...)` and return `{ as_of, vacation_year, rows }`.
- Validate `as_of` format and `vacation_year` range (invalid -> HTTP 400).
- Response rows must include: `hours_balance`, `vacation_planned`, `vacation_free`, `vacation_remaining_total`,
  `vacation_carryover`, `vacation_entitlement`, `vacation_taken`, `id`, `display_name`.

Router:
- Add `r.Get("/dashboard/team-overview", dh.TeamOverview)` in Leitung group.

Settings:
- Add `dashboard.team_overview.as_of_default` to `allowedSettingKeys`.

Handler deps:
- Wire `DashboardHandler` via `api.Deps` in `internal/api/router.go` (Users, WorkPeriods, Corrections, Absences, WeeklyHours, Holidays, Closures, Settings, VacationEnt).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/api/handler -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/handler/dashboard.go internal/api/router.go internal/api/handler/settings.go
git commit -m "feat: add team overview dashboard endpoint"
```

---

### Task 4: Frontend API + Dashboard UI

**Files:**
- Modify: `web/src/api/management.ts`
- Modify: `web/src/types/api.ts`
- Modify: `web/src/views/DashboardView.vue`

- [ ] **Step 1: Add API function**

```ts
export async function fetchTeamOverview(asOf: string, year: number) {
  const { data } = await api.get<{ as_of: string; vacation_year: number; rows: TeamOverviewRow[] }>(
    '/dashboard/team-overview',
    { params: { as_of: asOf, vacation_year: year } },
  )
  return data
}
```

- [ ] **Step 2: Update DashboardView**

UI:
- Add new Card section for Leitung: table with columns (Name, Stundensaldo, Urlaub geplant, Urlaub frei, Rest gesamt).
- Add Textsuche (InputText) bound to DataTable global filter.
- Add Stichtag DatePicker (local state, not persisted).
- Render Tooltip for Rest gesamt (Entitlement/Taken/Carryover).
- Tooltiptext: „Rest gesamt / Uebertrag / Anspruch / Genommen“.
- Default sort by Name.
- Fehlerfall: dezente Meldung in der Karte, restliches Dashboard bleibt sichtbar.
- Anzeige nur fuer Leitung/Superadmin (existing `isLeitung` guard).
- Tabelle soll kompakt und scrollbar sein.
- Initialer Call: `as_of` aus Response verwenden (falls leer: fallback auf `YYYY-01-01` lokal).
- Sortierbarkeit: Name + Saldo + Urlaub‑Spalten sortierbar.

- [ ] **Step 3: Run frontend build (für Embedded UI)**

Run: `cd web && npm run build && make sync-web-dist`  
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add web/src/api/management.ts web/src/types/api.ts web/src/views/DashboardView.vue
git commit -m "feat: add dashboard team overview table"
```

---

### Task 5: Optional docs

**Files:**
- Optional: `README.md`

- [ ] **Step 1: Add short note for setting**

```md
dashboard.team_overview.as_of_default (YYYY-MM-DD)
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: note team overview stichtag setting"
```

---

## Plan Review Loop
After writing the plan, dispatch a plan-document-reviewer subagent with:
- Plan: `docs/superpowers/plans/2026-03-26-dashboard-team-overview.md`
- Spec: `docs/superpowers/specs/2026-03-26-dashboard-team-overview-design.md`
Fix issues and re-dispatch until approved (max 3 iterations).

## Execution Handoff
Plan complete and saved to `docs/superpowers/plans/2026-03-26-dashboard-team-overview.md`. Two execution options:

1. **Subagent-Driven (recommended)** - I dispatch a fresh subagent per task, review between tasks, fast iteration  
2. **Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints  

Which approach?
