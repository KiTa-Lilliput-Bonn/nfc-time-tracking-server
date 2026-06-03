<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'

import WeekWorkTimeCalendar from '@/components/WeekWorkTimeCalendar.vue'
import {
  fetchMeAbsences,
  fetchMeCorrections,
  fetchMeSchedule,
  fetchMeScheduleBound,
  fetchMeTimes,
} from '@/api/me'
import { addDays, formatGermanDate, isoWeekAndYear, startOfISOWeek, toISODateLocal } from '@/utils/dates'
import type {
  Absence,
  HolidayCredit,
  Schedule,
  ScheduleBoundSetting,
  TeamMeeting,
  TimeCorrection,
  WorkPeriod,
} from '@/types/api'

const weekStart = ref(startOfISOWeek(new Date()))
const periods = ref<WorkPeriod[]>([])
const absences = ref<Absence[]>([])
const corrections = ref<TimeCorrection[]>([])
const schedules = ref<Schedule[]>([])
const holidays = ref<HolidayCredit[]>([])
const teamMeetings = ref<TeamMeeting[]>([])
const scheduleBoundList = ref<ScheduleBoundSetting[]>([])
const loading = ref(false)
const err = ref('')

const weekEndFriday = computed(() => addDays(weekStart.value, 4))

const weekLabel = computed(() => {
  const { year, week } = isoWeekAndYear(weekStart.value)
  const a = toISODateLocal(weekStart.value)
  const b = toISODateLocal(weekEndFriday.value)
  return `KW ${week} (${year}) · ${formatGermanDate(a)}–${formatGermanDate(b)}`
})

async function load() {
  loading.value = true
  err.value = ''
  const f = toISODateLocal(weekStart.value)
  const t = toISODateLocal(weekEndFriday.value)
  try {
    const [times, abs, corr, sch, sb] = await Promise.all([
      fetchMeTimes(f, t),
      fetchMeAbsences(f, t),
      fetchMeCorrections(f, t),
      fetchMeSchedule(f, t),
      fetchMeScheduleBound().catch((): ScheduleBoundSetting[] => []),
    ])
    periods.value = times.work_periods
    holidays.value = times.holidays ?? []
    absences.value = abs.absences
    corrections.value = corr.corrections
    schedules.value = sch.schedules
    teamMeetings.value = sch.team_meetings ?? []
    scheduleBoundList.value = sb
  } catch {
    err.value = 'Daten konnten nicht geladen werden.'
    periods.value = []
    holidays.value = []
    absences.value = []
    corrections.value = []
    schedules.value = []
    teamMeetings.value = []
    scheduleBoundList.value = []
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

const scheduleByDate = computed(() => {
  const m: Record<string, { shift_start: string; shift_end: string }> = {}
  for (const s of schedules.value) {
    const key = s.schedule_date.slice(0, 10)
    m[key] = { shift_start: s.shift_start, shift_end: s.shift_end }
  }
  return m
})
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
      :week-start="weekStart"
      :periods="periods"
      :absences="absences"
      :corrections="corrections"
      :holidays="holidays"
      :schedule-by-date="scheduleByDate"
      :team-meetings="teamMeetings"
      :schedule-bound-history="scheduleBoundList"
      :loading="loading"
      :dual-track="true"
    />
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 1rem;
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
.err {
  color: #b91c1c;
  margin: 0;
}
</style>
