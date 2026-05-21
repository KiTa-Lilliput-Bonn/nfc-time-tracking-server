<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import DatePicker from 'primevue/datepicker'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import InputSwitch from 'primevue/inputswitch'
import Select from 'primevue/select'
import { useToast } from 'primevue/usetoast'

import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { fetchEmployees } from '@/api/employees'
import { fetchGroups } from '@/api/groups'
import { createEmployeeAbsence, fetchClosureDays, fetchTeamOverview, fetchHolidays } from '@/api/management'
import VacationBalanceBar from '@/components/VacationBalanceBar.vue'
import { fetchMeBalance, fetchMeSchedule, fetchMeTimes, fetchMeVacation } from '@/api/me'
import { addDays, formatGermanDate, toISODateLocal } from '@/utils/dates'
import { getApiErrorMessage } from '@/utils/apiError'
import { friendlyAbsenceCreateError } from '@/utils/absenceErrors'
import {
  countSkippedNonWorkdays,
  enumerateInclusiveCalendarISO,
  enumerateWorkdayISO,
  holidayDateSetForRange,
  normalizeISODate,
} from '@/utils/workdays'
import { focusSelectFilterOnShow, openSelectDropdown } from '@/utils/selectFilterFocus'
import type { Employee, Schedule, TeamOverviewRow, UserGroup, VacationBalance } from '@/types/api'
import type { ClosureDay } from '@/types/api'

const auth = useAuthStore()
const router = useRouter()
const toast = useToast()

function goToEmployeeDetailById(id: number) {
  void router.push({ name: 'employee-detail', params: { id: String(id) } })
}
function onTeamOverviewRowClick(e: { data: TeamOverviewRow }) {
  goToEmployeeDetailById(e.data.id)
}
function onTeamEmployeeRowClick(e: { data: Employee }) {
  goToEmployeeDetailById(e.data.id)
}

const loading = ref(true)
const err = ref('')

const todayNetHours = ref(0)
const todayPeriodsCount = ref(0)
const monthBalance = ref<{ worked: number; target: number; balance: number } | null>(null)
const vacationBalance = ref<VacationBalance | null>(null)

const nextShift = ref<Schedule | null>(null)
const employees = ref<Employee[]>([])
const groups = ref<UserGroup[]>([])
const closures = ref<ClosureDay[]>([])
const closureDateSet = computed(() => new Set(closures.value.map((c) => normalizeISODate(c.closure_date))))

const isLeitung = computed(() => auth.role === 'leitung' || auth.role === 'superadmin')

const teamOverviewRows = ref<TeamOverviewRow[]>([])
const teamOverviewLoading = ref(false)
const teamOverviewErr = ref('')
const teamOverviewNameFilter = ref('')
/** Urlaubsjahrbuckets der Team-Übersicht: Kalenderjahr von heute. */
const teamOverviewVacationYear = computed(() => new Date().getFullYear())
/** Frühester Auswertungsbeginn Stundensaldo im Team (ISO), vom Server. */
const teamOverviewHoursFromISO = ref('')
const yesterdayDisplayISO = computed(() => {
  const d = new Date()
  d.setDate(d.getDate() - 1)
  return toISODateLocal(d)
})

const teamOverviewFiltered = computed(() => {
  const q = teamOverviewNameFilter.value.trim().toLowerCase()
  if (!q) return teamOverviewRows.value
  return teamOverviewRows.value.filter((r) => r.display_name.toLowerCase().includes(q))
})

const sortedGroupsList = computed(() =>
  [...groups.value].sort((a, b) => a.sort_order - b.sort_order || a.id - b.id),
)

/** Gemeinsamer Gruppenfilter für Team-Übersicht und Team-Stammdaten („Alle“ / eine Gruppe / ohne Gruppe). */
const teamGroupFilter = ref<'all' | 'none' | number>('all')

const teamGroupFilterOptions = computed(() => {
  const opts: { label: string; value: 'all' | 'none' | number }[] = [{ label: 'Alle Gruppen', value: 'all' }]
  for (const g of sortedGroupsList.value) {
    opts.push({ label: g.name, value: g.id })
  }
  opts.push({ label: 'Ohne Gruppe', value: 'none' })
  return opts
})

const employeeById = computed(() => new Map(employees.value.map((e) => [e.id, e])))
const knownGroupIds = computed(() => new Set(sortedGroupsList.value.map((g) => g.id)))

function matchesTeamGroupFilter(userId: number): boolean {
  const f = teamGroupFilter.value
  if (f === 'all') return true
  const e = employeeById.value.get(userId)
  if (f === 'none') {
    if (!e) return true
    return e.group_id == null || !knownGroupIds.value.has(e.group_id)
  }
  return e?.group_id === f
}

/** Team-Übersicht: Namensfilter + Gruppenfilter */
const teamOverviewTableRows = computed(() =>
  teamOverviewFiltered.value.filter((r) => matchesTeamGroupFilter(r.id)),
)

/** Team-Stammdaten: nur Gruppenfilter */
const teamEmployeesTableRows = computed(() => {
  const f = teamGroupFilter.value
  if (f === 'all') return employees.value
  if (f === 'none') {
    return employees.value.filter(
      (e) => e.group_id == null || !knownGroupIds.value.has(e.group_id),
    )
  }
  return employees.value.filter((e) => e.group_id === f)
})

watch(sortedGroupsList, () => {
  const v = teamGroupFilter.value
  if (typeof v === 'number' && !sortedGroupsList.value.some((g) => g.id === v)) {
    teamGroupFilter.value = 'all'
  }
})

/** Für Tooltip: Zeilenwert aus Team-Overview; sonst Stammdaten (ältere API ohne Feld oder 0). */
function openingVacationDaysForTooltip(row: TeamOverviewRow): number {
  const fromRow = Number(row.vacation_opening_days ?? 0)
  const fromEmp = Number(employeeById.value.get(row.id)?.opening_vacation_days ?? 0)
  if (Number.isFinite(fromRow) && Math.abs(fromRow) > 1e-9) return fromRow
  if (Number.isFinite(fromEmp) && Math.abs(fromEmp) > 1e-9) return fromEmp
  return Number.isFinite(fromRow) ? fromRow : 0
}

function restGesamtTitle(row: TeamOverviewRow): string {
  const fmt = (n: number) => (Number.isFinite(n) ? n.toFixed(1) : '—')
  const open = openingVacationDaysForTooltip(row)
  const parts: string[] = [`Rest gesamt: ${fmt(row.vacation_remaining_total)}`]
  if (Math.abs(open) > 1e-9) {
    parts.push(`Startsaldo Urlaub: ${fmt(open)}`)
  }
  parts.push(
    `Übertrag (Vorjahr): ${fmt(row.vacation_carryover)}`,
    `Anspruch (aktuelles Jahr): ${fmt(row.vacation_entitlement)}`,
    `Genommen (aktuelles Jahr): ${fmt(row.vacation_taken)}`,
  )
  return parts.join(' · ')
}

const teamVacationBar = computed(() => (row: TeamOverviewRow) => {
  const opening = Number(row.vacation_opening_days ?? 0)
  const total = opening + Number(row.vacation_carryover ?? 0) + Number(row.vacation_entitlement ?? 0)
  const taken = Number(row.vacation_taken ?? 0)
  const planned = Number(row.vacation_planned ?? 0)
  const open = total - taken - planned
  if (!Number.isFinite(total) || total <= 0) {
    return { total: 0, taken, planned, open, pctTaken: 0, pctPlanned: 0, pctOpen: 0 }
  }
  let a = (100 * taken) / total
  let b = (100 * planned) / total
  let c = (100 * Math.max(0, open)) / total
  const sum = a + b + c
  if (sum > 100.001) {
    a = (a / sum) * 100
    b = (b / sum) * 100
    c = (c / sum) * 100
  }
  return { total, taken, planned, open: Math.max(0, open), pctTaken: a, pctPlanned: b, pctOpen: c }
})

function teamVacationBarTitle(row: TeamOverviewRow): string {
  const b = teamVacationBar.value(row)
  const fmt = (n: number) => (Number.isFinite(n) ? n.toFixed(1) : '—')
  return [
    `Gesamt (Stand 01.01.): ${fmt(b.total)} Tg.`,
    `Genommen: ${fmt(b.taken)} Tg.`,
    `Geplant: ${fmt(b.planned)} Tg.`,
    `Noch offen: ${fmt(b.open)} Tg.`,
  ].join(' · ')
}

function segmentHours(punchIn: string, punchOut: string | null): number {
  if (!punchOut) return 0
  const a = new Date(punchIn).getTime()
  const b = new Date(punchOut).getTime()
  if (b <= a) return 0
  return Math.round(((b - a) / 3_600_000) * 100) / 100
}

async function loadTeamOverview() {
  if (!isLeitung.value) return
  teamOverviewLoading.value = true
  teamOverviewErr.value = ''
  try {
    const data = await fetchTeamOverview(teamOverviewVacationYear.value)
    teamOverviewHoursFromISO.value = (data.as_of && data.as_of.trim()) || ''
    teamOverviewRows.value = data.rows
  } catch {
    teamOverviewErr.value = 'Team-Übersicht konnte nicht geladen werden.'
  } finally {
    teamOverviewLoading.value = false
  }
}

async function load() {
  loading.value = true
  err.value = ''
  const today = toISODateLocal(new Date())
  const now = new Date()
  const y = now.getFullYear()
  const m = now.getMonth() + 1

  try {
    const [times, bal, vac, leadData] = await Promise.all([
      fetchMeTimes(today, today),
      fetchMeBalance(m, y),
      fetchMeVacation(),
      isLeitung.value
        ? Promise.all([
            fetchEmployees().catch((): Employee[] => []),
            fetchGroups().catch((): UserGroup[] => []),
            fetchClosureDays().catch((): ClosureDay[] => []),
          ]).then(([emps, grp, cls]) => ({ employees: emps, groups: grp, closures: cls }))
        : Promise.resolve({ employees: [] as Employee[], groups: [] as UserGroup[], closures: [] as ClosureDay[] }),
    ])

    todayPeriodsCount.value = times.work_periods.length
    let net = 0
    for (const p of times.work_periods) {
      if (p.is_break) continue
      if (p.punch_out) {
        net += segmentHours(p.punch_in, p.punch_out)
      } else {
        const start = new Date(p.punch_in).getTime()
        net += Math.round(((now.getTime() - start) / 3_600_000) * 100) / 100
      }
    }
    todayNetHours.value = Math.round(net * 100) / 100

    monthBalance.value = {
      worked: bal.worked_hours,
      target: bal.target_hours,
      balance: bal.balance_hours,
    }
    vacationBalance.value = vac

    const to = toISODateLocal(addDays(now, 21))
    try {
      const sch = await fetchMeSchedule(today, to)
      const upcoming = sch.schedules
        .filter((s) => s.schedule_date >= today)
        .sort((a, b) => a.schedule_date.localeCompare(b.schedule_date) || a.shift_start.localeCompare(b.shift_start))
      nextShift.value = upcoming[0] ?? null
    } catch {
      nextShift.value = null
    }

    employees.value = leadData.employees
    groups.value = leadData.groups
    closures.value = leadData.closures
  } catch {
    err.value = 'Dashboard-Daten konnten nicht geladen werden.'
  } finally {
    loading.value = false
  }

  // Unabhängig von Fehlern oben (z. B. /me/vacation, /me/balance): Team-Übersicht immer laden
  if (isLeitung.value) {
    await loadTeamOverview()
  }
}

onMounted(load)

// --- Schnell-Krankmeldung (Leitung) ---

const showQuickSick = ref(false)
const quickSickEmpId = ref<number | null>(null)
const quickSickFixedNonWorkSet = computed(() => {
  const e = employees.value.find((x) => x.id === quickSickEmpId.value)
  return new Set(e?.fixed_non_work_weekdays ?? [])
})
const quickSickFrom = ref<Date>(new Date())
const quickSickTo = ref<Date>(new Date())
const quickSickHalfDay = ref(false)
const quickSickSaving = ref(false)
const quickSickEmpSelect = ref<any>(null)

const quickSickEmployeeOptions = computed(() =>
  employees.value
    .filter((e) => e.role !== 'superadmin')
    .map((e) => ({ label: e.display_name, value: e.id })),
)

function openQuickSick() {
  const today = new Date()
  quickSickEmpId.value = quickSickEmployeeOptions.value[0]?.value ?? null
  quickSickFrom.value = new Date(today)
  quickSickTo.value = new Date(today)
  quickSickHalfDay.value = false
  showQuickSick.value = true
  // Dialog transition + overlay init; open employee dropdown so search is immediately active.
  setTimeout(() => openSelectDropdown(quickSickEmpSelect.value), 50)
}

function normalizeQuickRange(): [Date, Date] {
  const a = new Date(quickSickFrom.value.getFullYear(), quickSickFrom.value.getMonth(), quickSickFrom.value.getDate())
  const b = new Date(quickSickTo.value.getFullYear(), quickSickTo.value.getMonth(), quickSickTo.value.getDate())
  if (a.getTime() <= b.getTime()) return [a, b]
  return [b, a]
}

const quickSickIsSingleDay = computed(
  () => toISODateLocal(quickSickFrom.value) === toISODateLocal(quickSickTo.value),
)
watch(
  () => toISODateLocal(quickSickFrom.value),
  () => {
    const d = quickSickFrom.value
    quickSickTo.value = new Date(d.getFullYear(), d.getMonth(), d.getDate())
  },
)
watch([quickSickFrom, quickSickTo], () => {
  if (!quickSickIsSingleDay.value) quickSickHalfDay.value = false
})

async function submitQuickSick() {
  if (quickSickEmpId.value == null) return
  quickSickSaving.value = true
  try {
    const [df, dt] = normalizeQuickRange()
    const allCal = enumerateInclusiveCalendarISO(df, dt)
    let holidaySet: Set<string>
    try {
      holidaySet = await holidayDateSetForRange(df, dt, fetchHolidays)
    } catch {
      toast.add({
        group: 'dashboard-center',
        severity: 'error',
        summary: 'Feiertage',
        detail: 'Die Feiertagsliste konnte nicht geladen werden. Bitte erneut versuchen.',
        life: 10000,
      })
      return
    }

    const datesToBook = enumerateWorkdayISO(df, dt, holidaySet, closureDateSet.value, quickSickFixedNonWorkSet.value)
    const skipped = countSkippedNonWorkdays(
      allCal,
      holidaySet,
      closureDateSet.value,
      quickSickFixedNonWorkSet.value,
    )

    if (datesToBook.length === 0) {
      toast.add({
        group: 'dashboard-center',
        severity: 'info',
        summary: 'Keine Arbeitstage',
        detail: 'Im gewählten Zeitraum gibt es keinen Arbeitstag (nur Wochenende/Feiertag/Schließtag/fix frei).',
        life: 10000,
      })
      return
    }

    if (quickSickHalfDay.value && datesToBook.length !== 1) {
      toast.add({
        group: 'dashboard-center',
        severity: 'error',
        summary: 'Halbtag nicht möglich',
        detail: 'Halbtags-Krankmeldung ist nur an einem einzelnen Arbeitstag möglich.',
        life: 10000,
      })
      return
    }

    let ok = 0
    const failed: { iso: string; msg: string }[] = []
    for (const iso of datesToBook) {
      try {
        await createEmployeeAbsence(quickSickEmpId.value, {
          absence_date: iso,
          absence_type: 'sick',
          half_day: quickSickHalfDay.value,
        })
        ok++
      } catch (e) {
        const apiMsg = getApiErrorMessage(e)
        failed.push({
          iso,
          msg:
            friendlyAbsenceCreateError({ apiMessage: apiMsg, isoDate: iso, absenceLabel: 'Krankmeldung' }) ??
            'Die Krankmeldung konnte nicht gespeichert werden. Bitte Eingabe prüfen und erneut versuchen.',
        })
      }
    }

    if (ok > 0) {
      const summary = ok === 1 ? 'Krankmeldung eingetragen' : `${ok} Kranktage eingetragen`
      toast.add({ group: 'dashboard-center', severity: 'success', summary, life: 10000 })
    }
    if (skipped > 0) {
      toast.add({
        group: 'dashboard-center',
        severity: 'info',
        summary: 'Übersprungen',
        detail: `${skipped} Kalendertag(e) waren Wochenende, Feiertag, Schließtag oder fix frei und wurden nicht eingetragen.`,
        life: 10000,
      })
    }
    if (failed.length > 0) {
      const detail =
        failed.length === 1
          ? `${formatGermanDate(failed[0].iso)}: ${failed[0].msg}`
          : `${failed.length} Tag(e) konnten nicht eingetragen werden (z. B. bereits vorhanden).`
      toast.add({
        group: 'dashboard-center',
        severity: ok > 0 ? 'warn' : 'error',
        summary: 'Teilweise fehlgeschlagen',
        detail,
        life: 10000,
      })
    }

    showQuickSick.value = false
  } finally {
    quickSickSaving.value = false
  }
}
</script>

<template>
  <div class="dash">
    <p v-if="err" class="err">{{ err }}</p>
    <div v-if="loading" class="muted">Laden…</div>
    <template v-else>
      <p class="welcome">
        Hallo <strong>{{ auth.user?.display_name }}</strong>
      </p>
      <div class="cards">
        <Card v-if="isLeitung">
          <template #title>Krankmeldung</template>
          <template #content>
            <p class="stat">Krankmeldung für Mitarbeiter*in hinzufügen</p>
            <Button
              label="Jetzt eintragen"
              icon="pi pi-plus"
              size="small"
              :disabled="quickSickEmployeeOptions.length === 0"
              @click="openQuickSick"
            />
            <p v-if="employees.length === 0" class="sub" style="margin-top: 0.35rem">Keine Mitarbeitenden geladen.</p>
            <p v-else-if="quickSickEmployeeOptions.length === 0" class="sub" style="margin-top: 0.35rem">
              Keine auswählbaren Mitarbeitenden (Superadmin ist ausgeschlossen).
            </p>
          </template>
        </Card>
        <Card>
          <template #title>Heute</template>
          <template #content>
            <p class="stat">Netto-Arbeitszeit (ca.): <strong>{{ todayNetHours }} h</strong></p>
            <p class="sub">Stempel-Segmente: {{ todayPeriodsCount }}</p>
          </template>
        </Card>
        <Card v-if="monthBalance">
          <template #title>Aktueller Monat</template>
          <template #content>
            <p class="stat">
              Ist <strong>{{ monthBalance.worked.toFixed(2) }} h</strong> · Soll
              <strong>{{ monthBalance.target.toFixed(2) }} h</strong>
            </p>
            <p class="sub" :class="monthBalance.balance >= 0 ? 'pos' : 'neg'">
              Saldo: <strong>{{ monthBalance.balance.toFixed(2) }} h</strong>
            </p>
          </template>
        </Card>
        <Card v-if="vacationBalance">
          <template #title>Urlaub {{ vacationBalance.year }}</template>
          <template #content>
            <VacationBalanceBar :balance="vacationBalance" />
          </template>
        </Card>
        <Card>
          <template #title>Nächste Schicht</template>
          <template #content>
            <p v-if="nextShift" class="stat">
              {{ formatGermanDate(nextShift.schedule_date) }} · {{ nextShift.shift_start }} –
              {{ nextShift.shift_end }}
            </p>
            <p v-else class="sub">Kein Eintrag in den nächsten Wochen.</p>
          </template>
        </Card>
      </div>

      <Dialog v-model:visible="showQuickSick" header="Krankmeldung" modal :style="{ width: '460px' }">
        <div class="form">
          <label class="field-label">Mitarbeiter</label>
          <Select
            ref="quickSickEmpSelect"
            v-model="quickSickEmpId"
            :options="quickSickEmployeeOptions"
            option-label="label"
            option-value="value"
            class="w"
            filter
            @show="focusSelectFilterOnShow"
          />
          <label class="field-label">Von</label>
          <DatePicker v-model="quickSickFrom" date-format="dd.mm.yy" show-icon class="w" />
          <label class="field-label">Bis</label>
          <DatePicker v-model="quickSickTo" date-format="dd.mm.yy" show-icon class="w" :min-date="quickSickFrom" />
          <p class="sub">Wochenenden, Feiertage und Schließtage im Zeitraum werden übersprungen.</p>
          <label class="row">
            <span>Halber Tag</span>
            <InputSwitch v-model="quickSickHalfDay" :disabled="!quickSickIsSingleDay" />
          </label>
        </div>
        <template #footer>
          <Button label="Abbrechen" severity="secondary" text @click="showQuickSick = false" />
          <Button
            label="Speichern"
            :loading="quickSickSaving"
            :disabled="quickSickEmpId == null"
            @click="submitQuickSick"
          />
        </template>
      </Dialog>

      <Card v-if="isLeitung" class="team-overview-card">
        <template #title>Team-Übersicht</template>
        <template #content>
          <p v-if="teamOverviewErr" class="team-overview-err">{{ teamOverviewErr }}</p>
          <div class="team-overview-toolbar">
            <div class="toolbar-field toolbar-field--group">
              <label class="field-label" for="dash-team-group-overview">Gruppe</label>
              <Select
                id="dash-team-group-overview"
                v-model="teamGroupFilter"
                :options="teamGroupFilterOptions"
                option-label="label"
                option-value="value"
                class="group-filter-select"
              />
            </div>
            <InputText
              v-model="teamOverviewNameFilter"
              class="name-filter"
              placeholder="Name suchen…"
              type="search"
            />
          </div>
          <p v-if="teamOverviewHoursFromISO" class="sub team-overview-hours-range">
            Stundensaldo je Person: vom frühesten gespeicherten Stundensoll (Wochenstunden, „gültig ab“) bzw. ohne
            solchen Eintrag ab dem 1. Januar des Jahres, in dem der letzte volle Auswertungstag liegt, bis
            {{ formatGermanDate(yesterdayDisplayISO) }}. Im Team ist der früheste Beginn der Auswertung
            {{ formatGermanDate(teamOverviewHoursFromISO) }}.
          </p>
          <p class="sub team-overview-vac-hint">
            <strong>Anspruch</strong> bezieht sich nur auf das laufende Kalenderjahr (heute);
            <strong>Genommen</strong> nur im laufenden Kalenderjahr bis heute.
            <strong>Geplant</strong>: alle eingetragenen Urlaubstage nach heute.
            Beim Hover über die <strong>Zahl</strong> bei Rest gesamt siehst du die Aufschlüsselung inkl. Vorjahresübertrag.
          </p>
          <p v-if="teamOverviewLoading" class="muted">Laden…</p>
          <DataTable
            v-else
            :value="teamOverviewTableRows"
            data-key="id"
            size="small"
            striped-rows
            sort-field="display_name"
            :sort-order="1"
            scrollable
            scroll-height="380px"
            row-hover
            class="employee-nav-table"
            @row-click="onTeamOverviewRowClick"
          >
            <template #empty>
              <span class="muted">Keine Einträge für diese Auswahl.</span>
            </template>
            <Column field="display_name" header="Name" sortable style="min-width: 10rem" />
            <Column field="hours_balance" header="Stundensaldo" sortable style="min-width: 7rem">
              <template #body="{ data }: { data: TeamOverviewRow }">
                {{ data.hours_balance.toFixed(2) }}
              </template>
            </Column>
            <Column
              field="compensation_day_claims_open"
              header="Ausgleich offen"
              sortable
              style="min-width: 7rem"
            >
              <template #body="{ data }: { data: TeamOverviewRow }">
                {{ data.compensation_day_claims_open }}
              </template>
            </Column>
            <Column field="vacation_remaining_total" header="Urlaub" sortable style="min-width: 16rem">
              <template #body="{ data }: { data: TeamOverviewRow }">
                <div class="team-vac-cell">
                  <div class="team-vac-main">
                    <div
                      v-tooltip.bottom="{ value: teamVacationBarTitle(data), class: 'team-overview-kpi-tooltip' }"
                      class="team-vac-bar"
                      role="img"
                      :aria-label="`Urlaub: ${teamVacationBar(data).total.toFixed(1)} gesamt`"
                    >
                    <div
                      v-if="teamVacationBar(data).pctTaken > 0"
                      class="team-vac-seg team-vac-seg--taken"
                      :style="{ width: teamVacationBar(data).pctTaken + '%' }"
                      >
                        <span v-if="teamVacationBar(data).pctTaken >= 18" class="team-vac-label">{{
                          teamVacationBar(data).taken.toFixed(1)
                        }}</span>
                      </div>
                    <div
                      v-if="teamVacationBar(data).pctPlanned > 0"
                      class="team-vac-seg team-vac-seg--planned"
                      :style="{ width: teamVacationBar(data).pctPlanned + '%' }"
                      >
                        <span v-if="teamVacationBar(data).pctPlanned >= 18" class="team-vac-label">{{
                          teamVacationBar(data).planned.toFixed(1)
                        }}</span>
                      </div>
                    <div
                      v-if="teamVacationBar(data).pctOpen > 0"
                      class="team-vac-seg team-vac-seg--open"
                      :style="{ width: teamVacationBar(data).pctOpen + '%' }"
                      >
                        <span v-if="teamVacationBar(data).pctOpen >= 18" class="team-vac-label">{{
                          teamVacationBar(data).open.toFixed(1)
                        }}</span>
                      </div>
                  </div>
                  </div>
                  <span
                    v-tooltip.bottom="{ value: restGesamtTitle(data), class: 'team-overview-kpi-tooltip' }"
                    class="rest-gesamt-cell"
                    >{{ data.vacation_remaining_total.toFixed(1) }}</span
                  >
                </div>
              </template>
            </Column>
          </DataTable>
        </template>
      </Card>

      <Card v-if="isLeitung && employees.length" class="team">
        <template #title>Team (Übersicht)</template>
        <template #content>
          <p class="sub">
            Live-Anwesenheit aus Stempeldaten ist noch nicht angebunden — Stammdaten der Mitarbeitenden:
          </p>
          <div class="team-employee-toolbar">
            <label class="field-label" for="dash-team-group-stamm">Gruppe</label>
            <Select
              id="dash-team-group-stamm"
              v-model="teamGroupFilter"
              :options="teamGroupFilterOptions"
              option-label="label"
              option-value="value"
              class="group-filter-select"
            />
          </div>
          <DataTable
            :value="teamEmployeesTableRows"
            size="small"
            data-key="id"
            :paginator="teamEmployeesTableRows.length > 8"
            :rows="8"
            sort-field="display_name"
            :sort-order="1"
            row-hover
            class="employee-nav-table"
            @row-click="onTeamEmployeeRowClick"
          >
            <template #empty>
              <span class="muted">Keine Einträge für diese Auswahl.</span>
            </template>
            <Column field="display_name" header="Name" sortable />
            <Column field="role" header="Rolle" sortable />
            <Column field="active" header="Aktiv" sortable>
              <template #body="{ data }: { data: Employee }">
                {{ data.active ? 'Ja' : 'Nein' }}
              </template>
            </Column>
          </DataTable>
        </template>
      </Card>
    </template>
  </div>
</template>

<style scoped>
.dash {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.w {
  width: 100%;
}
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  margin-top: 0.5rem;
}
.welcome {
  margin: 0;
  font-size: 1.1rem;
  color: #334155;
}
.cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 1rem;
}
.stat {
  margin: 0 0 0.35rem;
  font-size: 0.95rem;
}
.sub {
  margin: 0;
  font-size: 0.85rem;
  color: #64748b;
}
.pos {
  color: #15803d;
}
.neg {
  color: #b91c1c;
}
.team {
  max-width: 720px;
}
.team-overview-card {
  max-width: 960px;
}
.team-overview-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 0.75rem 1rem;
  margin-bottom: 0.75rem;
}
.team-overview-hours-range {
  margin: 0 0 0.5rem;
  max-width: 52rem;
}
.name-filter {
  flex: 1;
  min-width: 160px;
  max-width: 280px;
}
.toolbar-field {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  min-width: 0;
}
.toolbar-field--group {
  width: 100%;
  max-width: 220px;
}
.field-label {
  margin: 0;
  font-size: 0.8rem;
  color: #64748b;
}
.group-filter-select {
  width: 100%;
}
.team-employee-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 0.5rem 1rem;
  margin-bottom: 0.75rem;
}
.team-employee-toolbar .field-label {
  width: 100%;
}
.team-employee-toolbar .group-filter-select {
  max-width: 280px;
}
.team-overview-err {
  margin: 0 0 0.75rem;
  font-size: 0.85rem;
  color: #94a3b8;
}
.team-overview-warn {
  margin: 0 0 0.5rem;
}
.team-overview-warn {
  color: #b45309;
}
/* Hit-Bereich = nur die Zahl (inline-block schrumpft auf Inhalt, kein volle Zelle). */
.rest-gesamt-cell {
  display: inline-block;
  cursor: help;
  text-decoration: underline dotted #94a3b8;
  text-underline-offset: 0.15em;
  text-decoration-thickness: 1px;
}

.team-vac-cell {
  display: flex;
  align-items: center;
  gap: 0.6rem;
}
.team-vac-main {
  flex: 1;
  min-width: 8rem;
}
.team-vac-bar {
  height: 0.85rem;
  border-radius: 6px;
  overflow: hidden;
  background: #e2e8f0;
  display: flex;
}
.team-vac-seg {
  min-width: 0;
  height: 100%;
  position: relative;
}
.team-vac-label {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  font-size: 0.7rem;
  font-weight: 800;
  color: #ffffff;
  text-shadow:
    0 1px 2px rgba(0, 0, 0, 0.45),
    0 0 1px rgba(0, 0, 0, 0.35);
  pointer-events: none;
}
.team-vac-seg--taken {
  background: #3b82f6;
}
.team-vac-seg--planned {
  background: #f59e0b;
}
.team-vac-seg--open {
  background: #22c55e;
}

/* PrimeVue Tooltip: mehrzeilig in scrollierbarer Tabelle ( natives title ist hier oft unzuverlässig ) */
:deep(.p-tooltip.team-overview-kpi-tooltip .p-tooltip-text) {
  max-width: min(32rem, 92vw);
  white-space: normal;
  text-align: left;
  line-height: 1.35;
}

.employee-nav-table :deep(.p-datatable-tbody > tr) {
  cursor: pointer;
}
.err {
  color: #b91c1c;
  margin: 0;
}
.muted {
  color: #64748b;
}
</style>
