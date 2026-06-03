<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'

import EmployeeAbsenceAddDialog from '@/components/EmployeeAbsenceAddDialog.vue'
import ManualWorkPeriodDialog from '@/components/ManualWorkPeriodDialog.vue'
import { fetchScheduleGaps } from '@/api/management'
import type { ScheduleGapItem } from '@/types/api'
import { formatGermanDate } from '@/utils/dates'

const router = useRouter()

const loading = ref(true)
const err = ref('')
const items = ref<ScheduleGapItem[]>([])
const through = ref('')

const showManual = ref(false)
const showAbsence = ref(false)
const activeRow = ref<ScheduleGapItem | null>(null)

async function load() {
  loading.value = true
  err.value = ''
  try {
    const data = await fetchScheduleGaps()
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

function shiftLabel(row: ScheduleGapItem): string {
  return `${row.shift_start} – ${row.shift_end}`
}

function openManualTime(row: ScheduleGapItem) {
  activeRow.value = row
  showManual.value = true
}

function openAbsence(row: ScheduleGapItem) {
  activeRow.value = row
  showAbsence.value = true
}

function goSchedule(row: ScheduleGapItem) {
  void router.push({
    name: 'schedule',
    query: {
      year: String(row.iso_week_year),
      week: String(row.iso_week),
    },
  })
}

function onDialogSaved() {
  void load()
}
</script>

<template>
  <div class="schedule-gaps">
    <p class="back">
      <RouterLink to="/dashboard">← Dashboard</RouterLink>
    </p>
    <h1 class="page-title">Offene Dienstplan-Tage</h1>
    <p class="sub">
      Geplante Schichten bis {{ through ? formatGermanDate(through) : 'gestern' }}, für die weder Arbeitszeiten noch eine
      Abwesenheit hinterlegt sind.
    </p>

    <p v-if="err" class="err">{{ err }}</p>
    <p v-else-if="loading" class="muted">Laden…</p>

    <Card v-else>
      <template #content>
        <p v-if="items.length === 0" class="muted">Keine offenen Tage.</p>
        <DataTable
          v-else
          :value="items"
          :row-key="(row: ScheduleGapItem) => `${row.user_id}-${row.schedule_date}`"
          striped-rows
          size="small"
          class="gaps-table"
          data-testid="schedule-gaps-table"
        >
          <Column header="Datum" sortable sort-field="schedule_date">
            <template #body="{ data }">
              {{ formatGermanDate(data.schedule_date) }}
            </template>
          </Column>
          <Column field="display_name" header="Mitarbeiter*in" sortable />
          <Column header="Geplant">
            <template #body="{ data }">
              {{ shiftLabel(data) }}
            </template>
          </Column>
          <Column header="Aktionen" style="min-width: 22rem">
            <template #body="{ data }">
              <div class="actions">
                <Button
                  label="Zeit erfassen"
                  icon="pi pi-clock"
                  size="small"
                  data-testid="schedule-gap-time-btn"
                  @click="openManualTime(data)"
                />
                <Button
                  label="Dienstplan bearbeiten"
                  icon="pi pi-calendar"
                  size="small"
                  severity="secondary"
                  data-testid="schedule-gap-schedule-btn"
                  @click="goSchedule(data)"
                />
                <Button
                  label="Abwesenheit eintragen"
                  icon="pi pi-user-minus"
                  size="small"
                  severity="secondary"
                  data-testid="schedule-gap-absence-btn"
                  @click="openAbsence(data)"
                />
              </div>
            </template>
          </Column>
        </DataTable>
      </template>
    </Card>

    <ManualWorkPeriodDialog
      v-if="activeRow"
      v-model:visible="showManual"
      :employee-id="activeRow.user_id"
      :employee-name="activeRow.display_name"
      :work-date="activeRow.schedule_date"
      :shift-start="activeRow.shift_start"
      :shift-end="activeRow.shift_end"
      @saved="onDialogSaved"
    />
    <EmployeeAbsenceAddDialog
      v-if="activeRow"
      v-model:visible="showAbsence"
      :employee-id="activeRow.user_id"
      :display-name="activeRow.display_name"
      :absence-date="activeRow.schedule_date"
      @saved="onDialogSaved"
    />
  </div>
</template>

<style scoped>
.schedule-gaps {
  max-width: 1100px;
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
</style>
