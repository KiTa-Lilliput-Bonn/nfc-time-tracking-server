<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'

import WeekWorkTimeCalendar from '@/components/WeekWorkTimeCalendar.vue'
import { fetchMeAbsences, fetchMeSchedule, fetchMeTimes } from '@/api/me'
import { addDays, formatGermanDate, isoWeekAndYear, startOfISOWeek, toISODateLocal } from '@/utils/dates'
import type { Absence, HolidayCredit, Schedule, TeamMeeting } from '@/types/api'

const weekStart = ref(startOfISOWeek(new Date()))
const schedules = ref<Schedule[]>([])
const absences = ref<Absence[]>([])
const holidays = ref<HolidayCredit[]>([])
const teamMeetings = ref<TeamMeeting[]>([])
const loading = ref(false)
const err = ref('')

/** Freitag derselben Kalenderwoche (Mo–Fr-Ansicht ohne Sa/So) */
const weekEndFriday = computed(() => addDays(weekStart.value, 4))

const weekLabel = computed(() => {
  const { year, week } = isoWeekAndYear(weekStart.value)
  const a = toISODateLocal(weekStart.value)
  const b = toISODateLocal(weekEndFriday.value)
  return `KW ${week} (${year}) · ${formatGermanDate(a)}–${formatGermanDate(b)}`
})

const scheduleByDate = computed(() => {
  const m: Record<string, { shift_start: string; shift_end: string }> = {}
  for (const s of schedules.value) {
    const key = s.schedule_date.slice(0, 10)
    m[key] = { shift_start: s.shift_start, shift_end: s.shift_end }
  }
  return m
})

const weekdayDE = ['So', 'Mo', 'Di', 'Mi', 'Do', 'Fr', 'Sa']

interface Row {
  date: string
  weekday: string
  shift_start: string
  shift_end: string
  /** Geplanter Urlaub oder Ausgleichstag laut Abwesenheiten */
  plannedAbsence: string
}

function dateKeyYMD(s: string) {
  return s.length >= 10 ? s.slice(0, 10) : s
}

function labelVacationOrComp(a: Absence) {
  const half = a.half_day ? ' (½ Tag)' : ''
  if (a.absence_type === 'vacation') return `Urlaub${half}`
  if (a.absence_type === 'compensation_day') return `Ausgleichstag${half}`
  return ''
}

const rows = computed<Row[]>(() => {
  const map = new Map<string, Schedule>()
  for (const s of schedules.value) {
    map.set(dateKeyYMD(s.schedule_date), s)
  }
  const absMap = new Map<string, Absence>()
  for (const a of absences.value) {
    if (a.absence_type !== 'vacation' && a.absence_type !== 'compensation_day') continue
    absMap.set(dateKeyYMD(a.absence_date), a)
  }
  const out: Row[] = []
  for (let i = 0; i < 5; i++) {
    const d = addDays(weekStart.value, i)
    const ds = toISODateLocal(d)
    const sch = map.get(ds)
    const ab = absMap.get(ds)
    const planned = ab ? labelVacationOrComp(ab) : ''
    out.push({
      date: ds,
      weekday: weekdayDE[d.getDay()],
      shift_start: sch?.shift_start ?? '—',
      shift_end: sch?.shift_end ?? '—',
      plannedAbsence: planned || '—',
    })
  }
  return out
})

async function load() {
  loading.value = true
  err.value = ''
  const from = toISODateLocal(weekStart.value)
  const to = toISODateLocal(weekEndFriday.value)
  try {
    const [data, absData, times] = await Promise.all([
      fetchMeSchedule(from, to),
      fetchMeAbsences(from, to),
      fetchMeTimes(from, to),
    ])
    schedules.value = data.schedules
    absences.value = absData.absences
    holidays.value = times.holidays ?? []
    teamMeetings.value = data.team_meetings ?? []
  } catch {
    err.value = 'Dienstplan konnte nicht geladen werden.'
    schedules.value = []
    absences.value = []
    holidays.value = []
    teamMeetings.value = []
  } finally {
    loading.value = false
  }
}

function setThisWeek() {
  weekStart.value = startOfISOWeek(new Date())
}

function prevWeek() {
  weekStart.value = addDays(weekStart.value, -7)
}

function nextWeek() {
  weekStart.value = addDays(weekStart.value, 7)
}

onMounted(load)
watch(weekStart, load)
</script>

<template>
  <div class="page">
    <div class="toolbar">
      <div class="nav">
        <Button icon="pi pi-chevron-left" rounded text severity="secondary" aria-label="Vorherige Woche" @click="prevWeek" />
        <span class="week-cap">{{ weekLabel }}</span>
        <Button icon="pi pi-chevron-right" rounded text severity="secondary" aria-label="Nächste Woche" @click="nextWeek" />
      </div>
      <Button label="Diese Woche" severity="secondary" text @click="setThisWeek" />
    </div>
    <p v-if="err" class="err">{{ err }}</p>
    <WeekWorkTimeCalendar
      class="schedule-cal"
      :week-start="weekStart"
      :periods="[]"
      :absences="absences"
      :holidays="holidays"
      :schedule-by-date="scheduleByDate"
      :team-meetings="teamMeetings"
      :loading="loading"
      mode="schedule"
    />
    <DataTable :value="rows" :loading="loading" size="small">
      <Column field="date" header="Datum">
        <template #body="{ data }">{{ formatGermanDate(data.date) }}</template>
      </Column>
      <Column field="weekday" header="Tag" />
      <Column field="shift_start" header="Beginn" />
      <Column field="shift_end" header="Ende" />
      <Column field="plannedAbsence" header="Geplant">
        <template #body="{ data }">
          <span :class="{ 'hint-abs': data.plannedAbsence !== '—' }">{{ data.plannedAbsence }}</span>
        </template>
      </Column>
    </DataTable>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  max-width: min(1200px, 100%);
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 1rem;
  justify-content: space-between;
}
.nav {
  display: flex;
  align-items: center;
  gap: 0.35rem;
}
.week-cap {
  font-size: 0.9rem;
  color: #475569;
  font-weight: 500;
}
.schedule-cal {
  width: 100%;
}
.err {
  color: #b91c1c;
  margin: 0;
}
.hint-abs {
  color: #1d4ed8;
  font-weight: 500;
}
</style>
