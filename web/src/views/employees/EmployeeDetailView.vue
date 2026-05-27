<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import DatePicker from 'primevue/datepicker'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import Tag from 'primevue/tag'
import { useToast } from 'primevue/usetoast'

import {
  createEmployeeAbsence,
  deleteEmployeeAbsence,
  fetchClosureDays,
  fetchEmployeeAbsences,
  fetchEmployeeBalance,
  fetchEmployeeCorrections,
  fetchEmployeeSchedule,
  fetchEmployeeTimes,
  fetchEmployeeVacation,
  fetchFixedNonWorkWeekdays,
  fetchWeeklyHours,
} from '@/api/management'
import { fetchEmployees } from '@/api/employees'
import BalanceCard from '@/components/BalanceCard.vue'
import VacationBalanceBar from '@/components/VacationBalanceBar.vue'
import TimeTable from '@/components/TimeTable.vue'
import type {
  Absence,
  ClosureDay,
  Employee,
  FixedNonWorkWeekdays,
  HolidayCredit,
  MonthBalance,
  Schedule,
  TimeCorrection,
  VacationBalance,
  WeeklyHours,
  WorkPeriod,
} from '@/types/api'
import {
  endOfISOWeek,
  formatGermanDate,
  formatGermanDateTime,
  startOfISOWeek,
  toISODateLocal,
} from '@/utils/dates'
import { canManageEmployeeByRole } from '@/utils/roles'
import { enumerateWorkdayISO, vacationDisplayGapOnlySkippable } from '@/utils/workdays'
import { useAuthStore } from '@/stores/auth'

const toast = useToast()

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const employeeId = computed(() => Number(route.params.id))

const canManageEmployee = computed(() =>
  employee.value ? canManageEmployeeByRole(auth.role, employee.value.role) : false,
)

const employee = ref<Employee | null>(null)
const weeklyHoursList = ref<WeeklyHours[]>([])
const fnwList = ref<FixedNonWorkWeekdays[]>([])
const tab = ref<'times' | 'balance' | 'absences' | 'corrections' | 'settings'>('times')

const from = ref<Date>(startOfISOWeek(new Date()))
const to = ref<Date>(endOfISOWeek(new Date()))
const periods = ref<WorkPeriod[]>([])
const absences = ref<Absence[]>([])
const corrections = ref<TimeCorrection[]>([])
const schedules = ref<Schedule[]>([])
const holidays = ref<HolidayCredit[]>([])
const closures = ref<ClosureDay[]>([])
const timesLoading = ref(false)

function normalizeISODate(s: string): string {
  const m = String(s).match(/^(\d{4})-(\d{2})-(\d{2})/)
  return m ? `${m[1]}-${m[2]}-${m[3]}` : String(s)
}

const balMonth = ref(new Date().getMonth() + 1)
const balYear = ref(new Date().getFullYear())
const balance = ref<MonthBalance | null>(null)
const balLoading = ref(false)

const employeeVacation = ref<VacationBalance | null>(null)
const vacLoading = ref(false)

const tabs: { id: typeof tab.value; label: string }[] = [
  { id: 'times', label: 'Zeiten' },
  { id: 'balance', label: 'Saldo' },
  { id: 'absences', label: 'Abwesenheiten' },
  { id: 'corrections', label: 'Korrekturen' },
  { id: 'settings', label: 'Einstellungen' },
]

async function resolveEmployee() {
  const list = await fetchEmployees()
  employee.value = list.find((e) => e.id === employeeId.value) ?? null
  if (!employee.value) router.replace('/employees')
}

async function loadTimesBlock() {
  if (!employee.value) return
  timesLoading.value = true
  try {
    const f = toISODateLocal(from.value)
    const t = toISODateLocal(to.value)
    const [tRes, aRes, cRes, schRes, cls, wh, fnw] = await Promise.all([
      fetchEmployeeTimes(employee.value.id, f, t),
      fetchEmployeeAbsences(employee.value.id, f, t),
      fetchEmployeeCorrections(employee.value.id, f, t),
      fetchEmployeeSchedule(employee.value.id, f, t),
      fetchClosureDays().catch((): ClosureDay[] => []),
      fetchWeeklyHours(employee.value.id).catch((): WeeklyHours[] => []),
      fetchFixedNonWorkWeekdays(employee.value.id).catch((): FixedNonWorkWeekdays[] => []),
    ])
    periods.value = tRes.work_periods
    holidays.value = tRes.holidays ?? []
    absences.value = aRes
    corrections.value = cRes
    schedules.value = schRes.schedules
    closures.value = cls
    weeklyHoursList.value = wh
    fnwList.value = fnw
  } finally {
    timesLoading.value = false
  }
}

async function loadBalance() {
  if (!employee.value) return
  balLoading.value = true
  try {
    balance.value = await fetchEmployeeBalance(employee.value.id, balMonth.value, balYear.value)
  } catch {
    balance.value = null
  } finally {
    balLoading.value = false
  }
}

async function loadEmployeeVacation() {
  if (!employee.value) return
  vacLoading.value = true
  try {
    employeeVacation.value = await fetchEmployeeVacation(employee.value.id)
  } catch {
    employeeVacation.value = null
  } finally {
    vacLoading.value = false
  }
}

onMounted(async () => {
  await resolveEmployee()
  await loadTimesBlock()
  await loadBalance()
  await loadEmployeeVacation()
})

watch(employeeId, async () => {
  await resolveEmployee()
  await loadTimesBlock()
  await loadBalance()
  await loadEmployeeVacation()
})

watch([from, to], () => {
  void loadTimesBlock()
})

watch([balMonth, balYear, employeeId], () => {
  void loadBalance()
})

const scheduleByDate = computed(() => {
  const m: Record<string, { shift_start: string; shift_end: string }> = {}
  for (const s of schedules.value) {
    const key = s.schedule_date.slice(0, 10)
    m[key] = { shift_start: s.shift_start, shift_end: s.shift_end }
  }
  return m
})

function absenceTypeLabel(t: string) {
  if (t === 'sick') return 'Krank'
  if (t === 'vacation') return 'Urlaub'
  return 'Sonstiges'
}

const holidayDateSet = computed(() => new Set(holidays.value.map((h) => normalizeISODate(h.holiday_date))))
const closureDateSet = computed(() => new Set(closures.value.map((c) => normalizeISODate(c.closure_date))))
const skipDatesSet = computed(() => {
  const s = new Set<string>()
  for (const x of holidayDateSet.value) s.add(x)
  for (const x of closureDateSet.value) s.add(x)
  return s
})

interface AbsenceViewRow {
  kind: 'range' | 'single'
  rowKey: string
  from: string
  to?: string
  absence_type: string
  half_day: boolean
  /** Nur bei Urlaub: tatsächlich gebuchte Urlaubstage (ohne WE/Feiertag/Schließtag). */
  vacation_days?: number
  ids: number[]
}

const absenceViewRows = computed<AbsenceViewRow[]>(() => {
  const list = [...absences.value]
    .map((a) => ({ ...a, absence_date: normalizeISODate(a.absence_date) }))
    .sort((a, b) => a.absence_date.localeCompare(b.absence_date) || (a.id ?? 0) - (b.id ?? 0))

  const fixedSet = fnwList.value
  const skip = skipDatesSet.value

  const singles: AbsenceViewRow[] = []
  const vac = list.filter((a) => a.absence_type === 'vacation' && !a.half_day)
  const other = list.filter((a) => !(a.absence_type === 'vacation' && !a.half_day))

  for (const a of other) {
    singles.push({
      kind: 'single',
      rowKey: `a-${a.id}`,
      from: a.absence_date,
      absence_type: a.absence_type,
      half_day: a.half_day,
      vacation_days: a.absence_type === 'vacation' ? (a.half_day ? 0.5 : 1) : undefined,
      ids: [a.id],
    })
  }

  const ranges: AbsenceViewRow[] = []
  let curFrom = ''
  let curTo = ''
  let curIds: number[] = []
  let curLen = 0

  function flushRange() {
    if (!curLen) return
    ranges.push({
      kind: curLen > 1 ? 'range' : 'single',
      rowKey: curLen > 1 ? `vr-${curFrom}-${curTo}` : `a-${curIds[0]}`,
      from: curFrom,
      to: curLen > 1 ? curTo : undefined,
      absence_type: 'vacation',
      half_day: false,
      vacation_days: curIds.length,
      ids: [...curIds],
    })
    curFrom = ''
    curTo = ''
    curIds = []
    curLen = 0
  }

  for (const a of vac) {
    const iso = a.absence_date
    if (!curLen) {
      curFrom = iso
      curTo = iso
      curIds = [a.id]
      curLen = 1
      continue
    }
    if (vacationDisplayGapOnlySkippable(curTo, iso, skip, fixedSet)) {
      curTo = iso
      curIds.push(a.id)
      curLen++
      continue
    }
    flushRange()
    curFrom = iso
    curTo = iso
    curIds = [a.id]
    curLen = 1
  }
  flushRange()

  return [...ranges, ...singles].sort((a, b) => a.from.localeCompare(b.from))
})

const showVacationEdit = ref(false)
const editFrom = ref<Date>(new Date())
const editTo = ref<Date>(new Date())
const editIds = ref<number[]>([])
const editSaving = ref(false)

watch(editFrom, (v) => {
  const a = new Date(v.getFullYear(), v.getMonth(), v.getDate())
  const b = new Date(editTo.value.getFullYear(), editTo.value.getMonth(), editTo.value.getDate())
  if (b.getTime() < a.getTime()) editTo.value = new Date(a)
})

function openVacationEdit(row: AbsenceViewRow) {
  if (row.absence_type !== 'vacation' || row.half_day) return
  editFrom.value = new Date(`${row.from}T00:00:00`)
  editTo.value = new Date(`${(row.to ?? row.from)}T00:00:00`)
  editIds.value = [...row.ids]
  showVacationEdit.value = true
}

async function removeVacationRow(row: AbsenceViewRow) {
  if (!employee.value) return
  if (row.absence_type !== 'vacation' || row.half_day) return
  const label =
    row.to && row.to !== row.from
      ? `Urlaub ${formatGermanDate(row.from)} – ${formatGermanDate(row.to)}`
      : `Urlaub ${formatGermanDate(row.from)}`
  if (!confirm(`${label} löschen?`)) return
  try {
    for (const id of row.ids) await deleteEmployeeAbsence(employee.value.id, id)
    toast.add({ severity: 'success', summary: 'Gelöscht', life: 10000 })
    await loadTimesBlock()
    await loadEmployeeVacation()
  } catch {
    toast.add({ severity: 'error', summary: 'Löschen fehlgeschlagen', life: 10000 })
  }
}

function enumerateVacationWorkdayISO(fromD: Date, toD: Date): string[] {
  return enumerateWorkdayISO(fromD, toD, holidayDateSet.value, closureDateSet.value, fnwList.value)
}

async function submitVacationEdit() {
  if (!employee.value) return
  editSaving.value = true
  try {
    // alte Tage löschen
    for (const id of editIds.value) {
      try {
        await deleteEmployeeAbsence(employee.value.id, id)
      } catch {
        /* ignore */
      }
    }

    const a = new Date(editFrom.value.getFullYear(), editFrom.value.getMonth(), editFrom.value.getDate())
    const b = new Date(editTo.value.getFullYear(), editTo.value.getMonth(), editTo.value.getDate())
    const days = enumerateVacationWorkdayISO(a, b)
    let ok = 0
    let skipped = 0
    for (const iso of days) {
      try {
        await createEmployeeAbsence(employee.value.id, {
          absence_date: iso,
          absence_type: 'vacation',
          half_day: false,
        })
        ok++
      } catch {
        skipped++
      }
    }
    toast.add({
      severity: ok ? 'success' : 'warn',
      summary: ok ? (ok === 1 ? 'Gespeichert' : `${ok} Urlaubstage gespeichert`) : 'Nichts gespeichert',
      ...(skipped ? { detail: `${skipped} übersprungen.` } : {}),
      life: 10000,
    })
    showVacationEdit.value = false
    await loadTimesBlock()
    await loadEmployeeVacation()
  } finally {
    editSaving.value = false
  }
}
</script>

<template>
  <div v-if="employee" class="page">
    <p v-if="!canManageEmployee" class="banner">
      Dieses Konto kann nur von einem Superadmin bearbeitet werden (Stammdaten, Zeiten, Dienstplan-Zuweisungen).
    </p>
    <div class="head">
      <div>
        <h2 class="title">{{ employee.display_name }}</h2>
        <p class="sub">{{ employee.username }} · {{ employee.role }}</p>
      </div>
      <RouterLink v-if="canManageEmployee" :to="`/employees/${employee.id}/edit`">
        <Button label="Bearbeiten" icon="pi pi-pencil" outlined />
      </RouterLink>
    </div>

    <Card class="employee-vacation-card">
      <template #title>Urlaub {{ employeeVacation?.year ?? new Date().getFullYear() }}</template>
      <template #content>
        <div v-if="vacLoading" class="muted">Laden…</div>
        <VacationBalanceBar v-else :balance="employeeVacation" />
      </template>
    </Card>

    <div class="tabs">
      <button
        v-for="t in tabs"
        :key="t.id"
        type="button"
        class="tab"
        :class="{ on: tab === t.id }"
        :data-testid="`employee-tab-${t.id}`"
        @click="tab = t.id"
      >
        {{ t.label }}
      </button>
    </div>

    <div v-show="tab === 'times'" class="panel">
      <div class="toolbar">
        <label class="lbl">Von</label>
        <DatePicker v-model="from" date-format="dd.mm.yy" show-icon />
        <label class="lbl">Bis</label>
        <DatePicker v-model="to" date-format="dd.mm.yy" show-icon />
      </div>
      <TimeTable
        :periods="periods"
        :absences="absences"
        :corrections="corrections"
        :holidays="holidays"
        :schedule-by-date="scheduleByDate"
        :weekly-hours="weeklyHoursList"
        :fixed-non-work-weekdays-history="fnwList"
        :loading="timesLoading"
        :row-correction="
          canManageEmployee ? { mode: 'employee', employeeId: employee.id } : undefined
        "
        @data-changed="loadTimesBlock"
      />
    </div>

    <div v-show="tab === 'balance'" class="panel">
      <div class="toolbar">
        <label class="lbl">Monat</label>
        <InputNumber
          v-model="balMonth"
          :min="1"
          :max="12"
          :use-grouping="false"
          show-buttons
          data-testid="employee-bal-month"
        />
        <label class="lbl">Jahr</label>
        <InputNumber
          v-model="balYear"
          :min="2000"
          :max="2100"
          :use-grouping="false"
          show-buttons
          data-testid="employee-bal-year"
        />
      </div>
      <div v-if="balLoading" class="muted">Laden…</div>
      <BalanceCard v-else-if="balance" :balance="balance" />
      <p v-else class="muted">Keine Saldo-Daten.</p>
    </div>

    <div v-show="tab === 'absences'" class="panel">
      <p class="hint">
        Abwesenheiten im gewählten Zeitraum (Tab „Zeiten“). Zum Eintragen:
        <RouterLink to="/absences">Abwesenheiten</RouterLink>
      </p>
      <DataTable :value="absenceViewRows" data-key="rowKey" striped-rows>
        <Column field="from" header="Datum" sortable>
          <template #body="{ data }: { data: AbsenceViewRow }">
            <span v-if="data.to">{{ formatGermanDate(data.from) }} – {{ formatGermanDate(data.to) }}</span>
            <span v-else>{{ formatGermanDate(data.from) }}</span>
          </template>
        </Column>
        <Column header="Art">
          <template #body="{ data }: { data: AbsenceViewRow }">{{ absenceTypeLabel(data.absence_type) }}</template>
        </Column>
        <Column field="vacation_days" header="Urlaubstage" sortable style="min-width: 6.5rem; width: 6.5rem">
          <template #body="{ data }: { data: AbsenceViewRow }">
            <span v-if="data.absence_type === 'vacation' && data.vacation_days != null">
              {{ String(data.vacation_days).replace('.', ',') }}
            </span>
            <span v-else class="muted">—</span>
          </template>
        </Column>
        <Column header="Halbtag">
          <template #body="{ data }: { data: AbsenceViewRow }">
            <Tag :severity="data.half_day ? 'warn' : 'secondary'" :value="data.half_day ? 'Ja' : 'Nein'" />
          </template>
        </Column>
        <Column header="" style="width: 6.5rem">
          <template #body="{ data }: { data: AbsenceViewRow }">
            <template v-if="data.absence_type === 'vacation' && !data.half_day">
              <Button icon="pi pi-pencil" text rounded aria-label="Bearbeiten" @click="openVacationEdit(data)" />
              <Button
                icon="pi pi-trash"
                severity="danger"
                text
                rounded
                aria-label="Löschen"
                @click="removeVacationRow(data)"
              />
            </template>
          </template>
        </Column>
      </DataTable>

      <Dialog v-model:visible="showVacationEdit" header="Urlaubszeitraum" modal :style="{ width: '420px' }">
        <div class="form">
          <label>Von</label>
          <DatePicker v-model="editFrom" date-format="dd.mm.yy" show-icon class="w" />
          <label>Bis</label>
          <DatePicker v-model="editTo" date-format="dd.mm.yy" show-icon class="w" :min-date="editFrom" />
          <p class="hint">
            Wochenenden, Feiertage, Schließtage und fest freie Wochentage im Zeitraum werden beim Urlaub automatisch
            übersprungen.
          </p>
        </div>
        <template #footer>
          <Button label="Abbrechen" severity="secondary" text @click="showVacationEdit = false" />
          <Button label="Speichern" :loading="editSaving" @click="submitVacationEdit" />
        </template>
      </Dialog>
    </div>

    <div v-show="tab === 'corrections'" class="panel">
      <p class="hint">
        Korrekturen vornehmen im
        <RouterLink to="/corrections">Korrektur-Tool</RouterLink>
        .
      </p>
      <DataTable :value="corrections" data-key="id" striped-rows>
        <Column field="created_at" header="Datum" sortable>
          <template #body="{ data }">{{ formatGermanDateTime(data.created_at) }}</template>
        </Column>
        <Column header="Korrektur von → bis">
          <template #body="{ data }">
            {{ formatGermanDateTime(data.corrected_in) }} → {{ formatGermanDateTime(data.corrected_out) }}
          </template>
        </Column>
        <Column field="reason" header="Grund" />
      </DataTable>
    </div>

    <div v-show="tab === 'settings'" class="panel">
      <Card>
        <template #title>Stammdaten</template>
        <template #content>
          <dl class="dl">
            <dt>Anzeigename</dt>
            <dd>{{ employee.display_name }}</dd>
            <dt>Benutzername</dt>
            <dd>{{ employee.username }}</dd>
            <dt>Rolle</dt>
            <dd>{{ employee.role }}</dd>
            <dt>Aktiv</dt>
            <dd>
              <Tag :severity="employee.active ? 'success' : 'secondary'" :value="employee.active ? 'Ja' : 'Nein'" />
            </dd>
            <dt>Passwortwechsel nötig</dt>
            <dd>{{ employee.must_change_password ? 'Ja' : 'Nein' }}</dd>
          </dl>
          <RouterLink v-if="canManageEmployee" :to="`/employees/${employee.id}/edit`">
            <Button label="Stammdaten bearbeiten" icon="pi pi-pencil" class="mt" />
          </RouterLink>
        </template>
      </Card>
      <Card>
        <template #title>Startsaldo (Import / Alt-System)</template>
        <template #content>
          <dl class="dl">
            <dt>Stundensaldo (h)</dt>
            <dd>
              {{ (employee.opening_hours_balance ?? 0).toFixed(2) }} h
            </dd>
            <dt>Urlaub (Tage)</dt>
            <dd>
              {{ (employee.opening_vacation_days ?? 0).toFixed(1) }} Tg.
            </dd>
          </dl>
        </template>
      </Card>
    </div>
  </div>
</template>

<style scoped>
.page {
  max-width: 1000px;
}
.employee-vacation-card {
  max-width: 520px;
  margin-bottom: 1rem;
}
.banner {
  margin: 0 0 1rem;
  padding: 0.65rem 0.85rem;
  font-size: 0.9rem;
  color: #92400e;
  background: #fffbeb;
  border: 1px solid #fde68a;
  border-radius: 8px;
}
.head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 1rem;
  margin-bottom: 1rem;
}
.title {
  margin: 0;
  font-size: 1.35rem;
}
.sub {
  margin: 0.25rem 0 0;
  color: #64748b;
  font-size: 0.9rem;
}
.tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem;
  margin-bottom: 1rem;
}
.tab {
  border: 1px solid #e2e8f0;
  background: #fff;
  padding: 0.4rem 0.85rem;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.9rem;
}
.tab.on {
  background: #0f172a;
  color: #fff;
  border-color: #0f172a;
}
.panel {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
}
.lbl {
  font-size: 0.85rem;
  color: #64748b;
}
.muted {
  color: #64748b;
}
.hint {
  margin: 0;
  font-size: 0.9rem;
  color: #64748b;
}
.hint a {
  color: #2563eb;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.form label {
  font-size: 0.85rem;
  color: #64748b;
}
.w {
  width: 100%;
}
.dl {
  display: grid;
  grid-template-columns: 180px 1fr;
  gap: 0.5rem 1rem;
  margin: 0;
}
.dl dt {
  color: #64748b;
  font-size: 0.85rem;
}
.dl dd {
  margin: 0;
}
.mt {
  margin-top: 1rem;
}
.fixed-free-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem 1.25rem;
  margin: 0.5rem 0 0.75rem;
}
.fixed-free-item {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.9rem;
  cursor: pointer;
}
</style>
