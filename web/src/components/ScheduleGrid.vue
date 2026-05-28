<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import InputText from 'primevue/inputtext'
import { useToast } from 'primevue/usetoast'

import {
  createSchedule,
  deleteSchedule,
  fetchFixedNonWorkWeekdays,
  fetchSchedulesForWeek,
  updateSchedule,
} from '@/api/management'
import type { Absence, Employee, FixedNonWorkWeekdays, Holiday, Schedule, TeamMeeting } from '@/types/api'
import { addDays, formatGermanDate, mondayOfISOWeek, shiftISOWeek, toISODateLocal } from '@/utils/dates'
import {
  horizontalBarPercentages,
  normalizeShiftClock,
  SCHEDULE_TIMELINE_END_H,
  SCHEDULE_TIMELINE_START_H,
  shiftTimeRangeLabel,
  timelineHourLabels,
} from '@/utils/scheduleShiftLayout'
import { teamMeetingBarLabel } from '@/utils/teamMeetingLabel'
import { isFixedNonWorkDayISO } from '@/utils/workdays'
import {
  clampMoveShift,
  clampResizeEnd,
  clampResizeStart,
  clientXToTimelineMinutes,
  minutesToHHMM,
  placeThreeHourShiftMinutes,
  readShiftIntervalMinutes,
} from '@/utils/scheduleTimelineInteract'

const props = defineProps<{
  /** Flaches Raster (falls keine sections übergeben werden). */
  employees?: Employee[]
  /** Gruppierte Abschnitte; hat Vorrang — Reihenfolge = Reihenfolge im Raster. */
  sections?: { title: string; employees: Employee[] }[]
  weekYear: number
  week: number
}>()

const weekNotes = defineModel<string>('weekNotes', { default: '' })

const emit = defineEmits<{
  autosaveHint: [msg: string]
  editTeamMeeting: [meetingId: number]
}>()

const displaySections = computed(() => {
  if (props.sections && props.sections.length > 0) return props.sections
  return [{ title: '', employees: props.employees ?? [] }]
})

/** Alle Mitarbeitenden für Daten/Kanten (flach). */
const allEmployees = computed(() => displaySections.value.flatMap((s) => s.employees))

const toast = useToast()
/** Nur für „Laden…“ / Tabellen-Unmount (nicht bei stillem Reload nach Speichern). */
const overlayLoading = ref(false)
/** Während GET/apply/Purge — verhindert Autosave-Timer mitten im Server-Sync. */
const applyingServerSchedule = ref(false)
/** Läuft persistCells + stilles Reload — blockiert parallele Autosave-Läufe. */
const persistWeekInFlight = ref(false)
/** Für Toolbar (Excel usw.): auch während stillem Sync blockieren. */
const scheduleBlocking = computed(
  () => overlayLoading.value || applyingServerSchedule.value || persistWeekInFlight.value,
)
const gridWrapRef = ref<HTMLElement | null>(null)
const tableRef = ref<HTMLTableElement | null>(null)
/** Höhe des thead (px), für sticky Gruppenzeilen unter der Kalenderwoche. */
const theadHeightPx = ref(52)
/** Gemessene Höhe der ersten Gruppenzeile (px), für Tagesansicht: sticky Stundenleiste ohne Lücke. */
const sectionHeadHeightPx = ref(0)
let theadResizeObserver: ResizeObserver | null = null
let windowResizeForGrid: (() => void) | null = null
const stickyHeaderVisible = ref(false)
const stickyHeaderLeft = ref(0)
const stickyHeaderTop = ref(0)
const stickyHeaderWidth = ref(0)
const stickyHeaderHeight = ref(0)
/** Abstand unterhalb einer sticky/fixed App-Topbar (px), damit Dienstplan-Köpfe nicht darunter rutschen. */
const layoutTopInset = ref(0)
const headerCellWidths = ref<number[]>([])

function colStyle(idx: number) {
  const w = headerCellWidths.value[idx]
  if (w == null || w <= 0) return {}
  return { width: `${w}px`, minWidth: `${w}px` }
}
function measureTheadHeight() {
  /** Nur Haupt-Raster (ref), nicht das optische Sticky-Duplikat (.sticky-header-table). */
  const table = tableRef.value
  if (!table) return
  const thead = table.querySelector('thead')
  if (thead) {
    theadHeightPx.value = Math.ceil(thead.getBoundingClientRect().height)
    stickyHeaderHeight.value = theadHeightPx.value
    const ths = Array.from(thead.querySelectorAll('th'))
    headerCellWidths.value = ths.map((th) => Math.ceil(th.getBoundingClientRect().width))
  } else {
    headerCellWidths.value = []
  }
  const sectionCell = table.querySelector('tr.section-head .section-head-cell')
  sectionHeadHeightPx.value = sectionCell
    ? Math.round(sectionCell.getBoundingClientRect().height)
    : 0
}
function attachTheadResizeObserver() {
  theadResizeObserver?.disconnect()
  theadResizeObserver = null
  const table = tableRef.value
  if (!table || typeof ResizeObserver === 'undefined') return
  const thead = table.querySelector('thead')
  const sectionCell = table.querySelector('tr.section-head .section-head-cell')
  if (!thead && !sectionCell) return
  theadResizeObserver = new ResizeObserver(() => measureTheadHeight())
  if (thead) theadResizeObserver.observe(thead)
  if (sectionCell) theadResizeObserver.observe(sectionCell)
}
function layoutInsetFromAppChrome(): number {
  if (typeof document === 'undefined') return 0
  const bar = document.querySelector('header.topbar')
  if (!bar) return 0
  const st = getComputedStyle(bar)
  if (st.position !== 'sticky' && st.position !== 'fixed') return 0
  const r = bar.getBoundingClientRect()
  if (r.bottom <= 0) return 0
  return Math.round(r.height)
}

function updateStickyHeader() {
  const wrap = gridWrapRef.value
  if (!wrap) {
    stickyHeaderVisible.value = false
    return
  }
  const rect = wrap.getBoundingClientRect()
  const visible = rect.top <= 0 && rect.bottom > theadHeightPx.value
  stickyHeaderVisible.value = visible
  if (!visible) return
  stickyHeaderLeft.value = rect.left - wrap.scrollLeft
  stickyHeaderWidth.value = tableRef.value?.scrollWidth ?? wrap.scrollWidth
  stickyHeaderTop.value = layoutTopInset.value
}
function updateStickyOffsets() {
  layoutTopInset.value = layoutInsetFromAppChrome()
  updateStickyHeader()
}
const copyBusy = ref(false)
const autosaveHint = ref('')
let autosaveHintTimer: ReturnType<typeof setTimeout> | null = null

function setAutosaveHint(message: string, clearAfterMs?: number) {
  if (autosaveHintTimer) {
    clearTimeout(autosaveHintTimer)
    autosaveHintTimer = null
  }
  autosaveHint.value = message
  emit('autosaveHint', message)
  if (clearAfterMs != null && clearAfterMs > 0) {
    autosaveHintTimer = setTimeout(() => {
      autosaveHintTimer = null
      autosaveHint.value = ''
      emit('autosaveHint', '')
    }, clearAfterMs)
  }
}

/** Debounced Auto-Save für den aktuellen ISO‑Kalender */
const SCHEDULE_AUTOSAVE_MS = 1400
let scheduleAutosaveTimer: ReturnType<typeof setTimeout> | null = null

interface Cell {
  shift_start: string
  shift_end: string
  id?: number
}

const cells = ref<Record<string, Cell>>({})
/** Rohdaten von GET /schedules (Abschnitt absences); Anzeige ableiten wir per computed (auch wenn Mitarbeiterliste später geladen wird). */
const lastWeekAbsences = ref<Absence[]>([])
const fnwByUser = ref<Map<number, FixedNonWorkWeekdays[]>>(new Map())
/** Gesetzliche Feiertage Mo–Fr dieser KW (week_holidays). */
const lastWeekHolidays = ref<Holiday[]>([])
const teamMeetings = ref<TeamMeeting[]>([])
let lastPersistedFingerprint = ''

function dateKeyYMD(s: string) {
  return s.length >= 10 ? s.slice(0, 10) : s
}

function meetingsForEmployeeOnDate(uid: number, date: string): TeamMeeting[] {
  const d = dateKeyYMD(date)
  return teamMeetings.value.filter(
    (m) => dateKeyYMD(m.meeting_date) === d && Array.isArray(m.user_ids) && m.user_ids.includes(uid),
  )
}

function meetingClockKey(t: string): string {
  return normalizeShiftClock(t).trim()
}

function sortedMeetingsForEmployeeOnDate(uid: number, date: string): TeamMeeting[] {
  return [...meetingsForEmployeeOnDate(uid, date)].sort((a, b) =>
    meetingClockKey(a.time_start).localeCompare(meetingClockKey(b.time_start)),
  )
}

function dayMeetingOverlapsShift(uid: number, date: string, m: TeamMeeting): boolean {
  const mPct = horizontalBarPercentages(
    date,
    m.time_start,
    m.time_end,
    SCHEDULE_TIMELINE_START_H,
    SCHEDULE_TIMELINE_END_H,
  )
  if (!mPct) return false
  const c = cells.value[cellKey(uid, date)]
  const sPct = horizontalBarPercentages(
    date,
    c?.shift_start ?? '',
    c?.shift_end ?? '',
    SCHEDULE_TIMELINE_START_H,
    SCHEDULE_TIMELINE_END_H,
  )
  if (!sPct) return false
  const m0 = mPct.leftPct
  const m1 = mPct.leftPct + mPct.widthPct
  const s0 = sPct.leftPct
  const s1 = sPct.leftPct + sPct.widthPct
  return m0 < s1 - 0.02 && m1 > s0 + 0.02
}

function dayMeetingBarStyle(_uid: number, date: string, m: TeamMeeting): Record<string, string> | undefined {
  const mPct = horizontalBarPercentages(
    date,
    m.time_start,
    m.time_end,
    SCHEDULE_TIMELINE_START_H,
    SCHEDULE_TIMELINE_END_H,
  )
  if (!mPct) return undefined
  const widthPct = Math.max(mPct.widthPct, 6)
  return {
    left: `${mPct.leftPct}%`,
    width: `${widthPct}%`,
  }
}

function dayMeetingBarLabel(m: TeamMeeting): string {
  return teamMeetingBarLabel(m)
}

const weekdays = ['Mo', 'Di', 'Mi', 'Do', 'Fr']

/** `null` = Wochenraster; 0–4 = fokussierter Wochentag (Timeline-Ansicht). */
const expandedDayIndex = ref<number | null>(null)

const timelineHours = timelineHourLabels(SCHEDULE_TIMELINE_START_H, SCHEDULE_TIMELINE_END_H)
const timelineHourCount = timelineHours.length

const dates = computed(() => {
  const mon = mondayOfISOWeek(props.weekYear, props.week)
  return [0, 1, 2, 3, 4].map((i) => toISODateLocal(addDays(mon, i)))
})

const selectedDate = computed(() => {
  const i = expandedDayIndex.value
  if (i == null || i < 0 || i >= dates.value.length) return ''
  return dates.value[i] ?? ''
})

const selectedWeekdayLabel = computed(() => {
  const i = expandedDayIndex.value
  if (i == null) return ''
  return weekdays[i] ?? ''
})

function toggleDayExpand(dayIndex: number) {
  expandedDayIndex.value = expandedDayIndex.value === dayIndex ? null : dayIndex
}

const dayNavCanPrev = computed(
  () => expandedDayIndex.value != null && expandedDayIndex.value > 0,
)
const dayNavCanNext = computed(
  () =>
    expandedDayIndex.value != null && expandedDayIndex.value < dates.value.length - 1,
)

function goAdjacentExpandedDay(delta: -1 | 1) {
  const i = expandedDayIndex.value
  if (i == null) return
  const next = i + delta
  if (next < 0 || next >= dates.value.length) return
  expandedDayIndex.value = next
}

function dayShiftBarStyle(uid: number, date: string): Record<string, string> | undefined {
  const c = cells.value[cellKey(uid, date)]
  if (!c) return undefined
  const pct = horizontalBarPercentages(
    date,
    c.shift_start,
    c.shift_end,
    SCHEDULE_TIMELINE_START_H,
    SCHEDULE_TIMELINE_END_H,
  )
  if (!pct) return undefined
  return {
    left: `${pct.leftPct}%`,
    width: `${pct.widthPct}%`,
  }
}

function dayShiftBarLabel(uid: number, date: string): string | null {
  const c = cells.value[cellKey(uid, date)]
  if (!c) return null
  return shiftTimeRangeLabel(c.shift_start, c.shift_end)
}

function applyShiftMinutes(uid: number, date: string, startMin: number, endMin: number) {
  const k = cellKey(uid, date)
  if (!cells.value[k]) cells.value[k] = { shift_start: '', shift_end: '' }
  const c = cells.value[k]
  c.shift_start = normalizeShiftClock(minutesToHHMM(startMin))
  c.shift_end = normalizeShiftClock(minutesToHHMM(endMin))
}

function onEmptyTimelineClick(ev: MouseEvent, uid: number, date: string) {
  if (Date.now() < blockEmptyTimelineClickUntil) return
  if (overlayLoading.value || applyingServerSchedule.value || persistWeekInFlight.value) return
  const el = ev.target as HTMLElement | null
  if (el?.closest?.('.day-meeting-bar')) return
  if (el?.closest?.('.day-shift-bar-outer')) return
  const k = cellKey(uid, date)
  const c = cells.value[k]
  if (!c) return
  if (!isBlankTime(c.shift_start) || !isBlankTime(c.shift_end)) return
  const wrap = el?.closest?.('.day-timeline-track-wrap') as HTMLElement | null
  if (!wrap) return
  const rect = wrap.getBoundingClientRect()
  const mins = clientXToTimelineMinutes(rect, ev.clientX, SCHEDULE_TIMELINE_START_H, SCHEDULE_TIMELINE_END_H)
  const placed = placeThreeHourShiftMinutes(mins, SCHEDULE_TIMELINE_START_H, SCHEDULE_TIMELINE_END_H)
  if (!placed) return
  applyShiftMinutes(uid, date, placed.startMin, placed.endMin)
}

function onDayMeetingBarClick(_ev: MouseEvent, tm: TeamMeeting) {
  if (overlayLoading.value || applyingServerSchedule.value || persistWeekInFlight.value) return
  emit('editTeamMeeting', tm.id)
}

type TimelineBarDragKind = 'move' | 'resize-l' | 'resize-r'

interface TimelineBarDrag {
  trackEl: HTMLElement
  uid: number
  date: string
  kind: TimelineBarDragKind
  origStartMin: number
  origEndMin: number
  anchorClientX: number
  pointerId: number
}

const timelineBarDragRef = ref<TimelineBarDrag | null>(null)
/** Nach Balken-Drag kurz leere Spur-Klicks unterdrücken (vermeidet Ghost-„Klick“). */
let blockEmptyTimelineClickUntil = 0

function detachTimelineBarDragListeners() {
  window.removeEventListener('pointermove', onTimelineBarPointerMove, true)
  window.removeEventListener('pointerup', onTimelineBarPointerUp, true)
  window.removeEventListener('pointercancel', onTimelineBarPointerUp, true)
}

function onTimelineBarPointerMove(ev: PointerEvent) {
  const d = timelineBarDragRef.value
  if (!d || ev.pointerId !== d.pointerId) return
  ev.preventDefault()
  const rect = d.trackEl.getBoundingClientRect()
  if (d.kind === 'move') {
    const { startMin, endMin } = clampMoveShift(
      d.origStartMin,
      d.origEndMin,
      ev.clientX,
      d.anchorClientX,
      rect,
      SCHEDULE_TIMELINE_START_H,
      SCHEDULE_TIMELINE_END_H,
    )
    applyShiftMinutes(d.uid, d.date, startMin, endMin)
  } else if (d.kind === 'resize-l') {
    const ns = clampResizeStart(d.origEndMin, ev.clientX, rect)
    applyShiftMinutes(d.uid, d.date, ns, d.origEndMin)
  } else {
    const ne = clampResizeEnd(d.origStartMin, ev.clientX, rect)
    applyShiftMinutes(d.uid, d.date, d.origStartMin, ne)
  }
}

function onTimelineBarPointerUp(ev: PointerEvent) {
  const d = timelineBarDragRef.value
  if (!d || ev.pointerId !== d.pointerId) return
  timelineBarDragRef.value = null
  detachTimelineBarDragListeners()
  blockEmptyTimelineClickUntil = Date.now() + 350
}

function onShiftBarPointerDown(ev: PointerEvent, uid: number, date: string, kind: TimelineBarDragKind) {
  if (overlayLoading.value || applyingServerSchedule.value || persistWeekInFlight.value) return
  const wrap = (ev.currentTarget as HTMLElement | null)?.closest?.('.day-timeline-track-wrap') as
    | HTMLElement
    | undefined
    | null
  if (!wrap) return
  const c = cells.value[cellKey(uid, date)]
  const inv = readShiftIntervalMinutes(
    c?.shift_start ?? '',
    c?.shift_end ?? '',
    SCHEDULE_TIMELINE_START_H,
    SCHEDULE_TIMELINE_END_H,
  )
  if (!inv) return
  ev.preventDefault()
  ev.stopPropagation()
  timelineBarDragRef.value = {
    trackEl: wrap,
    uid,
    date,
    kind,
    origStartMin: inv.startMin,
    origEndMin: inv.endMin,
    anchorClientX: ev.clientX,
    pointerId: ev.pointerId,
  }
  window.addEventListener('pointermove', onTimelineBarPointerMove, true)
  window.addEventListener('pointerup', onTimelineBarPointerUp, true)
  window.addEventListener('pointercancel', onTimelineBarPointerUp, true)
}

const dayTableCssVars = computed(() => {
  if (expandedDayIndex.value == null) return {}
  const h = sectionHeadHeightPx.value
  return {
    '--schedule-section-head-h': h > 0 ? `${h}px` : '2.45rem',
  } as Record<string, string>
})

function cellKey(uid: number, date: string) {
  return `${uid}_${date}`
}

function isBlankTime(s: string | undefined): boolean {
  return !String(s ?? '').trim()
}

function datesForIsoWeek(isoYear: number, isoWeek: number): string[] {
  const mon = mondayOfISOWeek(isoYear, isoWeek)
  return [0, 1, 2, 3, 4].map((i) => toISODateLocal(addDays(mon, i)))
}

function cellsFingerprint(c: Record<string, Cell>): string {
  const keys = Object.keys(c).sort()
  return keys.map((k) => `${k}:${c[k]?.shift_start ?? ''}\t${c[k]?.shift_end ?? ''}\t${c[k]?.id ?? ''}`).join('|')
}

function absenceCellLabel(a: Absence): string {
  if (a.absence_type === 'vacation') return a.half_day ? 'Urlaub (½)' : 'Urlaub'
  if (a.absence_type === 'compensation_day') return 'Ausgleichstag'
  return ''
}

/** API kann DATE als „YYYY-MM-DD“ oder mit Zeit-/Zeitzonen-Suffix liefern — Raster nutzt reines Kalenderdatum. */
function normalizeCalendarDay(s: string): string {
  const m = String(s).trim().match(/^(\d{4}-\d{2}-\d{2})/)
  return m ? m[1]! : String(s).trim()
}

function holidayDateSetFromHolidays(list: Holiday[]): Set<string> {
  return new Set(list.map((h) => normalizeCalendarDay(h.holiday_date)))
}

function isPublicHolidayDate(date: string): boolean {
  const day = normalizeCalendarDay(date)
  return lastWeekHolidays.value.some((h) => normalizeCalendarDay(h.holiday_date) === day)
}

function publicHolidayName(date: string): string {
  const day = normalizeCalendarDay(date)
  const h = lastWeekHolidays.value.find((x) => normalizeCalendarDay(x.holiday_date) === day)
  return h?.name ?? ''
}


/** Fester freier Wochentag (Mo–Fr) laut Stammdaten — keine reguläre Schichtplanung. */
function isFixedNonWorkDay(userId: number, date: string): boolean {
  const rows = fnwByUser.value.get(userId)
  return isFixedNonWorkDayISO(normalizeCalendarDay(date), rows)
}

async function loadFnwForEmployees() {
  const ids = [...new Set(allEmployees.value.map((e) => e.id))]
  const next = new Map<number, FixedNonWorkWeekdays[]>()
  await Promise.all(
    ids.map(async (uid) => {
      try {
        next.set(uid, await fetchFixedNonWorkWeekdays(uid))
      } catch {
        next.set(uid, [])
      }
    }),
  )
  fnwByUser.value = next
}

function scheduleCellDisabled(userId: number, date: string): boolean {
  return isPublicHolidayDate(date) || scheduleBlockedByFullDayAbsence(userId, date) || isFixedNonWorkDay(userId, date)
}

/** Ganztägiger Urlaub oder Ausgleichstag — keine Dienstplanzeiten (Halbtag-Urlaub weiterhin editierbar). */
function scheduleBlockedByFullDayAbsence(userId: number, date: string): boolean {
  const day = normalizeCalendarDay(date)
  for (const a of lastWeekAbsences.value) {
    if (a.user_id !== userId) continue
    if (normalizeCalendarDay(a.absence_date) !== day) continue
    if (a.absence_type === 'compensation_day') return true
    if (a.absence_type === 'vacation' && !a.half_day) return true
    return false
  }
  return false
}

const absenceHints = computed(() => {
  const next: Record<string, string> = {}
  const daySet = new Set(dates.value.map((d) => normalizeCalendarDay(d)))
  for (const a of lastWeekAbsences.value) {
    if (a.absence_type !== 'vacation' && a.absence_type !== 'compensation_day') continue
    const day = normalizeCalendarDay(a.absence_date)
    if (!daySet.has(day)) continue
    const label = absenceCellLabel(a)
    if (!label) continue
    next[cellKey(a.user_id, day)] = label
  }
  return next
})

function isPartialShiftCell(c: Cell | undefined): boolean {
  if (!c) return false
  const a = String(c.shift_start ?? '').trim()
  const b = String(c.shift_end ?? '').trim()
  return (Boolean(a) && !b) || (!a && Boolean(b))
}

function applySchedules(list: Schedule[]) {
  const prev = cells.value
  const next: Record<string, Cell> = {}
  for (const e of allEmployees.value) {
    for (const d of dates.value) {
      next[cellKey(e.id, d)] = { shift_start: '', shift_end: '' }
    }
  }
  for (const s of list) {
    const k = cellKey(s.user_id, normalizeCalendarDay(s.schedule_date))
    if (next[k]) {
      next[k] = {
        shift_start: s.shift_start,
        shift_end: s.shift_end,
        id: s.id,
      }
    }
  }
  for (const k of Object.keys(next)) {
    const p = prev[k]
    if (!p || !isPartialShiftCell(p)) continue
    const n = next[k]!
    const serverSlotEmpty =
      !String(n.shift_start ?? '').trim() && !String(n.shift_end ?? '').trim()
    if (serverSlotEmpty) {
      next[k] = {
        shift_start: p.shift_start,
        shift_end: p.shift_end,
        id: p.id,
      }
    }
  }
  cells.value = next
}

function clearScheduleAutosaveTimer() {
  if (scheduleAutosaveTimer) {
    clearTimeout(scheduleAutosaveTimer)
    scheduleAutosaveTimer = null
  }
}

function scheduleAutosaveSoon() {
  clearScheduleAutosaveTimer()
  scheduleAutosaveTimer = setTimeout(() => {
    scheduleAutosaveTimer = null
    void persistCurrentWeekAndReload()
  }, SCHEDULE_AUTOSAVE_MS)
}

/** Speichert die Zellen für genau eine ISO‑Woche (Kalenderjahr + KW). */
async function persistCellsForIsoWeek(
  isoYear: number,
  isoWeek: number,
  publicHolidayDates: Set<string>,
) {
  const weekDates = datesForIsoWeek(isoYear, isoWeek)
  for (const e of allEmployees.value) {
    for (const d of weekDates) {
      const nd = normalizeCalendarDay(d)
      if (publicHolidayDates.has(nd)) continue
      if (isFixedNonWorkDay(e.id, d)) {
        const k = cellKey(e.id, d)
        if (!cells.value[k]) cells.value[k] = { shift_start: '', shift_end: '' }
        const cx = cells.value[k]
        if (cx.id) {
          try {
            await deleteSchedule(cx.id)
          } catch {
            /* Sync beim nächsten Load */
          }
          cx.id = undefined
        }
        cx.shift_start = ''
        cx.shift_end = ''
        continue
      }
      if (scheduleBlockedByFullDayAbsence(e.id, d)) {
        const k = cellKey(e.id, d)
        if (!cells.value[k]) cells.value[k] = { shift_start: '', shift_end: '' }
        const cx = cells.value[k]
        if (cx.id) {
          try {
            await deleteSchedule(cx.id)
          } catch {
            /* Sync beim nächsten Load */
          }
          cx.id = undefined
        }
        cx.shift_start = ''
        cx.shift_end = ''
        continue
      }
      const k = cellKey(e.id, d)
      if (!cells.value[k]) cells.value[k] = { shift_start: '', shift_end: '' }
      const c = cells.value[k]
      const has = c.shift_start.trim() && c.shift_end.trim()
      if (c.id) {
        if (!has) {
          await deleteSchedule(c.id)
          c.id = undefined
          c.shift_start = ''
          c.shift_end = ''
        } else {
          const upd = await updateSchedule(c.id, {
            shift_start: c.shift_start.trim(),
            shift_end: c.shift_end.trim(),
          })
          c.shift_start = upd.shift_start
          c.shift_end = upd.shift_end
        }
      } else if (has) {
        const created = await createSchedule({
          user_id: e.id,
          schedule_date: d,
          shift_start: c.shift_start.trim(),
          shift_end: c.shift_end.trim(),
        })
        c.id = created.id
        c.shift_start = created.shift_start
        c.shift_end = created.shift_end
      }
    }
  }
  lastPersistedFingerprint = cellsFingerprint(cells.value)
}

/** Entfernt gespeicherte Schichten an gesetzlichen Feiertagen (Mo–Fr), sobald die KW geladen ist. */
async function purgeSchedulesOnPublicHolidays() {
  for (const d of dates.value) {
    if (!isPublicHolidayDate(d)) continue
    for (const e of allEmployees.value) {
      const k = cellKey(e.id, d)
      const c = cells.value[k]
      if (!c) continue
      if (c.id) {
        try {
          await deleteSchedule(c.id)
        } catch {
          /* nächster Load/sync */
        }
      }
      cells.value[k] = { shift_start: '', shift_end: '' }
    }
  }
}

/** Entfernt gespeicherte Schichten an Tagen mit Urlaub (ganz) oder Ausgleichstag. */
async function purgeSchedulesOnBlockingAbsences() {
  for (const d of dates.value) {
    for (const e of allEmployees.value) {
      if (!scheduleBlockedByFullDayAbsence(e.id, d)) continue
      const k = cellKey(e.id, d)
      const c = cells.value[k]
      if (!c) continue
      if (c.id) {
        try {
          await deleteSchedule(c.id)
        } catch {
          /* nächster Load/sync */
        }
      }
      cells.value[k] = { shift_start: '', shift_end: '' }
    }
  }
}

async function purgeSchedulesOnFixedNonWorkdays() {
  for (const d of dates.value) {
    for (const e of allEmployees.value) {
      if (!isFixedNonWorkDay(e.id, d)) continue
      const k = cellKey(e.id, d)
      const c = cells.value[k]
      if (!c) continue
      if (c.id) {
        try {
          await deleteSchedule(c.id)
        } catch {
          /* nächster Load/sync */
        }
      }
      cells.value[k] = { shift_start: '', shift_end: '' }
    }
  }
}

async function persistCurrentWeekAndReload() {
  if (overlayLoading.value || applyingServerSchedule.value || persistWeekInFlight.value) return
  if (cellsFingerprint(cells.value) === lastPersistedFingerprint) return
  persistWeekInFlight.value = true
  setAutosaveHint('Dienstplan wird gespeichert …')
  try {
    await persistCellsForIsoWeek(
      props.weekYear,
      props.week,
      holidayDateSetFromHolidays(lastWeekHolidays.value),
    )
    await load({ silent: true })
    setAutosaveHint('Gespeichert', 2200)
  } catch {
    setAutosaveHint('')
    toast.add({ severity: 'error', summary: 'Dienstplan', detail: 'Automatisches Speichern fehlgeschlagen.', life: 10000 })
  } finally {
    persistWeekInFlight.value = false
  }
}

async function load(opts?: { silent?: boolean }) {
  const silent = opts?.silent === true
  if (!silent) overlayLoading.value = true
  applyingServerSchedule.value = true
  try {
    await loadFnwForEmployees()
    const { schedules: list, weekNotes: notes, absences, weekHolidays, teamMeetings: tml } =
      await fetchSchedulesForWeek(props.weekYear, props.week)
    lastWeekHolidays.value = Array.isArray(weekHolidays) ? weekHolidays : []
    teamMeetings.value = Array.isArray(tml) ? tml : []
    applySchedules(Array.isArray(list) ? list : [])
    lastWeekAbsences.value = Array.isArray(absences) ? absences : []
    weekNotes.value = notes
    await purgeSchedulesOnPublicHolidays()
    await purgeSchedulesOnBlockingAbsences()
    await purgeSchedulesOnFixedNonWorkdays()
    lastPersistedFingerprint = cellsFingerprint(cells.value)
  } catch {
    if (!opts?.silent) {
      toast.add({ severity: 'error', summary: 'Dienstplan', detail: 'Konnte nicht geladen werden.', life: 10000 })
    }
    applySchedules([])
    lastWeekAbsences.value = []
    lastWeekHolidays.value = []
    teamMeetings.value = []
    weekNotes.value = ''
    lastPersistedFingerprint = cellsFingerprint(cells.value)
  } finally {
    applyingServerSchedule.value = false
    if (!silent) overlayLoading.value = false
  }
}

watch(
  () =>
    [
      props.weekYear,
      props.week,
      [...new Set(allEmployees.value.map((e) => e.id))]
        .sort((a, b) => a - b)
        .join(','),
    ] as const,
  async (nw, ow) => {
    if (ow && (nw[0] !== ow[0] || nw[1] !== ow[1])) {
      expandedDayIndex.value = null
    }
    clearScheduleAutosaveTimer()
    if (ow) {
      const [oy, oweek] = [ow[0], ow[1]]
      try {
        if (cellsFingerprint(cells.value) !== lastPersistedFingerprint) {
          setAutosaveHint('Dienstplan wird gespeichert …')
          await persistCellsForIsoWeek(oy, oweek, holidayDateSetFromHolidays(lastWeekHolidays.value))
          setAutosaveHint('')
        }
      } catch {
        setAutosaveHint('')
        toast.add({
          severity: 'error',
          summary: 'Dienstplan',
          detail: 'Änderungen vor dem Wechsel der Kalenderwoche konnten nicht gespeichert werden.',
      life: 10000,
        })
      }
    }
    await load()
  },
  { immediate: true },
)

watch(
  [
    () => props.weekYear,
    () => props.week,
    overlayLoading,
    applyingServerSchedule,
    () => displaySections.value,
    expandedDayIndex,
  ],
  async () => {
    await nextTick()
    measureTheadHeight()
    attachTheadResizeObserver()
    updateStickyOffsets()
  },
)

watch(
  cells,
  () => {
    if (overlayLoading.value || applyingServerSchedule.value || persistWeekInFlight.value) return
    if (cellsFingerprint(cells.value) === lastPersistedFingerprint) return
    scheduleAutosaveSoon()
  },
  { deep: true },
)

onMounted(() => {
  nextTick(() => {
    measureTheadHeight()
    attachTheadResizeObserver()
    windowResizeForGrid = () => {
      measureTheadHeight()
      updateStickyOffsets()
    }
    window.addEventListener('resize', windowResizeForGrid)
    window.addEventListener('scroll', updateStickyOffsets, { passive: true })
    updateStickyOffsets()
  })
})

onUnmounted(() => {
  detachTimelineBarDragListeners()
  timelineBarDragRef.value = null
  theadResizeObserver?.disconnect()
  theadResizeObserver = null
  if (windowResizeForGrid) {
    window.removeEventListener('resize', windowResizeForGrid)
    windowResizeForGrid = null
  }
  window.removeEventListener('scroll', updateStickyOffsets)
  clearScheduleAutosaveTimer()
  if (autosaveHintTimer) {
    clearTimeout(autosaveHintTimer)
    autosaveHintTimer = null
  }
})

async function copyPreviousWeek() {
  const { year: py, week: pw } = shiftISOWeek(props.weekYear, props.week, -1)
  copyBusy.value = true
  try {
    const { schedules: prev } = await fetchSchedulesForWeek(py, pw)
    const mon = mondayOfISOWeek(props.weekYear, props.week)
    const prevMon = mondayOfISOWeek(py, pw)
    const dayDiff = Math.round((mon.getTime() - prevMon.getTime()) / 86400000)
    const holidayDatesCurrent = holidayDateSetFromHolidays(lastWeekHolidays.value)
    for (const s of prev) {
      const oldD = new Date(s.schedule_date + 'T12:00:00')
      const newD = addDays(oldD, dayDiff)
      const newDate = toISODateLocal(newD)
      if (!dates.value.includes(newDate)) continue
      const nd = normalizeCalendarDay(newDate)
      if (holidayDatesCurrent.has(nd)) continue
      if (scheduleBlockedByFullDayAbsence(s.user_id, newDate)) continue
      const k = cellKey(s.user_id, newDate)
      if (!cells.value[k]) cells.value[k] = { shift_start: '', shift_end: '' }
      const c = cells.value[k]
      c.shift_start = s.shift_start
      c.shift_end = s.shift_end
      c.id = undefined
    }
    toast.add({
      severity: 'info',
      summary: 'Vorwoche übernommen',
      detail: 'Wird automatisch gespeichert …',
      life: 10000,
    })
    await persistCurrentWeekAndReload()
  } catch {
    toast.add({ severity: 'error', summary: 'Kopieren fehlgeschlagen', life: 10000 })
  } finally {
    copyBusy.value = false
  }
}

defineExpose({
  copyPreviousWeek,
  /** Lädt Dienstplan und Wochennotizen vom Server neu (z. B. nach Excel-Import). */
  reloadFromServer: () => load({ silent: true }),
  /** Aktuelle Teamsitzungen der geladenen KW (für Editor-Dialog). */
  listTeamMeetings: () => teamMeetings.value.slice(),
  /** 0–4 = fokussierter Werktag in der Tagesansicht; null = Wochenraster. */
  expandedDayIndex: () => expandedDayIndex.value,
  loading: scheduleBlocking,
  copyBusy,
})
</script>

<template>
  <div
    ref="gridWrapRef"
    class="grid-wrap"
    :style="{
      '--schedule-thead-h': theadHeightPx + 'px',
      '--layout-top-inset': layoutTopInset + 'px',
    }"
    @scroll="updateStickyOffsets"
  >
    <div
      v-if="stickyHeaderVisible && headerCellWidths.length && expandedDayIndex === null"
      class="sticky-header"
      :style="{
        top: stickyHeaderTop + 'px',
        left: stickyHeaderLeft + 'px',
        width: stickyHeaderWidth + 'px',
      }"
    >
      <!-- Gleiche Tabellen-Markup/Klassen wie der echte thead → identisches Aussehen -->
      <table class="grid sticky-header-table">
        <thead>
          <tr>
            <th class="name" :style="colStyle(0)">Mitarbeiter</th>
            <th
              v-for="(d, i) in dates"
              :key="`sticky-th-${d}`"
              class="day day--expandable"
              :class="{ 'day--holiday': publicHolidayName(d) }"
              :style="colStyle(i + 1)"
              role="button"
              tabindex="-1"
              :aria-label="`Tagesansicht ${weekdays[i]} ${formatGermanDate(d)}`"
              @click.prevent="toggleDayExpand(i)"
            >
              {{ weekdays[i] }}<span class="sub">{{ formatGermanDate(d) }}</span>
              <span v-if="publicHolidayName(d)" class="holiday-line">Feiertag · {{ publicHolidayName(d) }}</span>
            </th>
          </tr>
        </thead>
      </table>
    </div>
    <div
      v-if="stickyHeaderVisible && headerCellWidths.length && expandedDayIndex !== null"
      class="sticky-header"
      :style="{
        top: stickyHeaderTop + 'px',
        left: stickyHeaderLeft + 'px',
        width: stickyHeaderWidth + 'px',
      }"
    >
      <table class="grid sticky-header-table grid--day">
        <thead>
          <tr>
            <th class="name" :style="colStyle(0)">Mitarbeiter</th>
            <th
              class="day day--expandable day--timeline-head"
              :class="{ 'day--holiday': publicHolidayName(selectedDate) }"
              :style="colStyle(1)"
              role="button"
              tabindex="-1"
              aria-pressed="true"
              :aria-label="`Wochenansicht anzeigen (${selectedWeekdayLabel} ${formatGermanDate(selectedDate)})`"
              @click.prevent="expandedDayIndex !== null && toggleDayExpand(expandedDayIndex)"
              @keydown.left.prevent="dayNavCanPrev && goAdjacentExpandedDay(-1)"
              @keydown.right.prevent="dayNavCanNext && goAdjacentExpandedDay(1)"
            >
              <div class="day-timeline-head-row">
                <button
                  type="button"
                  class="day-timeline-nav-btn"
                  :disabled="!dayNavCanPrev"
                  aria-label="Vorheriger Wochentag"
                  tabindex="-1"
                  @click.stop.prevent="goAdjacentExpandedDay(-1)"
                >
                  <i class="pi pi-chevron-left" aria-hidden="true" />
                </button>
                <span class="day-timeline-head-center">
                  {{ selectedWeekdayLabel }}<span class="sub">{{ formatGermanDate(selectedDate) }}</span>
                  <span v-if="publicHolidayName(selectedDate)" class="holiday-line">
                    Feiertag · {{ publicHolidayName(selectedDate) }}
                  </span>
                  <span class="day-collapse-hint">Klick für Wochenansicht</span>
                </span>
                <button
                  type="button"
                  class="day-timeline-nav-btn"
                  :disabled="!dayNavCanNext"
                  aria-label="Nächster Wochentag"
                  tabindex="-1"
                  @click.stop.prevent="goAdjacentExpandedDay(1)"
                >
                  <i class="pi pi-chevron-right" aria-hidden="true" />
                </button>
              </div>
            </th>
          </tr>
        </thead>
      </table>
    </div>
    <div v-if="overlayLoading" class="muted">Laden…</div>
    <table v-else-if="expandedDayIndex === null" ref="tableRef" class="grid" data-testid="schedule-grid">
      <thead>
        <tr>
          <th class="name">Mitarbeiter</th>
          <th
            v-for="(d, i) in dates"
            :key="d"
            class="day day--expandable"
            :class="{ 'day--holiday': publicHolidayName(d) }"
            role="button"
            tabindex="0"
            :aria-label="`Tagesansicht ${weekdays[i]} ${formatGermanDate(d)}`"
            @click.prevent="toggleDayExpand(i)"
            @keydown.enter.prevent="toggleDayExpand(i)"
            @keydown.space.prevent="toggleDayExpand(i)"
          >
            {{ weekdays[i] }}<span class="sub">{{ formatGermanDate(d) }}</span>
            <span v-if="publicHolidayName(d)" class="holiday-line">Feiertag · {{ publicHolidayName(d) }}</span>
          </th>
        </tr>
      </thead>
      <tbody v-for="(sec, si) in displaySections" :key="'sec-' + si">
        <tr v-if="sec.title" class="section-head">
          <td class="section-head-cell" :colspan="dates.length + 1">{{ sec.title }}</td>
        </tr>
        <tr v-for="emp in sec.employees" :key="emp.id">
          <td class="name">{{ emp.display_name }}</td>
          <td
            v-for="d in dates"
            :key="cellKey(emp.id, d)"
            class="cell"
            :class="{
              'cell--holiday': isPublicHolidayDate(d),
              'cell--absence-block': scheduleBlockedByFullDayAbsence(emp.id, d),
              'cell--fixed-free': isFixedNonWorkDay(emp.id, d),
            }"
          >
            <div class="pair">
              <div
                class="time-slot"
                :class="{
                  'time-slot--empty':
                    isBlankTime(cells[cellKey(emp.id, d)]?.shift_start) &&
                    !isPublicHolidayDate(d) &&
                    !scheduleBlockedByFullDayAbsence(emp.id, d) &&
                    !isFixedNonWorkDay(emp.id, d),
                }"
              >
                <InputText
                  v-model="cells[cellKey(emp.id, d)].shift_start"
                  class="t"
                  autocomplete="off"
                  :aria-label="`Schichtbeginn ${emp.display_name} ${formatGermanDate(d)}`"
                  :disabled="scheduleCellDisabled(emp.id, d)"
                />
              </div>
              <div
                class="time-slot"
                :class="{
                  'time-slot--empty':
                    isBlankTime(cells[cellKey(emp.id, d)]?.shift_end) &&
                    !isPublicHolidayDate(d) &&
                    !scheduleBlockedByFullDayAbsence(emp.id, d) &&
                    !isFixedNonWorkDay(emp.id, d),
                }"
              >
                <InputText
                  v-model="cells[cellKey(emp.id, d)].shift_end"
                  class="t"
                  autocomplete="off"
                  :aria-label="`Schichtende ${emp.display_name} ${formatGermanDate(d)}`"
                  :disabled="scheduleCellDisabled(emp.id, d)"
                />
              </div>
            </div>
            <div
              v-if="isFixedNonWorkDay(emp.id, d) && !absenceHints[cellKey(emp.id, d)]"
              class="abs-hint abs-hint--fixed-free"
            >
              Fix frei
            </div>
            <div
              v-if="absenceHints[cellKey(emp.id, d)]"
              class="abs-hint"
              :class="{
                'abs-hint--vacation': absenceHints[cellKey(emp.id, d)]!.startsWith('Urlaub'),
                'abs-hint--compensation': absenceHints[cellKey(emp.id, d)] === 'Ausgleichstag',
              }"
            >
              {{ absenceHints[cellKey(emp.id, d)] }}
            </div>
          </td>
        </tr>
      </tbody>
    </table>
    <table v-else ref="tableRef" class="grid grid--day" :style="dayTableCssVars" data-testid="schedule-grid">
      <thead>
        <tr>
          <th class="name">Mitarbeiter</th>
          <th
            class="day day--expandable day--timeline-head"
            :class="{ 'day--holiday': publicHolidayName(selectedDate) }"
            role="button"
            tabindex="0"
            aria-pressed="true"
            :aria-label="`Wochenansicht anzeigen (${selectedWeekdayLabel} ${formatGermanDate(selectedDate)})`"
            @click.prevent="expandedDayIndex !== null && toggleDayExpand(expandedDayIndex)"
            @keydown.enter.prevent="expandedDayIndex !== null && toggleDayExpand(expandedDayIndex)"
            @keydown.space.prevent="expandedDayIndex !== null && toggleDayExpand(expandedDayIndex)"
            @keydown.left.prevent="dayNavCanPrev && goAdjacentExpandedDay(-1)"
            @keydown.right.prevent="dayNavCanNext && goAdjacentExpandedDay(1)"
          >
            <div class="day-timeline-head-row">
              <button
                type="button"
                class="day-timeline-nav-btn"
                :disabled="!dayNavCanPrev"
                aria-label="Vorheriger Wochentag"
                @click.stop.prevent="goAdjacentExpandedDay(-1)"
              >
                <i class="pi pi-chevron-left" aria-hidden="true" />
              </button>
              <span class="day-timeline-head-center">
                {{ selectedWeekdayLabel }}<span class="sub">{{ formatGermanDate(selectedDate) }}</span>
                <span v-if="publicHolidayName(selectedDate)" class="holiday-line">
                  Feiertag · {{ publicHolidayName(selectedDate) }}
                </span>
                <span class="day-collapse-hint">Klick für Wochenansicht</span>
              </span>
              <button
                type="button"
                class="day-timeline-nav-btn"
                :disabled="!dayNavCanNext"
                aria-label="Nächster Wochentag"
                @click.stop.prevent="goAdjacentExpandedDay(1)"
              >
                <i class="pi pi-chevron-right" aria-hidden="true" />
              </button>
            </div>
          </th>
        </tr>
      </thead>
      <tbody v-for="(sec, si) in displaySections" :key="'day-sec-' + si">
        <tr v-if="sec.title" class="section-head">
          <td class="section-head-cell" colspan="2">{{ sec.title }}</td>
        </tr>
        <tr
          v-if="!isPublicHolidayDate(selectedDate) && (Boolean(sec.title) || si === 0)"
          class="day-hour-ruler-row"
          :class="
            sec.title ? 'day-hour-ruler-row--after-section' : 'day-hour-ruler-row--after-thead-only'
          "
        >
          <td class="name day-hour-ruler-name" aria-hidden="true" />
          <td class="cell day-hour-ruler-cell">
            <div class="day-timeline-hours" :style="{ '--day-hours': timelineHourCount }">
              <span v-for="h in timelineHours" :key="h" class="day-timeline-hour">
                {{ String(h).padStart(2, '0') }}:00
              </span>
            </div>
          </td>
        </tr>
        <tr v-for="emp in sec.employees" :key="'day-emp-' + emp.id">
          <td class="name">{{ emp.display_name }}</td>
          <td
            class="cell cell--timeline"
            :class="{
              'cell--holiday': isPublicHolidayDate(selectedDate),
              'cell--absence-block': scheduleBlockedByFullDayAbsence(emp.id, selectedDate),
              'cell--fixed-free': isFixedNonWorkDay(emp.id, selectedDate),
            }"
          >
            <template v-if="isPublicHolidayDate(selectedDate)">
              <div class="day-timeline-blocked day-timeline-blocked--holiday">
                Feiertag · {{ publicHolidayName(selectedDate) }}
              </div>
            </template>
            <template v-else-if="scheduleBlockedByFullDayAbsence(emp.id, selectedDate)">
              <div class="day-timeline-blocked">
                {{ absenceHints[cellKey(emp.id, selectedDate)] || '—' }}
              </div>
            </template>
            <template v-else-if="isFixedNonWorkDay(emp.id, selectedDate)">
              <div class="day-timeline-blocked day-timeline-blocked--fixed-free">Fix frei</div>
            </template>
            <template v-else>
              <div class="day-timeline day-timeline--track-only">
                <div
                  class="day-timeline-track-wrap"
                  @click="onEmptyTimelineClick($event, emp.id, selectedDate)"
                >
                  <div class="day-timeline-grid-bg" :style="{ '--day-hours': timelineHourCount }" />
                  <div
                    v-if="dayShiftBarStyle(emp.id, selectedDate)"
                    class="day-shift-bar-outer"
                    :style="dayShiftBarStyle(emp.id, selectedDate)"
                    @click.stop
                  >
                    <div
                      class="day-shift-bar-inner"
                      role="group"
                      :aria-label="`Schicht ${dayShiftBarLabel(emp.id, selectedDate) ?? ''}`"
                    >
                      <div
                        class="day-shift-handle day-shift-handle--left"
                        aria-label="Schichtbeginn ziehen"
                        @pointerdown.prevent.stop="onShiftBarPointerDown($event, emp.id, selectedDate, 'resize-l')"
                      />
                      <div
                        class="day-shift-body"
                        aria-label="Schicht verschieben"
                        @pointerdown.prevent.stop="onShiftBarPointerDown($event, emp.id, selectedDate, 'move')"
                      />
                      <div
                        class="day-shift-handle day-shift-handle--right"
                        aria-label="Schichtende ziehen"
                        @pointerdown.prevent.stop="onShiftBarPointerDown($event, emp.id, selectedDate, 'resize-r')"
                      />
                    </div>
                    <span class="day-shift-bar-label">{{ dayShiftBarLabel(emp.id, selectedDate) }}</span>
                  </div>
                  <div
                    v-for="(tm, mix) in sortedMeetingsForEmployeeOnDate(emp.id, selectedDate)"
                    :key="'tm-' + tm.id"
                    class="day-meeting-bar"
                    :class="{ 'day-meeting-bar--overlaps-shift': dayMeetingOverlapsShift(emp.id, selectedDate, tm) }"
                    :style="{
                      ...(dayMeetingBarStyle(emp.id, selectedDate, tm) || {}),
                      zIndex: String(2 + mix),
                    }"
                    @click.stop="onDayMeetingBarClick($event, tm)"
                  >
                    <span class="day-meeting-bar-label">{{ dayMeetingBarLabel(tm) }}</span>
                  </div>
                </div>
              </div>
              <div
                v-if="absenceHints[cellKey(emp.id, selectedDate)]"
                class="abs-hint abs-hint--below-timeline"
                :class="{
                  'abs-hint--vacation': absenceHints[cellKey(emp.id, selectedDate)]!.startsWith('Urlaub'),
                  'abs-hint--compensation': absenceHints[cellKey(emp.id, selectedDate)] === 'Ausgleichstag',
                }"
              >
                {{ absenceHints[cellKey(emp.id, selectedDate)] }}
              </div>
            </template>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
/* Kein overflow-x auf diesem Wrapper: sonst wird ein Scroll-Container erzeugt und
   position:sticky auf Tabellen-Zellen (Gruppenzeilen) funktioniert in Chromium nicht mehr.
   Breitere Tabellen erzeugen bei Bedarf eine horizontale Seitenleiste am Viewport. */
.grid-wrap {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  min-width: 0;
  width: 100%;
  overflow-x: visible;
  overflow-y: visible;
  position: relative;
}
.grid {
  /* sticky funktioniert in Tabellen zuverlässiger ohne collapse */
  border-collapse: separate;
  border-spacing: 0;
  font-size: 0.85rem;
  min-width: 640px;
  width: 100%;
}
.grid th,
.grid td {
  border-right: 1px solid #e2e8f0;
  border-bottom: 1px solid #e2e8f0;
  padding: 0.35rem;
  vertical-align: middle;
}
.grid thead tr:first-child th {
  border-top: 1px solid #e2e8f0;
}
.grid th.name,
.grid td.name {
  border-left: 1px solid #e2e8f0;
}
.grid thead th {
  position: sticky;
  top: var(--layout-top-inset, 0px);
  z-index: 5;
  box-shadow: 0 1px 0 #e2e8f0;
}
.grid th {
  background: #f8fafc;
}
.section-head .section-head-cell {
  position: sticky;
  top: calc(var(--layout-top-inset, 0px) + var(--schedule-thead-h, 3.25rem));
  z-index: 4;
  background: #e2e8f0;
  color: #0f172a;
  font-weight: 700;
  font-size: 0.8rem;
  letter-spacing: 0.02em;
  padding: 0.45rem 0.5rem;
  border-left: 1px solid #e2e8f0;
  border-right: 1px solid #e2e8f0;
  border-bottom: 2px solid #cbd5e1;
  box-shadow: 0 1px 0 rgba(203, 213, 225, 0.9);
}
.sticky-header {
  position: fixed;
  z-index: 7;
  pointer-events: none;
  overflow: visible;
}
/* Gleiche .grid-Styles wie das Haupt-Raster; sticky im Overlay abschalten */
.sticky-header-table {
  margin: 0;
  table-layout: fixed;
}
.sticky-header-table thead th {
  position: static;
  top: auto;
  z-index: auto;
}
.name {
  text-align: left;
  white-space: nowrap;
  min-width: 140px;
}
.day {
  text-align: center;
}
.day--holiday {
  background: linear-gradient(180deg, #ffedd5 0%, #fed7aa 100%);
  color: #9a3412;
  font-weight: 700;
}
.day--holiday .sub {
  color: #c2410c;
  font-weight: 600;
}
.holiday-line {
  display: inline-block;
  margin-top: 0.35rem;
  padding: 0.2rem 0.5rem;
  border-radius: 6px;
  font-weight: 700;
  font-size: 0.68rem;
  line-height: 1.2;
  letter-spacing: 0.02em;
  color: #fff;
  background: #ea580c;
  box-shadow: 0 1px 2px rgba(154, 52, 18, 0.25);
}
.sub {
  display: block;
  font-weight: 400;
  font-size: 0.7rem;
  color: #64748b;
}
.cell--holiday {
  background: linear-gradient(180deg, #fffbeb 0%, #fef3c7 100%);
}
.cell--holiday :deep(.p-inputtext:disabled) {
  border-color: #fdba74 !important;
  background: #fde68a !important;
  color: #57534e !important;
  opacity: 1 !important;
}
.cell--absence-block {
  background: linear-gradient(180deg, #eef2ff 0%, #e0e7ff 100%);
}
.cell--absence-block :deep(.p-inputtext:disabled) {
  border-color: #a5b4fc !important;
  background: #e0e7ff !important;
  color: #4338ca !important;
  opacity: 1 !important;
}
.cell--fixed-free {
  background: linear-gradient(180deg, #f8fafc 0%, #e2e8f0 100%);
}
.cell--fixed-free :deep(.p-inputtext:disabled) {
  border-color: #cbd5e1 !important;
  background: #f1f5f9 !important;
  color: #475569 !important;
  opacity: 1 !important;
}
.abs-hint--fixed-free {
  color: #475569;
  font-weight: 600;
}
.pair {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
}
.time-slot {
  display: block;
}
.time-slot--empty :deep(.p-inputtext) {
  border-style: dashed;
  border-color: #cbd5e1;
  background: #f8fafc;
  color: #64748b;
}
.time-slot:not(.time-slot--empty) :deep(.p-inputtext) {
  border-style: solid;
  border-color: #e2e8f0;
  background: #fff;
  color: #0f172a;
}
.t {
  width: 100%;
  min-width: 4rem;
}
.muted {
  color: #64748b;
}
.abs-hint {
  margin-top: 0.35rem;
  font-size: 0.72rem;
  font-weight: 600;
  line-height: 1.25;
  text-align: center;
}
.abs-hint--vacation {
  color: #4338ca;
}
.abs-hint--compensation {
  color: #0369a1;
}
.grid--day {
  min-width: 520px;
}
.grid--day .day-hour-ruler-row td {
  padding-top: 0;
  padding-bottom: 0.2rem;
  vertical-align: bottom;
}
.grid--day .day-hour-ruler-name {
  background: #fff;
}
.grid--day .day-hour-ruler-cell {
  position: sticky;
  z-index: 4;
  background: #fff;
  box-shadow: 0 1px 0 #e2e8f0;
}
.grid--day .day-hour-ruler-row--after-thead-only .day-hour-ruler-cell {
  top: calc(var(--layout-top-inset, 0px) + var(--schedule-thead-h, 3.25rem));
}
.grid--day .day-hour-ruler-row--after-section .day-hour-ruler-cell {
  top: calc(
    var(--layout-top-inset, 0px) + var(--schedule-thead-h, 3.25rem) + var(--schedule-section-head-h, 2.45rem)
  );
}
.grid--day .day-timeline-hours {
  margin-bottom: 0;
}
.day--expandable {
  cursor: pointer;
  user-select: none;
}
.day--expandable:focus-visible {
  outline: 2px solid #2563eb;
  outline-offset: 2px;
}
.day--timeline-head {
  text-align: center;
}
.day-timeline-head-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.2rem;
}
.sticky-header .day-timeline-head-row {
  pointer-events: auto;
}
.day-timeline-head-center {
  flex: 1;
  min-width: 0;
  text-align: center;
}
.day-timeline-nav-btn {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.65rem;
  height: 1.65rem;
  margin: 0;
  padding: 0;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  background: #fff;
  color: #334155;
  cursor: pointer;
  line-height: 1;
  font-size: 0.75rem;
}
.day-timeline-nav-btn:hover:not(:disabled) {
  background: #f1f5f9;
  border-color: #94a3b8;
  color: #0f172a;
}
.day-timeline-nav-btn:focus-visible {
  outline: 2px solid #2563eb;
  outline-offset: 1px;
}
.day-timeline-nav-btn:disabled {
  opacity: 0.35;
  cursor: not-allowed;
}
.day--holiday .day-timeline-nav-btn {
  border-color: #fdba74;
  background: #fff7ed;
  color: #9a3412;
}
.day--holiday .day-timeline-nav-btn:hover:not(:disabled) {
  background: #ffedd5;
}
@media (max-width: 768px) {
  .day-timeline-nav-btn {
    width: 2.75rem;
    height: 2.75rem;
    font-size: 1.05rem;
    border-radius: 8px;
  }
}
.day-collapse-hint {
  display: block;
  margin-top: 0.35rem;
  font-size: 0.65rem;
  font-weight: 500;
  color: #64748b;
}
.cell--timeline {
  vertical-align: middle;
  min-width: 0;
}
.day-timeline-blocked {
  padding: 0.65rem 0.5rem;
  text-align: center;
  font-size: 0.78rem;
  font-weight: 600;
  color: #4338ca;
  border-radius: 6px;
  background: #eef2ff;
}
.day-timeline-blocked--holiday {
  color: #9a3412;
  background: linear-gradient(180deg, #fffbeb 0%, #fef3c7 100%);
}
.day-timeline-blocked--fixed-free {
  color: #475569;
  background: linear-gradient(180deg, #f8fafc 0%, #e2e8f0 100%);
}
.day-timeline {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  min-width: 0;
}
.day-timeline--track-only {
  gap: 0;
  padding-top: 0.1rem;
}
.day-timeline-hours {
  display: grid;
  grid-template-columns: repeat(var(--day-hours, 14), minmax(0, 1fr));
  font-size: 0.65rem;
  font-weight: 600;
  color: #64748b;
  line-height: 1.2;
  user-select: none;
}
.day-timeline-hour {
  text-align: left;
}
.day-timeline-track-wrap {
  position: relative;
  height: 46px;
  min-width: 0;
  cursor: default;
  user-select: none;
}
.day-timeline-grid-bg {
  position: absolute;
  inset: 0;
  border-radius: 6px;
  border: 1px solid #cbd5e1;
  --dh: var(--day-hours, 14);
  /*
   * Volle Stunden: dickere Linie am Periodenanfang (keine Überlappung mit Halbstunden).
   * Halbe Stunden: nur in der Mitte jeder Stunden-Periode — vermeidet Doppel-Layer-Artefakte.
   * Erstes Bild = oben (Stundenlinien).
   */
  --hour-cell: calc(100% / var(--dh));
  background-image:
    repeating-linear-gradient(
      to right,
      rgba(148, 163, 184, 0.38) 0,
      rgba(148, 163, 184, 0.38) 2px,
      transparent 2px,
      transparent var(--hour-cell)
    ),
    repeating-linear-gradient(
      to right,
      transparent 0,
      transparent calc(var(--hour-cell) / 2 - 1px),
      rgba(148, 163, 184, 0.22) calc(var(--hour-cell) / 2 - 1px),
      rgba(148, 163, 184, 0.22) calc(var(--hour-cell) / 2 + 1px),
      transparent calc(var(--hour-cell) / 2 + 1px),
      transparent var(--hour-cell)
    );
  background-color: #f8fafc;
  pointer-events: none;
  /* Subpixel-Rundung: weniger „gebrochene“ vertikale Linien beim Skalieren */
  transform: translateZ(0);
}
.day-shift-bar-outer {
  position: absolute;
  top: 5px;
  bottom: 5px;
  z-index: 1;
  box-sizing: border-box;
  min-width: 3px;
  display: flex;
  flex-direction: column;
  align-items: stretch;
  justify-content: stretch;
}
.day-shift-bar-inner {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: row;
  align-items: stretch;
  border-radius: 5px;
  overflow: hidden;
  box-sizing: border-box;
  background: linear-gradient(180deg, #bfdbfe 0%, #93c5fd 100%);
  border: 1px solid #2563eb;
  box-shadow: 0 1px 2px rgba(37, 99, 235, 0.2);
}
.day-shift-handle {
  flex: 0 0 10px;
  min-width: 6px;
  cursor: ew-resize;
  touch-action: none;
  background: rgba(37, 99, 235, 0.12);
}
.day-shift-handle:hover {
  background: rgba(37, 99, 235, 0.22);
}
.day-shift-body {
  flex: 1;
  min-width: 0;
  cursor: grab;
  touch-action: none;
}
.day-shift-body:active {
  cursor: grabbing;
}
.day-shift-bar-label {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: none;
  font-size: 0.62rem;
  font-weight: 700;
  color: #1e3a8a;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
  padding: 0 0.35rem;
}
.day-meeting-bar {
  position: absolute;
  top: 5px;
  bottom: 5px;
  box-sizing: border-box;
  min-width: 3px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  padding: 0 0.35rem;
  background: linear-gradient(180deg, #e9d5ff 0%, #d8b4fe 100%);
  border: 1px solid #7e22ce;
  border-radius: 5px;
  box-shadow: 0 1px 2px rgba(126, 34, 206, 0.22);
}
/** Editor-Tagesansicht: bei Überlappung halbe Höhe unten, kompaktes Label unten im Balken. */
.day-meeting-bar.day-meeting-bar--overlaps-shift {
  top: auto;
  bottom: 5px;
  height: calc((100% - 10px) / 2);
  align-items: flex-end;
  justify-content: center;
  padding-top: 2px;
  padding-bottom: 3px;
}
.day-meeting-bar.day-meeting-bar--overlaps-shift .day-meeting-bar-label {
  font-size: 0.52rem;
  font-weight: 700;
  line-height: 1.05;
}
.day-meeting-bar-label {
  font-size: 0.62rem;
  font-weight: 700;
  color: #581c87;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}
.abs-hint--below-timeline {
  margin-top: 0.4rem;
  text-align: left;
}
</style>
