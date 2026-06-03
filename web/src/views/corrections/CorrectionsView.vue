<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import DatePicker from 'primevue/datepicker'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import { useToast } from 'primevue/usetoast'

import {
  createCorrection,
  createManualWorkPeriod,
  deleteManualWorkPeriod,
  fetchEmployeeCorrections,
  fetchEmployeeTimes,
} from '@/api/management'
import { fetchEmployees } from '@/api/employees'
import type { Employee, TimeCorrection, WorkPeriod } from '@/types/api'
import { getApiErrorMessage } from '@/utils/apiError'
import { focusSelectFilterOnShow } from '@/utils/selectFilterFocus'
import {
  buildCorrectionTimeInstants,
  endOfISOWeek,
  formatGermanDate,
  formatGermanTime,
  isoToTimeInputValue,
  parseTimeHHMM,
  plusOneHourHHMM,
  startOfISOWeek,
  toISODateLocal,
} from '@/utils/dates'
import { clearRouteQueryKeys, queryISODate, queryPositiveInt, queryString } from '@/utils/leitungDeepLink'

const toast = useToast()
const route = useRoute()
const router = useRouter()

const employees = ref<Employee[]>([])
const employeeId = ref<number | null>(null)
const from = ref<Date>(startOfISOWeek(new Date()))
const to = ref<Date>(endOfISOWeek(new Date()))

const periods = ref<WorkPeriod[]>([])
const corrections = ref<TimeCorrection[]>([])
const loading = ref(false)

const corrByWp = computed(() => {
  const m = new Map<number, TimeCorrection>()
  for (const c of corrections.value) m.set(c.work_period_id, c)
  return m
})

interface Row {
  rowKey: string
  period: WorkPeriod
  correction?: TimeCorrection
}

const tableRows = computed<Row[]>(() =>
  periods.value
    .filter((p) => !p.is_break)
    .map((p) => ({ rowKey: String(p.id), period: p, correction: corrByWp.value.get(p.id) }))
    .sort((a, b) => {
      const da = a.period.work_date.localeCompare(b.period.work_date)
      if (da !== 0) return da
      return a.period.punch_in.localeCompare(b.period.punch_in)
    }),
)

async function load() {
  if (employeeId.value == null) {
    periods.value = []
    corrections.value = []
    return
  }
  loading.value = true
  try {
    const f = toISODateLocal(from.value)
    const t = toISODateLocal(to.value)
    const [tRes, cRes] = await Promise.all([
      fetchEmployeeTimes(employeeId.value, f, t),
      fetchEmployeeCorrections(employeeId.value, f, t),
    ])
    periods.value = tRes.work_periods
    corrections.value = cRes
  } catch {
    toast.add({ severity: 'error', summary: 'Laden fehlgeschlagen', life: 10000 })
  } finally {
    loading.value = false
  }
}

function tryOpenManualFromQuery() {
  if (route.query.open !== 'manual') return
  const emp = queryPositiveInt(route.query.employeeId)
  if (emp != null && employees.value.some((e) => e.id === emp)) {
    employeeId.value = emp
  }
  const d = queryISODate(route.query.date)
  if (d) {
    manDate.value = d
    manualDateMax.value = endOfLocalToday()
  }
  const shiftStart = queryString(route.query.shiftStart)
  const shiftEnd = queryString(route.query.shiftEnd)
  if (shiftStart) {
    manIn.value = shiftStart
    manOutManuallyEdited.value = Boolean(shiftEnd)
    manOut.value = shiftEnd || plusOneHourHHMM(shiftStart) || manOut.value
  }
  showManual.value = true
  clearRouteQueryKeys(router, ['open', 'employeeId', 'date', 'shiftStart', 'shiftEnd'])
}

onMounted(async () => {
  employees.value = await fetchEmployees()
  employeeId.value = employees.value[0]?.id ?? null
  await load()
  tryOpenManualFromQuery()
})

watch([employeeId, from, to], load)

const showCorrect = ref(false)
const editWp = ref<WorkPeriod | null>(null)
const corrIn = ref('')
const corrOut = ref('')
const corrOutManuallyEdited = ref(false)
const corrReason = ref('')
const saving = ref(false)

function openCorrect(p: WorkPeriod) {
  editWp.value = p
  const c = corrByWp.value.get(p.id)
  corrIn.value = isoToTimeInputValue(c ? c.corrected_in : p.punch_in)
  corrOutManuallyEdited.value = false
  const outSrc = c ? c.corrected_out : p.punch_out
  corrOut.value = outSrc ? isoToTimeInputValue(outSrc) : (plusOneHourHHMM(corrIn.value) ?? '09:00')
  corrReason.value = ''
  showCorrect.value = true
}

watch(corrIn, (next) => {
  if (!showCorrect.value || corrOutManuallyEdited.value) return
  const auto = plusOneHourHHMM(next)
  if (auto) corrOut.value = auto
})

async function submitCorrect() {
  if (!editWp.value || employeeId.value == null) return
  if (!corrReason.value.trim()) {
    toast.add({ severity: 'warn', summary: 'Grund erforderlich', life: 10000 })
    return
  }
  const a = parseTimeHHMM(corrIn.value)
  const b = parseTimeHHMM(corrOut.value)
  if (!a || !b) {
    toast.add({ severity: 'warn', summary: 'Bitte gültige Uhrzeiten (HH:MM) eingeben', life: 10000 })
    return
  }
  if (a.h === b.h && a.m === b.m) {
    toast.add({ severity: 'warn', summary: 'Kommen und Gehen dürfen nicht dieselbe Uhrzeit haben.', life: 10000 })
    return
  }
  const inst = buildCorrectionTimeInstants(editWp.value.work_date, corrIn.value, corrOut.value)
  if (!inst) {
    toast.add({
      severity: 'warn',
      summary: 'Ungültige Uhrzeiten',
      detail: 'Am selben Kalendertag muss Gehen zeitlich nach Kommen liegen (z. B. Kommen 08:00, Gehen 16:00).',
      life: 10000,
    })
    return
  }
  saving.value = true
  try {
    await createCorrection(employeeId.value, {
      work_period_id: editWp.value.id,
      corrected_in: inst.corrected_in,
      corrected_out: inst.corrected_out,
      reason: corrReason.value.trim(),
    })
    toast.add({ severity: 'success', summary: 'Korrektur gespeichert', life: 10000 })
    showCorrect.value = false
    await load()
  } catch (e) {
    const detail = getApiErrorMessage(e)
    toast.add({
      severity: 'error',
      summary: 'Korrektur fehlgeschlagen',
      ...(detail ? { detail, life: 10000 } : { life: 10000 }),
    })
  } finally {
    saving.value = false
  }
}

const showManual = ref(false)
const manDate = ref<Date>(new Date())
const manIn = ref('08:00')
const manOut = ref('16:00')
const manOutManuallyEdited = ref(false)

function endOfLocalToday(): Date {
  const d = new Date()
  return new Date(d.getFullYear(), d.getMonth(), d.getDate(), 23, 59, 59, 999)
}

const manualDateMax = ref(endOfLocalToday())

function openManual() {
  manDate.value = new Date()
  manualDateMax.value = endOfLocalToday()
  manIn.value = '08:00'
  manOutManuallyEdited.value = false
  manOut.value = plusOneHourHHMM(manIn.value) ?? '09:00'
  showManual.value = true
}

watch(manIn, (next) => {
  if (!showManual.value || manOutManuallyEdited.value) return
  const auto = plusOneHourHHMM(next)
  if (auto) manOut.value = auto
})

function onCorrOutInput() {
  corrOutManuallyEdited.value = true
}

function onManOutInput() {
  manOutManuallyEdited.value = true
}

function timeOnDateToISO(d: Date, hhmm: string): string {
  const p = parseTimeHHMM(hhmm)
  if (!p) return new Date(NaN).toISOString()
  const local = new Date(d.getFullYear(), d.getMonth(), d.getDate(), p.h, p.m, 0, 0)
  return local.toISOString()
}

async function submitManual() {
  if (employeeId.value == null) return
  saving.value = true
  try {
    const workYmd = toISODateLocal(manDate.value)
    const todayYmd = toISODateLocal(new Date())
    if (workYmd > todayYmd) {
      toast.add({
        severity: 'warn',
        summary: 'Zukünftige Tage sind nicht erlaubt',
        detail: 'Manuelle Zeiten nur für heute oder die Vergangenheit. Uhrzeiten am heutigen Tag dürfen in der Zukunft liegen.',
        life: 10000,
      })
      return
    }
    const punchInISO = timeOnDateToISO(manDate.value, manIn.value)
    const punchOutISO = timeOnDateToISO(manDate.value, manOut.value)
    if (Number.isNaN(new Date(punchInISO).getTime()) || Number.isNaN(new Date(punchOutISO).getTime())) {
      toast.add({ severity: 'warn', summary: 'Bitte gültige Uhrzeiten eingeben', life: 10000 })
      return
    }
    if (new Date(punchOutISO).getTime() <= new Date(punchInISO).getTime()) {
      toast.add({ severity: 'warn', summary: 'Gehen muss nach Kommen liegen', life: 10000 })
      return
    }
    await createManualWorkPeriod(employeeId.value, {
      work_date: toISODateLocal(manDate.value),
      punch_in: punchInISO,
      punch_out: punchOutISO,
    })
    toast.add({ severity: 'success', summary: 'Manuelle Zeit gespeichert', life: 10000 })
    showManual.value = false
    await load()
  } catch (e) {
    const detail = getApiErrorMessage(e)
    toast.add({
      severity: 'error',
      summary: 'Speichern fehlgeschlagen',
      ...(detail ? { detail, life: 10000 } : { life: 10000 }),
    })
  } finally {
    saving.value = false
  }
}

/** Leerer source = regulärer Stempel/Import (kein gespeicherter Vermerk). */
function periodSourceLabel(source: string): string {
  const s = (source ?? '').trim()
  if (s === 'manual') return 'manuell'
  return ''
}

async function removeManual(p: WorkPeriod) {
  if (p.source !== 'manual') return
  if (!confirm('Manuellen Eintrag löschen?')) return
  if (employeeId.value == null) return
  try {
    await deleteManualWorkPeriod(employeeId.value, p.id)
    toast.add({ severity: 'success', summary: 'Gelöscht', life: 10000 })
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Löschen fehlgeschlagen', life: 10000 })
  }
}
</script>

<template>
  <div class="page">
    <Card>
      <template #title>Korrekturen & manuelle Zeiten</template>
      <template #content>
        <div class="toolbar">
          <Select
            v-model="employeeId"
            :options="employees.map((e) => ({ label: e.display_name, value: e.id }))"
            option-label="label"
            option-value="value"
            placeholder="Mitarbeiter"
            class="emp"
            filter
            data-testid="corrections-employee-select"
            @show="focusSelectFilterOnShow"
          />
          <div class="range">
            <label class="lbl">Von</label>
            <DatePicker v-model="from" date-format="dd.mm.yy" show-icon />
            <label class="lbl">Bis</label>
            <DatePicker v-model="to" date-format="dd.mm.yy" show-icon />
          </div>
          <Button label="Manuelle Zeit" icon="pi pi-plus" data-testid="corrections-manual-btn" @click="openManual" />
        </div>

        <DataTable
          :value="tableRows"
          :loading="loading"
          data-key="rowKey"
          striped-rows
          data-testid="corrections-table"
        >
          <Column header="Datum">
            <template #body="{ data }">{{ formatGermanDate(data.period.work_date) }}</template>
          </Column>
          <Column header="Original">
            <template #body="{ data }">
              {{ formatGermanTime(data.period.punch_in) }}
              →
              {{ data.period.punch_out ? formatGermanTime(data.period.punch_out) : '—' }}
            </template>
          </Column>
          <Column header="Korrektur">
            <template #body="{ data }">
              <template v-if="data.correction">
                {{ formatGermanTime(data.correction.corrected_in) }} →
                {{ formatGermanTime(data.correction.corrected_out) }}
              </template>
              <span v-else class="muted">—</span>
            </template>
          </Column>
          <Column header="Quelle">
            <template #body="{ data }">
              <span :class="{ muted: !periodSourceLabel(data.period.source) }">{{
                periodSourceLabel(data.period.source)
              }}</span>
            </template>
          </Column>
          <Column header="">
            <template #body="{ data }">
              <Button label="Korrigieren" size="small" text @click="openCorrect(data.period)" />
              <Button
                v-if="data.period.source === 'manual'"
                icon="pi pi-trash"
                severity="danger"
                size="small"
                text
                rounded
                @click="removeManual(data.period)"
              />
            </template>
          </Column>
        </DataTable>
      </template>
    </Card>

    <Dialog v-model:visible="showCorrect" header="Zeit korrigieren" modal :style="{ width: '440px' }">
      <p v-if="editWp" class="dlg-sub">Arbeitstag: {{ formatGermanDate(editWp.work_date) }}</p>
      <div class="form">
        <label>Kommen (Uhrzeit)</label>
        <input v-model="corrIn" type="time" step="60" class="p-inputtext p-component w" />
        <label>Gehen (Uhrzeit)</label>
        <input v-model="corrOut" type="time" step="60" class="p-inputtext p-component w" @input="onCorrOutInput" />
        <label>Grund (Pflicht)</label>
        <InputText v-model="corrReason" class="w" />
      </div>
      <template #footer>
        <Button label="Abbrechen" severity="secondary" text @click="showCorrect = false" />
        <Button label="Speichern" :loading="saving" data-testid="corrections-save-correction" @click="submitCorrect" />
      </template>
    </Dialog>

    <Dialog v-model:visible="showManual" header="Manuelle Arbeitszeit" modal :style="{ width: '440px' }">
      <div class="form">
        <label>Datum</label>
        <DatePicker
          v-model="manDate"
          date-format="dd.mm.yy"
          show-icon
          class="w"
          :max-date="manualDateMax"
        />
        <label>Kommen</label>
        <input v-model="manIn" type="time" step="60" class="p-inputtext p-component w" />
        <label>Gehen</label>
        <input v-model="manOut" type="time" step="60" class="p-inputtext p-component w" @input="onManOutInput" />
      </div>
      <template #footer>
        <Button label="Abbrechen" severity="secondary" text @click="showManual = false" />
        <Button label="Speichern" :loading="saving" data-testid="corrections-save-manual" @click="submitManual" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.page {
  max-width: 1100px;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  align-items: flex-end;
  margin-bottom: 1rem;
}
.emp {
  min-width: 220px;
}
.range {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  align-items: center;
}
.lbl {
  font-size: 0.85rem;
  color: #64748b;
}
.muted {
  color: #94a3b8;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.form label {
  font-size: 0.8rem;
  color: #64748b;
}
.w {
  width: 100%;
}
.dlg-sub {
  margin: 0 0 0.35rem;
  color: #334155;
  font-size: 0.9rem;
}
.row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-top: 0.5rem;
}
</style>
