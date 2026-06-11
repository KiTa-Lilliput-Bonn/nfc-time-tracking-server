<script setup lang="ts">
import { computed, ref } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import TimeCorrectionDialog from '@/components/TimeCorrectionDialog.vue'
import { useToast } from 'primevue/usetoast'
import type {
  Absence,
  BreakRule,
  FixedNonWorkWeekdays,
  ScheduleBoundSetting,
  TimeCorrection,
  WeeklyHours,
  WorkPeriod,
} from '@/types/api'
import type { HolidayCredit } from '@/types/api'
import { absenceByDate, buildTimeTableRows, holidayByDate, type TimeTableRow } from '@/utils/timeTableModel'
import { formatGermanDate, formatGermanTime } from '@/utils/dates'

const props = defineProps<{
  periods: WorkPeriod[]
  absences?: Absence[]
  corrections?: TimeCorrection[]
  holidays?: HolidayCredit[]
  /** Pro Kalendertag (YYYY-MM-DD): Schicht aus dem Dienstplan — für effektiven Beginn max(Stempel, Schichtbeginn). */
  scheduleByDate?: Record<string, { shift_start: string; shift_end: string }>
  breakRules?: BreakRule[]
  roundingMinutes?: number
  weeklyHours?: WeeklyHours[]
  fixedNonWorkWeekdays?: number[]
  fixedNonWorkWeekdaysHistory?: FixedNonWorkWeekdays[]
  scheduleBoundHistory?: ScheduleBoundSetting[]
  loading?: boolean
  rowCorrection?: { mode: 'self' } | { mode: 'employee'; employeeId: number }
}>()

const emit = defineEmits<{
  dataChanged: []
}>()

const toast = useToast()

const absByDate = computed(() => absenceByDate(props.absences))

const holByDate = computed(() => holidayByDate(props.holidays))

const rows = computed<TimeTableRow[]>(() =>
  buildTimeTableRows({
    periods: props.periods,
    absences: props.absences,
    corrections: props.corrections,
    holidays: props.holidays,
    scheduleByDate: props.scheduleByDate,
    breakRules: props.breakRules,
    roundingMinutes: props.roundingMinutes,
    weeklyHours: props.weeklyHours,
    fixedNonWorkWeekdays: props.fixedNonWorkWeekdays,
    fixedNonWorkWeekdaysHistory: props.fixedNonWorkWeekdaysHistory,
    scheduleBoundHistory: props.scheduleBoundHistory,
  }),
)

const rowClickable = computed(() => !!props.rowCorrection)

function rowClass(data: TimeTableRow) {
  const abs = absByDate.value[data.workDate]
  if (holByDate.value[data.workDate]) return 'row-holiday'
  if (abs?.absence_type === 'vacation') return 'row-vacation'
  if (abs?.absence_type === 'sick') return 'row-sick'
  if (abs?.absence_type === 'compensation_day') return 'row-compensation'
  if (abs?.absence_type === 'other') return 'row-other'
  return ''
}

/** Inline-Style am <tr> (PrimeVue), wirkt auch auf Zellen (cursor vererbt). */
function rowDataStyle(_data: TimeTableRow) {
  if (!props.rowCorrection) return undefined
  return { cursor: 'pointer' }
}

// --- Korrektur-Dialog (Zeilenklick) ---

const showCorrect = ref(false)
const dialogDate = ref('')
const dialogCandidates = ref<WorkPeriod[]>([])
const initialWpId = ref<number | null>(null)

function onRowClick(e: { data: TimeTableRow }) {
  if (!props.rowCorrection) return
  const row = e.data
  if (!row.candidates.length) {
    toast.add({
      severity: 'info',
      summary: 'Kein Eintrag',
      detail: 'Für diesen Tag gibt es keine korrigierbare Arbeitszeit (nur Abwesenheit oder leer).',
      life: 10000,
    })
    return
  }
  dialogDate.value = row.workDate
  dialogCandidates.value = row.candidates
  initialWpId.value = row.primaryPeriodId
  showCorrect.value = true
}
</script>

<template>
  <div :class="rowClickable ? 'tt-wrap tt-click' : 'tt-wrap'">
    <DataTable
      :value="rows"
      :loading="loading"
      striped-rows
      :row-class="(d: TimeTableRow) => rowClass(d)"
      :row-style="rowDataStyle"
      :row-hover="!!rowClickable"
      data-key="rowKey"
      size="small"
      @row-click="onRowClick"
    >
      <Column field="workDate" header="Datum" sortable>
        <template #body="{ data }: { data: TimeTableRow }">
          {{ formatGermanDate(data.workDate) }}
        </template>
      </Column>
      <Column header="Beginn">
        <template #body="{ data }: { data: TimeTableRow }">
          <template v-if="data.effectiveIn">
            <span class="tt-begin-wrap">
              <span class="tt-begin-main">{{ formatGermanTime(data.effectiveIn) }}</span>
              <span
                v-if="
                  data.stampInEarliest &&
                  new Date(data.stampInEarliest).getTime() !== new Date(data.effectiveIn).getTime()
                "
                class="tt-begin-stamp"
                title="Einstempelzeit"
              >
                {{ formatGermanTime(data.stampInEarliest) }}
              </span>
            </span>
          </template>
          <template v-else>—</template>
        </template>
      </Column>
      <Column header="Ende">
        <template #body="{ data }: { data: TimeTableRow }">
          {{ data.effectiveOut ? formatGermanTime(data.effectiveOut) : '—' }}
        </template>
      </Column>
      <Column field="gross" header="Brutto (h)" />
      <Column field="net" header="Netto (h)" />
      <Column header="Hinweise">
        <template #body="{ data }: { data: TimeTableRow }">
          <span v-if="data.notes">{{ data.notes }}</span>
          <span v-if="data.rowHint" class="tt-row-hint">{{ data.rowHint }}</span>
        </template>
      </Column>
    </DataTable>

    <TimeCorrectionDialog
      v-if="rowCorrection"
      v-model:visible="showCorrect"
      :dialog-date="dialogDate"
      :candidates="dialogCandidates"
      :initial-work-period-id="initialWpId ?? undefined"
      :periods="periods"
      :corrections="corrections"
      :row-correction="rowCorrection"
      @saved="emit('dataChanged')"
    />
  </div>
</template>

<style>
/* Zusatz zu rowStyle: tr[role=row] ist die Datenzeile in PrimeVue 4 */
.tt-wrap.tt-click .p-datatable-tbody tr[role='row'] {
  cursor: pointer;
}
.tt-wrap.tt-click .p-datatable-tbody tr[role='row'] > td {
  cursor: pointer;
}
.row-vacation > td {
  background: #eef2ff !important;
}
.row-sick > td {
  background: #fef3c7 !important;
}
.row-other > td {
  background: #f3f4f6 !important;
}
.row-compensation > td {
  background: #e0f2fe !important;
}
.row-holiday > td {
  background: #ecfdf5 !important;
}
.tt-begin-wrap {
  display: inline-flex;
  align-items: baseline;
  gap: 0.35rem;
  flex-wrap: wrap;
}
.tt-begin-stamp {
  font-size: 0.72rem;
  color: #64748b;
  font-weight: 400;
}
.tt-row-hint {
  display: block;
  font-size: 0.72rem;
  color: #64748b;
  font-weight: 400;
  margin-top: 0.15rem;
}
</style>
