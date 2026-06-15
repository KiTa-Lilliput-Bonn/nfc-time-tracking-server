<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Tag from 'primevue/tag'
import { useToast } from 'primevue/usetoast'

import ShiftAlertSettingsPanel from '@/components/ShiftAlertSettingsPanel.vue'
import TimeCorrectionDialog from '@/components/TimeCorrectionDialog.vue'
import {
  dismissShiftAlert,
  fetchEmployeeCorrections,
  fetchEmployeeTimes,
  fetchShiftAlerts,
} from '@/api/management'
import type { ShiftAlertItem, TimeCorrection, WorkPeriod } from '@/types/api'
import { getApiErrorMessage } from '@/utils/apiError'
import { formatGermanDate } from '@/utils/dates'
import { pickPrimaryWorkPeriod } from '@/utils/timeTableModel'

const toast = useToast()
const router = useRouter()

const loading = ref(true)
const err = ref('')
const items = ref<ShiftAlertItem[]>([])
const through = ref('')

const showCorrect = ref(false)
const activeRow = ref<ShiftAlertItem | null>(null)
const dayPeriods = ref<WorkPeriod[]>([])
const dayCorrections = ref<TimeCorrection[]>([])
const correctLoading = ref(false)
const dismissLoadingKey = ref('')

const reasonLabels: Record<string, string> = {
  long_duration: 'Lange Schicht',
  late_end: 'Spätes Ende',
}

async function load() {
  loading.value = true
  err.value = ''
  try {
    const data = await fetchShiftAlerts()
    items.value = data.items
    through.value = data.through
  } catch {
    err.value = 'Die Liste konnte nicht geladen werden.'
    items.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void load()
})

function formatDurationMinutes(min: number): string {
  const h = Math.floor(min / 60)
  const m = min % 60
  if (m === 0) return `${h} h`
  return `${h} h ${m} min`
}

function recordedLabel(row: ShiftAlertItem): string {
  return `${formatDurationMinutes(row.total_duration_minutes)}, Ende ${row.latest_end}`
}

function dismissKey(row: ShiftAlertItem): string {
  return `${row.user_id}-${row.work_date}`
}

async function openCorrect(row: ShiftAlertItem) {
  activeRow.value = row
  correctLoading.value = true
  showCorrect.value = true
  dayPeriods.value = []
  dayCorrections.value = []
  try {
    const [times, corrections] = await Promise.all([
      fetchEmployeeTimes(row.user_id, row.work_date, row.work_date),
      fetchEmployeeCorrections(row.user_id, row.work_date, row.work_date),
    ])
    dayPeriods.value = times.work_periods
    dayCorrections.value = corrections
  } catch (e) {
    showCorrect.value = false
    toast.add({
      severity: 'error',
      summary: 'Laden fehlgeschlagen',
      detail: getApiErrorMessage(e),
      life: 5000,
    })
  } finally {
    correctLoading.value = false
  }
}

async function dismissRow(row: ShiftAlertItem) {
  const key = dismissKey(row)
  dismissLoadingKey.value = key
  try {
    await dismissShiftAlert(row.user_id, row.work_date)
    await load()
    toast.add({ severity: 'success', summary: 'Als in Ordnung markiert', life: 3000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Speichern fehlgeschlagen',
      detail: getApiErrorMessage(e),
      life: 5000,
    })
  } finally {
    dismissLoadingKey.value = ''
  }
}

function correctionCandidates(): WorkPeriod[] {
  return dayPeriods.value.filter((p) => !p.is_break && p.punch_out)
}

function initialWorkPeriodId(): number | null {
  const primary = pickPrimaryWorkPeriod(correctionCandidates())
  return primary?.id ?? null
}

function onDialogSaved() {
  void load()
}

function onSettingsSaved() {
  void load()
}

function goEmployeeDetail(row: ShiftAlertItem) {
  void router.push({
    name: 'employee-detail',
    params: { id: String(row.user_id) },
    query: {
      year: String(row.iso_week_year),
      week: String(row.iso_week),
    },
  })
}

function onRowClick(e: { data: ShiftAlertItem }) {
  goEmployeeDetail(e.data)
}
</script>

<template>
  <div class="shift-alerts">
    <p class="back">
      <RouterLink to="/dashboard">← Dashboard</RouterLink>
    </p>
    <h1 class="page-title">Auffällige Arbeitszeiten</h1>
    <p class="sub">
      Erfasste Arbeitszeiten bis {{ through ? formatGermanDate(through) : 'gestern' }}, die länger als der Schwellwert
      sind oder nach der konfigurierten Uhrzeit enden. Zeile anklicken öffnet die Mitarbeiterdetailseite in der
      betreffenden Kalenderwoche.
    </p>

    <ShiftAlertSettingsPanel class="settings-panel" @saved="onSettingsSaved" />

    <p v-if="err" class="err">{{ err }}</p>
    <p v-else-if="loading" class="muted">Laden…</p>

    <Card v-else>
      <template #content>
        <p v-if="items.length === 0" class="muted">Keine auffälligen Arbeitszeiten.</p>
        <DataTable
          v-else
          :value="items"
          :row-key="(row: ShiftAlertItem) => `${row.user_id}-${row.work_date}`"
          striped-rows
          size="small"
          class="alerts-table alerts-table--clickable"
          data-testid="shift-alerts-table"
          @row-click="onRowClick"
        >
          <Column header="Datum" sortable sort-field="work_date">
            <template #body="{ data }">
              {{ formatGermanDate(data.work_date) }}
            </template>
          </Column>
          <Column field="display_name" header="Mitarbeiter*in" sortable />
          <Column header="Erfasst">
            <template #body="{ data }">
              {{ recordedLabel(data) }}
            </template>
          </Column>
          <Column header="Grund">
            <template #body="{ data }">
              <div class="reason-tags">
                <Tag
                  v-for="reason in data.reasons"
                  :key="reason"
                  :value="reasonLabels[reason] ?? reason"
                  severity="warn"
                />
              </div>
            </template>
          </Column>
          <Column header="Aktionen" style="min-width: 18rem">
            <template #body="{ data }">
              <div class="actions" @click.stop>
                <Button
                  label="Zeit korrigieren"
                  icon="pi pi-pencil"
                  size="small"
                  data-testid="shift-alert-correct-btn"
                  :loading="correctLoading && activeRow?.user_id === data.user_id && activeRow?.work_date === data.work_date"
                  @click="openCorrect(data)"
                />
                <Button
                  label="In Ordnung"
                  icon="pi pi-check"
                  size="small"
                  severity="secondary"
                  data-testid="shift-alert-dismiss-btn"
                  :loading="dismissLoadingKey === dismissKey(data)"
                  @click="dismissRow(data)"
                />
              </div>
            </template>
          </Column>
        </DataTable>
      </template>
    </Card>

    <TimeCorrectionDialog
      v-if="activeRow"
      v-model:visible="showCorrect"
      :dialog-date="activeRow.work_date"
      :candidates="correctionCandidates()"
      :initial-work-period-id="initialWorkPeriodId()"
      :periods="dayPeriods"
      :corrections="dayCorrections"
      :row-correction="{ mode: 'employee', employeeId: activeRow.user_id }"
      @saved="onDialogSaved"
    />
  </div>
</template>

<style scoped>
.shift-alerts {
  max-width: 1100px;
}
.settings-panel {
  margin-bottom: 1rem;
}
.back {
  margin: 0 0 0.5rem;
}
.back a {
  color: var(--p-primary-color);
  text-decoration: none;
}
.back a:hover {
  text-decoration: underline;
}
.page-title {
  margin: 0 0 0.35rem;
  font-size: 1.35rem;
}
.sub {
  margin: 0 0 1rem;
  color: var(--p-text-muted-color);
  font-size: 0.9rem;
}
.err {
  color: var(--p-red-500);
}
.muted {
  color: var(--p-text-muted-color);
}
.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
}
.reason-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem;
}
.alerts-table--clickable :deep(.p-datatable-tbody > tr) {
  cursor: pointer;
}
</style>
