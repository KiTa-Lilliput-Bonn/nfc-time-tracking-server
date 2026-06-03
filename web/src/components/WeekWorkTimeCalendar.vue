<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, withDefaults } from 'vue'
import type {
  Absence,
  HolidayCredit,
  ScheduleBoundSetting,
  TeamMeeting,
  TimeCorrection,
  WorkPeriod,
} from '@/types/api'
import TimeCorrectionDialog from '@/components/TimeCorrectionDialog.vue'
import { addDays, formatGermanDate, formatGermanTime, toISODateLocal } from '@/utils/dates'
import {
  absenceByDate,
  buildCalendarSegmentsForDay,
  correctionByWorkPeriod,
  dayStatusClass,
  holidayByDate,
  hasPlannedShift,
  type CalendarSegment,
} from '@/utils/timeTableModel'
import { teamMeetingBarTag } from '@/utils/teamMeetingLabel'

const GRID_START_H = 6
const GRID_END_H = 20
const BANNER_H = 26
const HOUR_PX = 44

const props = withDefaults(
  defineProps<{
    weekStart: Date
    periods: WorkPeriod[]
    absences?: Absence[]
    corrections?: TimeCorrection[]
    holidays?: HolidayCredit[]
    scheduleByDate?: Record<string, { shift_start: string; shift_end: string }>
    /** Geplante Teamsitzungen (Montag = meeting_date), gefiltert auf den Kalender-Nutzer */
    teamMeetings?: TeamMeeting[]
    scheduleBoundHistory?: ScheduleBoundSetting[]
    loading?: boolean
    rowCorrection?: { mode: 'self' } | { mode: 'employee'; employeeId: number }
    /** schedule: nur Schicht/Abwesenheit/Feiertag; times: Stempelblöcke (optional dualTrack) */
    mode?: 'schedule' | 'times'
    /** Zwei Spalten pro Tag: links Geplant (Schicht), rechts Gebucht (Stempel, ohne Schicht-Clamp) */
    dualTrack?: boolean
  }>(),
  { mode: 'times', dualTrack: false, teamMeetings: () => [] },
)

const emit = defineEmits<{
  dataChanged: []
}>()

const weekdayDE = ['So', 'Mo', 'Di', 'Mi', 'Do', 'Fr', 'Sa']

const gridMinutes = (GRID_END_H - GRID_START_H) * 60
const gridPx = (GRID_END_H - GRID_START_H) * HOUR_PX

const hourLabels = computed(() => {
  const out: number[] = []
  for (let h = GRID_START_H; h < GRID_END_H; h++) out.push(h)
  return out
})

const days = computed(() => {
  const out: { iso: string; weekday: string; label: string; isToday: boolean }[] = []
  const today = toISODateLocal(new Date())
  for (let i = 0; i < 5; i++) {
    const d = addDays(props.weekStart, i)
    const iso = toISODateLocal(d)
    out.push({
      iso,
      weekday: weekdayDE[d.getDay()] ?? '',
      label: formatGermanDate(iso),
      isToday: iso === today,
    })
  }
  return out
})

const periodsByDate = computed(() => {
  const m = new Map<string, WorkPeriod[]>()
  for (const p of props.periods) {
    const arr = m.get(p.work_date) ?? []
    arr.push(p)
    m.set(p.work_date, arr)
  }
  return m
})

const corrMap = computed(() => correctionByWorkPeriod(props.corrections))
const absMap = computed(() => absenceByDate(props.absences))
const holMap = computed(() => holidayByDate(props.holidays))

const isScheduleMode = computed(() => props.mode === 'schedule')
const useDualLayout = computed(() => props.dualTrack && !isScheduleMode.value)

function clockMinutesLocal(iso: string): number {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return 0
  return d.getHours() * 60 + d.getMinutes() + d.getSeconds() / 60
}

/** 0–100 relativ zum Raster [GRID_START_H, GRID_END_H]. */
function segmentLayout(startIso: string, endIso: string | null): { topPct: number; heightPct: number } {
  const startM = clockMinutesLocal(startIso)
  const endM = endIso ? clockMinutesLocal(endIso) : GRID_END_H * 60
  const g0 = GRID_START_H * 60
  const g1 = GRID_END_H * 60
  const s = Math.max(g0, Math.min(g1, startM))
  const e = Math.max(g0, Math.min(g1, endM))
  const dur = Math.max(e - s, 12)
  return {
    topPct: ((s - g0) / gridMinutes) * 100,
    heightPct: (dur / gridMinutes) * 100,
  }
}

function timeTopPct(iso: string): number {
  const startM = clockMinutesLocal(iso)
  const g0 = GRID_START_H * 60
  const g1 = GRID_END_H * 60
  const s = Math.max(g0, Math.min(g1, startM))
  return ((s - g0) / gridMinutes) * 100
}

function shiftLayout(
  workDate: string,
  shiftStart: string,
  shiftEnd: string,
): { topPct: number; heightPct: number } | null {
  const a = shiftStart.trim()
  const b = shiftEnd.trim()
  if (!a || !b) return null
  const partsA = a.split(':').map((x) => Number.parseInt(x.replace(/\D/g, ''), 10))
  const partsB = b.split(':').map((x) => Number.parseInt(x.replace(/\D/g, ''), 10))
  if (partsA.length < 2 || partsB.length < 2) return null
  const inM = partsA[0]! * 60 + (partsA[1] ?? 0)
  const outM = partsB[0]! * 60 + (partsB[1] ?? 0)
  if (outM <= inM) return null
  const d0 = new Date(workDate + 'T12:00:00')
  const y = d0.getFullYear()
  const mo = d0.getMonth()
  const da = d0.getDate()
  const startIso = new Date(y, mo, da, partsA[0], partsA[1] ?? 0, 0, 0).toISOString()
  const endIso = new Date(y, mo, da, partsB[0], partsB[1] ?? 0, 0, 0).toISOString()
  return segmentLayout(startIso, endIso)
}

/** Zwei Uhrzeit-Intervalle überlappen im gleichen Tagesraster (wie shiftLayout). */
function intervalsOverlapVerticalOnDay(
  iso: string,
  startA: string,
  endA: string,
  startB: string,
  endB: string,
): boolean {
  const a = shiftLayout(iso, startA, endA)
  const b = shiftLayout(iso, startB, endB)
  if (!a || !b) return false
  const a0 = a.topPct
  const a1 = a.topPct + a.heightPct
  const b0 = b.topPct
  const b1 = b.topPct + b.heightPct
  return a0 < b1 - 0.08 && a1 > b0 + 0.08
}

function meetingOverlapsPlannedShift(iso: string, m: TeamMeeting): boolean {
  if (!hasPlannedShift(iso, props.scheduleByDate)) return false
  const sch = props.scheduleByDate?.[iso]
  if (!sch?.shift_start?.trim() || !sch?.shift_end?.trim()) return false
  return intervalsOverlapVerticalOnDay(iso, m.time_start, m.time_end, sch.shift_start, sch.shift_end)
}

/** Irgendeine Teamsitzung überlappt die geplante Schicht → Schicht-Zeiten links, Meeting rechts. */
function shiftOverlapsAnyMeeting(iso: string): boolean {
  return meetingsForIsoDay(iso).some((m) => meetingOverlapsPlannedShift(iso, m))
}

function statusBannerText(iso: string): string {
  const h = holMap.value[iso]
  if (h) return `Feiertag: ${h.name}`
  const a = absMap.value[iso]
  if (!a) return ''
  const half = a.half_day ? ' (½ Tag)' : ''
  if (a.absence_type === 'vacation') return `Urlaub${half}`
  if (a.absence_type === 'sick') return `Krank${half}`
  if (a.absence_type === 'compensation_day') return `Ausgleichstag${half}`
  if (a.absence_type === 'other') return `Abwesend${half}`
  return ''
}

function segmentsForDay(iso: string): CalendarSegment[] {
  if (isScheduleMode.value) return []
  const list = periodsByDate.value.get(iso) ?? []
  const dayMeetings = meetingsForIsoDay(iso)
  if (useDualLayout.value) {
    return buildCalendarSegmentsForDay(iso, list, corrMap.value, props.scheduleByDate, {
      applyShiftClamp: false,
    })
  }
  return buildCalendarSegmentsForDay(iso, list, corrMap.value, props.scheduleByDate, {
    teamMeetings: dayMeetings,
    scheduleBoundHistory: props.scheduleBoundHistory,
  })
}

function hasBookedSegmentsForDay(iso: string): boolean {
  return segmentsForDay(iso).length > 0
}

/** Woche hat mindestens einen Tag mit Buchung → Legende und breitere Mindestbreite. */
const weekHasAnyBookedTime = computed(() => {
  if (!useDualLayout.value) return false
  return days.value.some((d) => hasBookedSegmentsForDay(d.iso))
})

function dayColClass(iso: string) {
  return dayStatusClass(iso, props.absences, props.holidays)
}

function shiftStyleForDay(iso: string): Record<string, string> | undefined {
  const sch = props.scheduleByDate?.[iso]
  if (!sch || !hasPlannedShift(iso, props.scheduleByDate)) return undefined
  const b = shiftLayout(iso, sch.shift_start, sch.shift_end)
  if (!b) return undefined
  return { top: b.topPct + '%', height: b.heightPct + '%' }
}

function dateKeyYMD(s: string) {
  return s.length >= 10 ? s.slice(0, 10) : s
}

function meetingsForIsoDay(iso: string): TeamMeeting[] {
  return (props.teamMeetings ?? []).filter((m) => dateKeyYMD(m.meeting_date) === iso)
}

function meetingBarStyle(iso: string, m: TeamMeeting): Record<string, string> | undefined {
  const b = shiftLayout(iso, m.time_start, m.time_end)
  if (!b) return undefined
  const overlaps = meetingOverlapsPlannedShift(iso, m)
  return {
    top: b.topPct + '%',
    height: Math.max(b.heightPct, 0.6) + '%',
    left: overlaps ? 'calc(50% + 2px)' : '2px',
    right: '2px',
    zIndex: '2',
  }
}

function meetingRowsForDay(iso: string): { meeting: TeamMeeting; style: Record<string, string> }[] {
  const out: { meeting: TeamMeeting; style: Record<string, string> }[] = []
  for (const m of meetingsForIsoDay(iso)) {
    const st = meetingBarStyle(iso, m)
    if (st) out.push({ meeting: m, style: st })
  }
  return out
}

function meetingBarLabel(m: TeamMeeting): string {
  const tag = teamMeetingBarTag(m)
  return `${tag} ${normalizeShiftClock(m.time_start)}–${normalizeShiftClock(m.time_end)}`
}

/** Anzeige HH:MM aus Dienstplan-Feldern (ohne Datum). */
function normalizeShiftClock(s: string): string {
  const t = s.trim()
  const m = t.match(/^(\d{1,2}):(\d{2})/)
  if (m) {
    const h = Number.parseInt(m[1]!, 10)
    const min = m[2]!
    if (Number.isFinite(h)) return `${String(h).padStart(2, '0')}:${min}`
  }
  return t
}

function shiftTimeDisplay(iso: string): { start: string; end: string } | null {
  const sch = props.scheduleByDate?.[iso]
  if (!sch?.shift_start?.trim() || !sch?.shift_end?.trim()) return null
  return {
    start: normalizeShiftClock(sch.shift_start),
    end: normalizeShiftClock(sch.shift_end),
  }
}

function segBlockStyle(seg: CalendarSegment): Record<string, string> {
  const { topPct, heightPct } = segmentLayout(seg.effectiveIn, seg.effectiveOut)
  return { top: topPct + '%', height: heightPct + '%' }
}

function blockEndTimeLabel(seg: CalendarSegment): string {
  return seg.effectiveOut ? formatGermanTime(seg.effectiveOut) : '…'
}

const nowTick = ref(0)
let nowTimer: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  nowTimer = setInterval(() => {
    nowTick.value++
  }, 60_000)
})
onUnmounted(() => {
  if (nowTimer) clearInterval(nowTimer)
})

const nowLineStyle = computed(() => {
  nowTick.value
  const today = toISODateLocal(new Date())
  const inWeek = days.value.some((d) => d.iso === today)
  if (!inWeek) return null
  const topPct = timeTopPct(new Date().toISOString())
  return {
    top: `calc(${BANNER_H}px + ${(topPct / 100) * gridPx}px)`,
  }
})

const showCorrect = ref(false)
const dialogDate = ref('')
const dialogCandidates = ref<WorkPeriod[]>([])
const initialWpId = ref<number | null>(null)

function onWorkBlockClick(iso: string, workPeriodId: number) {
  if (!props.rowCorrection) return
  const candidates = props.periods
    .filter((p) => p.work_date === iso && !p.is_break)
    .slice()
    .sort((a, b) => a.punch_in.localeCompare(b.punch_in))
  if (!candidates.length) return
  dialogDate.value = iso
  dialogCandidates.value = candidates
  initialWpId.value = workPeriodId
  showCorrect.value = true
}
</script>

<template>
  <div class="wc-root">
    <div v-if="loading" class="wc-loading muted">Laden…</div>
    <div class="wc-shell" :class="{ dim: loading }">
      <div class="wc-head">
        <div class="wc-corner" />
        <div
          v-for="d in days"
          :key="d.iso"
          class="wc-day-head"
          :class="[dayColClass(d.iso), { today: d.isToday }]"
        >
          <span class="wc-dow">{{ d.weekday }}</span>
          <span class="wc-dlabel">{{ d.label }}</span>
        </div>
      </div>

      <div v-if="useDualLayout && weekHasAnyBookedTime" class="wc-legend" aria-hidden="true">
        <span class="wc-legend-item wc-legend-planned">Geplant (Schicht)</span>
        <span class="wc-legend-item wc-legend-booked">Gebucht (Stempel)</span>
      </div>

      <div class="wc-scroll">
        <div class="wc-scroll-inner" :class="{ 'wc-scroll-wide': useDualLayout && weekHasAnyBookedTime }">
          <div class="wc-gutter">
            <div class="wc-gutter-banner" :style="{ height: BANNER_H + 'px' }" />
            <div
              v-for="h in hourLabels"
              :key="h"
              class="wc-gutter-hour"
              :style="{ height: HOUR_PX + 'px' }"
            >
              {{ String(h).padStart(2, '0') }}:00
            </div>
          </div>

          <div class="wc-days">
            <div v-for="d in days" :key="d.iso" class="wc-day-col" :class="dayColClass(d.iso)">
              <div class="wc-banner" :class="{ empty: !statusBannerText(d.iso) }">
                {{ statusBannerText(d.iso) || '\u00a0' }}
              </div>
              <div
                class="wc-grid-wrap"
                :class="{ 'wc-grid-wrap-dual': useDualLayout && hasBookedSegmentsForDay(d.iso) }"
                :style="{ height: gridPx + 'px' }"
              >
                <div class="wc-grid-bg" />
                <template v-if="useDualLayout && hasBookedSegmentsForDay(d.iso)">
                  <div class="wc-dual-tracks">
                    <div class="wc-track wc-track-planned">
                      <div
                        v-if="shiftStyleForDay(d.iso) && shiftTimeDisplay(d.iso)"
                        class="wc-shift"
                        :class="{ 'wc-shift--with-meeting-overlap': shiftOverlapsAnyMeeting(d.iso) }"
                        :style="shiftStyleForDay(d.iso)"
                      >
                        <span class="wc-shift-time">{{ shiftTimeDisplay(d.iso)!.start }}</span>
                        <span class="wc-shift-time">{{ shiftTimeDisplay(d.iso)!.end }}</span>
                      </div>
                      <div
                        v-for="row in meetingRowsForDay(d.iso)"
                        :key="'mt-' + row.meeting.id"
                        class="wc-meeting"
                        :class="{ 'wc-meeting--overlap-shift': meetingOverlapsPlannedShift(d.iso, row.meeting) }"
                        :style="row.style"
                      >
                        <span class="wc-meeting-label">{{ meetingBarLabel(row.meeting) }}</span>
                      </div>
                    </div>
                    <div class="wc-track wc-track-booked">
                      <div
                        v-for="seg in segmentsForDay(d.iso)"
                        :key="seg.workPeriodId + (seg.segmentSuffix ?? '') + (seg.isBreak ? '-b' : '-w')"
                        class="wc-block-wrap"
                        :class="{
                          break: seg.isBreak,
                          work: !seg.isBreak,
                          click: rowCorrection && !seg.isBreak,
                        }"
                        :style="segBlockStyle(seg)"
                        @click="
                          !seg.isBreak && rowCorrection ? onWorkBlockClick(d.iso, seg.workPeriodId) : undefined
                        "
                      >
                        <div class="wc-block">
                          <span class="wc-time-row wc-time-start">{{ formatGermanTime(seg.effectiveIn) }}</span>
                          <span class="wc-block-kind">{{ seg.isBreak ? 'Pause' : 'Arbeit' }}</span>
                          <span class="wc-time-row wc-time-end">{{ blockEndTimeLabel(seg) }}</span>
                        </div>
                      </div>
                    </div>
                  </div>
                </template>
                <template v-else>
                  <div
                    v-if="shiftStyleForDay(d.iso) && shiftTimeDisplay(d.iso)"
                    class="wc-shift"
                    :class="{ 'wc-shift--with-meeting-overlap': shiftOverlapsAnyMeeting(d.iso) }"
                    :style="shiftStyleForDay(d.iso)"
                  >
                    <span class="wc-shift-time">{{ shiftTimeDisplay(d.iso)!.start }}</span>
                    <span class="wc-shift-time">{{ shiftTimeDisplay(d.iso)!.end }}</span>
                  </div>
                  <div
                    v-for="row in meetingRowsForDay(d.iso)"
                    :key="'mt-' + row.meeting.id"
                    class="wc-meeting"
                    :class="{ 'wc-meeting--overlap-shift': meetingOverlapsPlannedShift(d.iso, row.meeting) }"
                    :style="row.style"
                  >
                    <span class="wc-meeting-label">{{ meetingBarLabel(row.meeting) }}</span>
                  </div>
                  <template v-if="!isScheduleMode">
                    <div
                      v-for="seg in segmentsForDay(d.iso)"
                      :key="seg.workPeriodId + (seg.segmentSuffix ?? '') + (seg.isBreak ? '-b' : '-w')"
                      class="wc-block-wrap"
                      :class="{
                        break: seg.isBreak,
                        work: !seg.isBreak,
                        click: rowCorrection && !seg.isBreak,
                      }"
                      :style="segBlockStyle(seg)"
                      @click="
                        !seg.isBreak && rowCorrection ? onWorkBlockClick(d.iso, seg.workPeriodId) : undefined
                      "
                    >
                      <div class="wc-block">
                        <span class="wc-time-row wc-time-start">{{ formatGermanTime(seg.effectiveIn) }}</span>
                        <span class="wc-block-kind">{{ seg.isBreak ? 'Pause' : 'Arbeit' }}</span>
                        <span class="wc-time-row wc-time-end">{{ blockEndTimeLabel(seg) }}</span>
                      </div>
                    </div>
                  </template>
                </template>
              </div>
            </div>
          </div>

          <div v-if="nowLineStyle" class="wc-now-line" :style="nowLineStyle" />
        </div>
      </div>
    </div>

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

<style scoped>
.muted {
  color: #64748b;
  font-size: 0.9rem;
}
.wc-root {
  position: relative;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #f8fafc;
  /* kein overflow:hidden — sonst funktioniert position:sticky im Feiertags-Banner nicht */
}
.wc-loading {
  position: absolute;
  inset: 0;
  z-index: 6;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(248, 250, 252, 0.85);
  border-radius: 8px;
}
.wc-shell.dim {
  opacity: 0.55;
  pointer-events: none;
}
.wc-legend {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 1.25rem 2rem;
  padding: 0.4rem 0.75rem;
  font-size: 0.72rem;
  font-weight: 600;
  color: #475569;
  background: #fff;
  border-bottom: 1px solid #e2e8f0;
}
.wc-legend-item {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}
.wc-legend-planned::before {
  content: '';
  width: 12px;
  height: 12px;
  border-radius: 3px;
  background: #bfdbfe;
  border: 1px solid #2563eb;
}
.wc-legend-booked::before {
  content: '';
  width: 12px;
  height: 12px;
  border-radius: 3px;
  background: linear-gradient(135deg, #60a5fa 0%, #2563eb 100%);
  border: 1px solid #1e40af;
}
.wc-head {
  display: grid;
  grid-template-columns: 52px repeat(5, 1fr);
  background: #fff;
  border-bottom: 1px solid #e2e8f0;
  border-radius: 8px 8px 0 0;
}
.wc-corner {
  border-right: 1px solid #e2e8f0;
}
.wc-day-head {
  padding: 0.5rem 0.35rem;
  text-align: center;
  border-left: 1px solid #e2e8f0;
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}
.wc-day-head.today:not(.col-holiday) {
  background: #eff6ff;
  box-shadow: inset 0 -2px 0 #3b82f6;
}
.wc-day-head.today.col-holiday {
  box-shadow: inset 0 -2px 0 #047857;
}
.wc-day-head.col-holiday {
  background: linear-gradient(180deg, #a7f3d0 0%, #6ee7b7 100%);
  border-bottom: 2px solid #059669;
}
.wc-day-head.col-holiday .wc-dow {
  color: #065f46;
}
.wc-day-head.col-holiday .wc-dlabel {
  color: #064e3b;
  font-weight: 700;
}
.wc-dow {
  font-size: 0.72rem;
  font-weight: 600;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.wc-dlabel {
  font-size: 0.88rem;
  font-weight: 600;
  color: #0f172a;
}
.wc-scroll {
  max-height: min(70vh, 720px);
  overflow-y: auto;
  overflow-x: auto;
  background: #f1f5f9;
  border-radius: 0 0 8px 8px;
}
.wc-scroll-inner {
  position: relative;
  display: flex;
  align-items: stretch;
  min-width: 520px;
}
.wc-scroll-inner.wc-scroll-wide {
  min-width: 640px;
}
.wc-gutter {
  width: 52px;
  flex-shrink: 0;
  background: #f8fafc;
  border-right: 1px solid #e2e8f0;
  z-index: 2;
}
.wc-gutter-banner {
  flex-shrink: 0;
  border-bottom: 1px solid #e2e8f0;
}
.wc-gutter-hour {
  font-size: 0.7rem;
  color: #94a3b8;
  padding-right: 4px;
  text-align: right;
  box-sizing: border-box;
  border-bottom: 1px solid #cbd5e1;
}
.wc-days {
  flex: 1;
  display: flex;
  min-width: 0;
}
.wc-day-col {
  flex: 1;
  min-width: 0;
  background: #fff;
  border-left: 1px solid #e2e8f0;
  display: flex;
  flex-direction: column;
}
.wc-day-col.col-vacation .wc-grid-wrap {
  background: #f8fafc linear-gradient(180deg, rgba(238, 242, 255, 0.65) 0%, #fff 18%);
}
.wc-day-col.col-sick .wc-grid-wrap {
  background: #fffbeb;
}
.wc-day-col.col-other .wc-grid-wrap {
  background: #f9fafb;
}
.wc-day-col.col-compensation .wc-grid-wrap {
  background: #f0f9ff;
}
.wc-day-col.col-holiday {
  background: #ecfdf5;
  box-shadow: inset 2px 0 0 #059669;
}
.wc-day-col.col-holiday .wc-grid-wrap {
  background: linear-gradient(180deg, #d1fae5 0%, #ecfdf5 35%, #f0fdf4 100%);
}
.wc-day-col.col-holiday .wc-banner:not(.empty) {
  position: sticky;
  top: 0;
  z-index: 4;
  background: #059669;
  color: #fff;
  border-bottom: 2px solid #047857;
  font-weight: 700;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.14);
}
.wc-day-col.col-holiday .wc-banner.empty {
  background: #a7f3d0;
  border-bottom: 2px solid #059669;
}
.wc-banner {
  min-height: 26px;
  box-sizing: border-box;
  padding: 0.2rem 0.35rem;
  font-size: 0.68rem;
  font-weight: 600;
  color: #334155;
  border-bottom: 1px solid #e2e8f0;
  background: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
  line-height: 1.2;
}
.wc-banner.empty {
  color: transparent;
}
.wc-grid-wrap {
  position: relative;
  flex: 1;
  overflow: visible;
}
.wc-grid-wrap-dual .wc-dual-tracks {
  position: absolute;
  inset: 0;
  z-index: 2;
  display: flex;
  flex-direction: row;
  align-items: stretch;
}
.wc-grid-wrap-dual .wc-track {
  flex: 1;
  position: relative;
  min-width: 0;
  height: 100%;
}
.wc-grid-wrap-dual .wc-track-booked .wc-time-row {
  font-size: 10px;
}
.wc-grid-wrap-dual .wc-track-booked .wc-block-kind {
  font-size: 9px;
}
.wc-grid-bg {
  position: absolute;
  inset: 0;
  z-index: 0;
  pointer-events: none;
  background-image: linear-gradient(to bottom, #e2e8f0 1px, transparent 1px),
    linear-gradient(to bottom, #f1f5f9 1px, transparent 1px);
  background-size: 100% 44px, 100% 22px;
  background-position: top, top;
}
.wc-shift {
  position: absolute;
  left: 2px;
  right: 2px;
  z-index: 1;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  align-items: center;
  padding: 4px 3px 5px;
  gap: 2px;
  background: #bfdbfe;
  border: 1px solid #2563eb;
  border-radius: 4px;
  pointer-events: none;
}
/** Schicht-Start/Ende links, Teamsitzung nutzt die rechte Spurhälfte (volle Zeit-Höhe). */
.wc-shift.wc-shift--with-meeting-overlap {
  align-items: flex-start;
  text-align: left;
  padding-right: calc(50% + 4px);
}
.wc-shift.wc-shift--with-meeting-overlap .wc-shift-time {
  max-width: 100%;
  text-align: left;
}
.wc-shift.wc-shift--with-meeting-overlap .wc-shift-time:last-child {
  margin-top: auto;
}
.wc-meeting {
  position: absolute;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 4px 3px 5px;
  background: linear-gradient(180deg, #e9d5ff 0%, #d8b4fe 100%);
  border: 1px solid #7e22ce;
  border-radius: 4px;
  pointer-events: none;
  overflow: hidden;
}
.wc-meeting.wc-meeting--overlap-shift {
  align-items: flex-start;
  justify-content: flex-start;
  padding-top: 3px;
}
.wc-meeting-label {
  font-size: 10px;
  font-weight: 700;
  color: #581c87;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}
.wc-shift-time {
  flex: 0 0 auto;
  font-size: 11px;
  font-weight: 800;
  line-height: 1.15;
  color: #0f172a;
  letter-spacing: 0.02em;
  white-space: nowrap;
}
.wc-block-wrap {
  position: absolute;
  left: 4px;
  right: 4px;
  z-index: 3;
  pointer-events: none;
}
.wc-block-wrap.click {
  pointer-events: auto;
  cursor: pointer;
}
.wc-block {
  position: absolute;
  inset: 0;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  align-items: center;
  border-radius: 5px;
  border-style: solid;
  border-width: 1px;
  border-color: #1e40af;
  border-left-width: 4px;
  border-left-color: #1e3a8a;
  background: linear-gradient(135deg, #60a5fa 0%, #2563eb 100%);
  box-shadow: 0 2px 8px rgba(30, 64, 175, 0.35);
  overflow: visible;
  padding: 3px 5px 4px;
  gap: 1px;
}
.wc-time-row {
  flex: 0 0 auto;
  font-size: 11px;
  font-weight: 800;
  line-height: 1.15;
  letter-spacing: 0.02em;
  text-align: center;
  white-space: nowrap;
  width: 100%;
  opacity: 1;
  -webkit-text-fill-color: currentColor;
}
.wc-block-wrap.work .wc-time-row {
  color: #f8fafc;
  text-shadow: 0 1px 2px rgba(15, 23, 42, 0.45);
}
.wc-block-wrap.work .wc-block-kind {
  flex: 1 1 auto;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 0;
  font-size: 10px;
  font-weight: 700;
  color: #f8fafc;
  text-shadow: 0 1px 2px rgba(15, 23, 42, 0.45);
  opacity: 1;
  -webkit-text-fill-color: #f8fafc;
}
.wc-block-wrap.break .wc-block {
  border-style: solid;
  border-width: 1px;
  border-color: #0f766e;
  border-left-width: 4px;
  border-left-color: #134e4a;
  background: linear-gradient(135deg, #ccfbf1 0%, #99f6e4 100%);
  box-shadow: 0 2px 6px rgba(13, 148, 136, 0.2);
  opacity: 1;
}
.wc-block-wrap.break .wc-time-row {
  color: #042f2e;
  text-shadow: 0 0 1px rgba(255, 255, 255, 0.9);
}
.wc-block-wrap.break .wc-block-kind {
  flex: 1 1 auto;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 0;
  font-size: 10px;
  font-weight: 700;
  color: #115e59;
  opacity: 1;
  -webkit-text-fill-color: #115e59;
}
.wc-block-wrap.work.click:hover .wc-block {
  box-shadow: 0 3px 12px rgba(30, 64, 175, 0.45);
  border-color: #1e3a8a;
  filter: brightness(1.05);
}
.wc-block-wrap.break.click:hover .wc-block {
  box-shadow: 0 3px 10px rgba(13, 148, 136, 0.32);
  border-color: #0f766e;
  filter: brightness(1.02);
}
.wc-now-line {
  position: absolute;
  left: 52px;
  right: 0;
  height: 2px;
  margin-top: -1px;
  background: #ef4444;
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.8);
  pointer-events: none;
  z-index: 5;
}
</style>
