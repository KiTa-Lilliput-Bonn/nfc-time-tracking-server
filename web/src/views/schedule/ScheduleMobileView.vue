<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Menu from 'primevue/menu'
import Select from 'primevue/select'
import { useToast } from 'primevue/usetoast'

import { fetchEmployees } from '@/api/employees'
import { fetchGroups } from '@/api/groups'
import {
  createSchedule,
  deleteSchedule,
  fetchSchedulePlanning,
  fetchSchedulesForWeek,
  updateSchedule,
} from '@/api/management'
import ShiftEditSheet, { type ShiftSheetDay } from '@/components/schedule/ShiftEditSheet.vue'
import type { Absence, Employee, Holiday, Schedule, SchedulePlanning, TeamMeeting, UserGroup } from '@/types/api'
import { getApiErrorMessage } from '@/utils/apiError'
import { addDays, formatGermanDate, isoWeekAndYear, mondayOfISOWeek, shiftISOWeek, toISODateLocal } from '@/utils/dates'
import { clearRouteQueryKeys, queryPositiveInt } from '@/utils/leitungDeepLink'
import {
  horizontalBarPercentages,
  SCHEDULE_TIMELINE_END_H,
  SCHEDULE_TIMELINE_START_H,
} from '@/utils/scheduleShiftLayout'
import {
  absenceLabel,
  clockToMinutes,
  formatHoursMinutes,
  formatHoursShort,
  frequentShifts,
  plannableDayTargetMinutes,
  shiftNetMinutes,
  shortShiftLabel,
} from '@/utils/schedulePlanning'
import { teamMeetingBarTag } from '@/utils/teamMeetingLabel'

const emit = defineEmits<{ openTable: [] }>()

const toast = useToast()
const route = useRoute()
const router = useRouter()

/** Wochen für die Schnellwahl „häufig bei …“ */
const HISTORY_WEEKS = 6
const UNDO_MS = 6000
const WEEKDAYS = ['Mo', 'Di', 'Mi', 'Do', 'Fr'] as const
const WEEKDAYS_LONG = ['Montag', 'Dienstag', 'Mittwoch', 'Donnerstag', 'Freitag'] as const

const now = isoWeekAndYear(new Date())
const weekYear = ref(now.year)
const week = ref(now.week)

const employees = ref<Employee[]>([])
const groups = ref<UserGroup[]>([])
const schedules = ref<Schedule[]>([])
const holidays = ref<Holiday[]>([])
const teamMeetings = ref<TeamMeeting[]>([])
const planning = ref<SchedulePlanning | null>(null)
const prevWeekSchedules = ref<Schedule[]>([])
const history = ref<Schedule[]>([])
const loading = ref(true)
const loadError = ref('')
const saveState = ref<'idle' | 'saving' | 'saved' | 'error'>('idle')
let loadSeq = 0

type Mode = 'week' | 'day' | 'person'
const mode = ref<Mode>('week')
const dayIndex = ref(defaultDayIndex())
const personId = ref<number | null>(null)

function defaultDayIndex(): number {
  const d = new Date().getDay()
  return d >= 1 && d <= 5 ? d - 1 : 0
}

// ---------- Woche ----------

const dates = computed(() => {
  const mon = mondayOfISOWeek(weekYear.value, week.value)
  return [0, 1, 2, 3, 4].map((i) => toISODateLocal(addDays(mon, i)))
})

const weekRangeLabel = computed(() => `${formatGermanDate(dates.value[0]!)}–${formatGermanDate(dates.value[4]!)}`)
const isCurrentWeek = computed(() => weekYear.value === now.year && week.value === now.week)
const todayISO = toISODateLocal(new Date())

function goWeek(delta: number) {
  const n = shiftISOWeek(weekYear.value, week.value, delta)
  weekYear.value = n.year
  week.value = n.week
}

function goToday() {
  weekYear.value = now.year
  week.value = now.week
  dayIndex.value = defaultDayIndex()
}

// ---------- Personen und Gruppen (wie im Desktop-Editor) ----------

const planEmployees = computed(() => employees.value.filter((e) => e.active && e.role !== 'superadmin'))

const sections = computed((): { title: string; employees: Employee[] }[] => {
  const list = planEmployees.value
  const known = new Set(groups.value.map((g) => g.id))
  const out: { title: string; employees: Employee[] }[] = []
  const byName = (a: Employee, b: Employee) => a.display_name.localeCompare(b.display_name, 'de')
  for (const g of groups.value) {
    const emps = list.filter((e) => e.group_id === g.id).sort(byName)
    if (emps.length) out.push({ title: g.name, employees: emps })
  }
  const orphan = list.filter((e) => e.group_id == null || !known.has(e.group_id)).sort(byName)
  if (orphan.length) out.push({ title: groups.value.length ? 'Ohne Gruppe' : '', employees: orphan })
  return out
})

const orderedEmployees = computed(() => sections.value.flatMap((s) => s.employees))

function groupNameOf(e: Employee): string {
  return groups.value.find((g) => g.id === e.group_id)?.name ?? ''
}

/** „Anna Becker“ → „Anna B.“ */
function shortName(name: string): string {
  const parts = name.trim().split(/\s+/)
  if (parts.length < 2) return name
  return `${parts[0]} ${parts[parts.length - 1]![0]}.`
}

// ---------- Tageszustand je Zelle ----------

function day10(s: string) {
  return String(s).slice(0, 10)
}

const scheduleByCell = computed(() => {
  const m = new Map<string, Schedule>()
  for (const s of schedules.value) m.set(`${s.user_id}_${day10(s.schedule_date)}`, s)
  return m
})

const absenceByCell = computed(() => {
  const m = new Map<string, Absence>()
  for (const a of planning.value?.absences ?? []) m.set(`${a.user_id}_${day10(a.absence_date)}`, a)
  return m
})

const holidayByDate = computed(() => {
  const m = new Map<string, string>()
  for (const h of holidays.value) m.set(day10(h.holiday_date), h.name)
  return m
})

const closureByDate = computed(() => {
  const m = new Map<string, string>()
  for (const c of planning.value?.closure_days ?? []) m.set(day10(c.closure_date), c.name)
  return m
})

const planningByUser = computed(() => {
  const m = new Map<number, { hours: number; fixed: number[] }>()
  for (const u of planning.value?.users ?? []) {
    m.set(u.user_id, { hours: u.hours_per_week, fixed: u.fixed_non_work_weekdays ?? [] })
  }
  return m
})

function weekdayOf(date: string): number {
  return new Date(`${date}T12:00:00`).getDay()
}

type CellKind = 'holiday' | 'fixed' | 'blocked-absence' | 'absence' | 'shift' | 'empty'

interface CellInfo {
  kind: CellKind
  /** Text in der Wochenzelle */
  label: string
  /** Hinweis für Tagesansicht und Bearbeiten-Fenster */
  note: string
  blocked: boolean
  schedule: Schedule | null
  absence: Absence | null
}

function cellInfo(uid: number, date: string): CellInfo {
  const sch = scheduleByCell.value.get(`${uid}_${date}`) ?? null
  const abs = absenceByCell.value.get(`${uid}_${date}`) ?? null
  const hol = holidayByDate.value.get(date)
  const closure = closureByDate.value.get(date)
  const fixed = planningByUser.value.get(uid)?.fixed ?? []
  const shiftLabel = sch ? shortShiftLabel(sch.shift_start, sch.shift_end) : ''
  if (hol) return { kind: 'holiday', label: 'Feiertag', note: `Feiertag · ${hol}`, blocked: true, schedule: null, absence: abs }
  if (fixed.includes(weekdayOf(date))) {
    return { kind: 'fixed', label: 'frei', note: 'Regulär frei (fester freier Tag)', blocked: true, schedule: null, absence: abs }
  }
  if (abs && !abs.half_day && (abs.absence_type === 'vacation' || abs.absence_type === 'compensation_day')) {
    return { kind: 'blocked-absence', label: absenceLabel(abs), note: absenceLabel(abs), blocked: true, schedule: null, absence: abs }
  }
  const closureNote = closure ? `Schließtag · ${closure}` : ''
  if (abs) {
    const note = [absenceLabel(abs), closureNote].filter(Boolean).join(' · ')
    // Krank/sonstige ganztägig: Abwesenheit zeigen, eine noch geplante Schicht bleibt sichtbar zum Austragen.
    const label = abs.half_day && shiftLabel ? shiftLabel : absenceLabel(abs)
    return { kind: 'absence', label, note, blocked: false, schedule: sch, absence: abs }
  }
  if (sch && shiftLabel) return { kind: 'shift', label: shiftLabel, note: closureNote, blocked: false, schedule: sch, absence: null }
  return { kind: 'empty', label: closure ? 'zu' : '', note: closureNote, blocked: false, schedule: null, absence: null }
}

function cellNetMinutes(uid: number, date: string): number {
  const c = cellInfo(uid, date)
  // Ganztägig krank/abwesend: eine noch eingetragene Schicht findet nicht statt.
  if (c.blocked || !c.schedule || (c.absence && !c.absence.half_day)) return 0
  return shiftNetMinutes(c.schedule.shift_start, c.schedule.shift_end, planning.value?.break_rules ?? [])
}

function weekPlannedMinutes(uid: number, exceptDate?: string): number {
  return dates.value.reduce((sum, d) => (d === exceptDate ? sum : sum + cellNetMinutes(uid, d)), 0)
}

function weekTargetMinutes(uid: number): number {
  const p = planningByUser.value.get(uid)
  if (!p || p.hours <= 0) return 0
  return dates.value.reduce((sum, d) => {
    const abs = absenceByCell.value.get(`${uid}_${d}`) ?? null
    return (
      sum +
      plannableDayTargetMinutes(p.hours, {
        weekday: weekdayOf(d),
        fixedNonWorkWeekdays: p.fixed,
        closed: holidayByDate.value.has(d) || closureByDate.value.has(d),
        absence: abs,
      })
    )
  }, 0)
}

function plannedLabel(uid: number): { text: string; over: boolean; under: boolean } {
  const planned = weekPlannedMinutes(uid)
  const target = weekTargetMinutes(uid)
  if (target <= 0) return { text: planned > 0 ? `${formatHoursShort(planned)} h` : '', over: false, under: false }
  return {
    text: `${formatHoursShort(planned)} / ${formatHoursShort(target)} h`,
    over: planned - target >= 1,
    under: target - planned >= 1,
  }
}

const weekHasShifts = computed(() => schedules.value.length > 0)

// ---------- Laden ----------

async function loadWeek() {
  const seq = ++loadSeq
  loadError.value = ''
  loading.value = true
  try {
    const [wk, pl] = await Promise.all([
      fetchSchedulesForWeek(weekYear.value, week.value),
      fetchSchedulePlanning(weekYear.value, week.value),
    ])
    if (seq !== loadSeq) return
    schedules.value = wk.schedules
    holidays.value = wk.weekHolidays
    teamMeetings.value = wk.teamMeetings
    planning.value = pl
  } catch (e) {
    if (seq !== loadSeq) return
    loadError.value = (getApiErrorMessage(e) ?? 'Dienstplan konnte nicht geladen werden.')
  } finally {
    if (seq === loadSeq) loading.value = false
  }
  void loadHistory(seq)
}

/** Vorwochen für Schnellwahl und „wie letzte Woche“ (im Hintergrund, Fehler sind unkritisch). */
async function loadHistory(seq: number) {
  try {
    const weeks = Array.from({ length: HISTORY_WEEKS }, (_, i) => shiftISOWeek(weekYear.value, week.value, -(i + 1)))
    const results = await Promise.all(weeks.map((w) => fetchSchedulesForWeek(w.year, w.week)))
    if (seq !== loadSeq) return
    prevWeekSchedules.value = results[0]?.schedules ?? []
    history.value = results.flatMap((r) => r.schedules)
  } catch {
    if (seq !== loadSeq) return
    prevWeekSchedules.value = []
    history.value = []
  }
}

watch([weekYear, week], () => {
  closeSheet()
  void loadWeek()
})

onMounted(async () => {
  const y = queryPositiveInt(route.query.year)
  const w = queryPositiveInt(route.query.week)
  if (y != null && w != null && w >= 1 && w <= 53) {
    weekYear.value = y
    week.value = w
  }
  if (route.query.year != null || route.query.week != null) clearRouteQueryKeys(router, ['year', 'week'])
  try {
    const [emp, grp] = await Promise.all([fetchEmployees(), fetchGroups()])
    employees.value = emp
    groups.value = grp
  } catch (e) {
    loadError.value = (getApiErrorMessage(e) ?? 'Mitarbeitende konnten nicht geladen werden.')
  }
  await loadWeek()
})

// ---------- Speichern und Rückgängig ----------

interface UndoEntry {
  text: string
  uid: number
  date: string
  start: string
  end: string
}
const undo = ref<UndoEntry | null>(null)
let undoTimer: ReturnType<typeof setTimeout> | null = null
let savedTimer: ReturnType<typeof setTimeout> | null = null

function showUndo(entry: UndoEntry) {
  undo.value = entry
  if (undoTimer) clearTimeout(undoTimer)
  undoTimer = setTimeout(() => {
    undo.value = null
    undoTimer = null
  }, UNDO_MS)
}

function markSaved() {
  saveState.value = 'saved'
  if (savedTimer) clearTimeout(savedTimer)
  savedTimer = setTimeout(() => {
    if (saveState.value === 'saved') saveState.value = 'idle'
  }, 2500)
}

onUnmounted(() => {
  if (undoTimer) clearTimeout(undoTimer)
  if (savedTimer) clearTimeout(savedTimer)
})

/** Schreibt eine Zelle (leer = löschen) und aktualisiert die lokale Liste. */
async function writeCell(uid: number, date: string, start: string, end: string): Promise<boolean> {
  const existing = scheduleByCell.value.get(`${uid}_${date}`) ?? null
  const empty = !start.trim() || !end.trim()
  saveState.value = 'saving'
  try {
    if (empty) {
      if (existing) {
        await deleteSchedule(existing.id)
        schedules.value = schedules.value.filter((s) => s.id !== existing.id)
      }
    } else if (existing) {
      const upd = await updateSchedule(existing.id, { shift_start: start, shift_end: end })
      schedules.value = schedules.value.map((s) => (s.id === existing.id ? { ...s, ...upd } : s))
    } else {
      const created = await createSchedule({ user_id: uid, schedule_date: date, shift_start: start, shift_end: end })
      schedules.value = [...schedules.value, created]
    }
    markSaved()
    return true
  } catch (e) {
    saveState.value = 'error'
    toast.add({
      severity: 'error',
      summary: 'Dienstplan',
      detail: (getApiErrorMessage(e) ?? 'Speichern fehlgeschlagen.'),
      life: 8000,
    })
    return false
  }
}

async function changeCell(uid: number, date: string, start: string, end: string): Promise<boolean> {
  const before = scheduleByCell.value.get(`${uid}_${date}`) ?? null
  const ok = await writeCell(uid, date, start, end)
  if (!ok) return false
  const emp = employees.value.find((e) => e.id === uid)
  const who = emp ? shortName(emp.display_name) : ''
  const wd = WEEKDAYS[dates.value.indexOf(date)] ?? ''
  const text = start && end ? `${who}: ${wd} ${shortShiftLabel(start, end)} gespeichert` : `${who}: ${wd} frei`
  showUndo({ text, uid, date, start: before?.shift_start ?? '', end: before?.shift_end ?? '' })
  return true
}

async function doUndo() {
  const u = undo.value
  if (!u) return
  undo.value = null
  if (undoTimer) clearTimeout(undoTimer)
  if (await writeCell(u.uid, u.date, u.start, u.end)) {
    toast.add({ severity: 'info', summary: 'Rückgängig gemacht', life: 2500 })
  }
}

// ---------- Bearbeiten-Fenster ----------

const sheetVisible = ref(false)
const sheetUid = ref<number | null>(null)
const sheetDate = ref('')
const sheetSaving = ref(false)

function openSheet(uid: number, date: string) {
  sheetUid.value = uid
  sheetDate.value = date
  sheetVisible.value = true
}

function closeSheet() {
  sheetVisible.value = false
}

const sheetEmployee = computed(() => employees.value.find((e) => e.id === sheetUid.value) ?? null)
const sheetCell = computed(() =>
  sheetUid.value != null && sheetDate.value ? cellInfo(sheetUid.value, sheetDate.value) : null,
)
const sheetDay = computed((): ShiftSheetDay => ({
  blocked: sheetCell.value?.blocked ?? false,
  note: sheetCell.value?.note ?? '',
}))
const sheetDateLabel = computed(() => {
  const i = dates.value.indexOf(sheetDate.value)
  return i >= 0 ? `${WEEKDAYS[i]} ${formatGermanDate(sheetDate.value)}` : ''
})

const sheetQuickPicks = computed(() => {
  const uid = sheetUid.value
  if (uid == null) return []
  return frequentShifts(history.value.filter((s) => s.user_id === uid))
})

const sheetLastWeek = computed(() => {
  const uid = sheetUid.value
  if (uid == null || !sheetDate.value) return null
  const prev = toISODateLocal(addDays(new Date(`${sheetDate.value}T12:00:00`), -7))
  const s = prevWeekSchedules.value.find((x) => x.user_id === uid && day10(x.schedule_date) === prev)
  if (!s) return null
  const a = clockToMinutes(s.shift_start)
  const b = clockToMinutes(s.shift_end)
  if (a == null || b == null) return null
  return { start: s.shift_start.slice(0, 5), end: s.shift_end.slice(0, 5) }
})

const sheetLastWeekLabel = computed(() => {
  const i = dates.value.indexOf(sheetDate.value)
  return i >= 0 ? `letzten ${WEEKDAYS[i]}` : 'letzte Woche'
})

const sheetMeetings = computed(() => {
  const uid = sheetUid.value
  if (uid == null) return []
  return teamMeetings.value.filter((m) => day10(m.meeting_date) === sheetDate.value && (m.user_ids ?? []).includes(uid))
})

/** Reihenfolge der Navigation im Fenster: in Woche/Person die Tage, in der Tagesansicht die Personen. */
const sheetNavTargets = computed((): { prev: { uid: number; date: string } | null; next: { uid: number; date: string } | null } => {
  const uid = sheetUid.value
  if (uid == null) return { prev: null, next: null }
  if (mode.value === 'day') {
    const list = orderedEmployees.value
    const i = list.findIndex((e) => e.id === uid)
    return {
      prev: i > 0 ? { uid: list[i - 1]!.id, date: sheetDate.value } : null,
      next: i >= 0 && i < list.length - 1 ? { uid: list[i + 1]!.id, date: sheetDate.value } : null,
    }
  }
  const i = dates.value.indexOf(sheetDate.value)
  return {
    prev: i > 0 ? { uid, date: dates.value[i - 1]! } : null,
    next: i >= 0 && i < 4 ? { uid, date: dates.value[i + 1]! } : null,
  }
})

function navLabel(t: { uid: number; date: string } | null): string {
  if (!t) return ''
  if (mode.value === 'day') {
    const e = employees.value.find((x) => x.id === t.uid)
    return e ? shortName(e.display_name) : ''
  }
  const i = dates.value.indexOf(t.date)
  return `${WEEKDAYS[i]} ${formatGermanDate(t.date).slice(0, 6)}`
}

async function onSheetSave(start: string, end: string) {
  if (sheetUid.value == null) return
  sheetSaving.value = true
  const ok = await changeCell(sheetUid.value, sheetDate.value, start, end)
  sheetSaving.value = false
  if (ok) closeSheet()
}

async function onSheetClear() {
  if (sheetUid.value == null) return
  sheetSaving.value = true
  const ok = await changeCell(sheetUid.value, sheetDate.value, '', '')
  sheetSaving.value = false
  if (ok) closeSheet()
}

/** Weiterblättern speichert Änderungen an einer bestehenden Schicht; leere Tage werden dabei nicht angelegt. */
async function onSheetNav(dir: -1 | 1, start: string, end: string) {
  const target = dir < 0 ? sheetNavTargets.value.prev : sheetNavTargets.value.next
  if (!target || sheetUid.value == null) return
  const cur = sheetCell.value
  if (cur?.schedule && !cur.blocked) {
    const changed =
      clockToMinutes(start) !== clockToMinutes(cur.schedule.shift_start) ||
      clockToMinutes(end) !== clockToMinutes(cur.schedule.shift_end)
    if (changed && clockToMinutes(start)! < clockToMinutes(end)!) {
      sheetSaving.value = true
      const ok = await changeCell(sheetUid.value, sheetDate.value, start, end)
      sheetSaving.value = false
      if (!ok) return
    }
  }
  sheetUid.value = target.uid
  sheetDate.value = target.date
}

// ---------- Tagesansicht ----------

const selectedDate = computed(() => dates.value[dayIndex.value] ?? dates.value[0]!)
const timelineHours = computed(() => {
  const out: number[] = []
  for (let h = SCHEDULE_TIMELINE_START_H; h < SCHEDULE_TIMELINE_END_H; h += 2) out.push(h)
  return out
})

function barStyle(s: Schedule | null): Record<string, string> | undefined {
  if (!s) return undefined
  const p = horizontalBarPercentages(selectedDate.value, s.shift_start, s.shift_end)
  if (!p) return undefined
  return { left: `${p.leftPct}%`, width: `${p.widthPct}%` }
}

function meetingStyles(uid: number): { style: Record<string, string>; title: string }[] {
  const out: { style: Record<string, string>; title: string }[] = []
  for (const m of teamMeetings.value) {
    if (day10(m.meeting_date) !== selectedDate.value || !(m.user_ids ?? []).includes(uid)) continue
    const p = horizontalBarPercentages(selectedDate.value, m.time_start, m.time_end)
    if (p) out.push({ style: { left: `${p.leftPct}%`, width: `${p.widthPct}%` }, title: teamMeetingBarTag(m) })
  }
  return out
}

/** Anwesende je halbe Stunde zwischen Beginn und Ende der Zeitleiste. */
function coverage(emps: Employee[]): { count: number; max: number }[] {
  const slots: number[] = []
  for (let t = SCHEDULE_TIMELINE_START_H * 60; t < SCHEDULE_TIMELINE_END_H * 60; t += 30) {
    let n = 0
    for (const e of emps) {
      const c = cellInfo(e.id, selectedDate.value)
      if (c.blocked || !c.schedule || (c.absence && !c.absence.half_day)) continue
      const a = clockToMinutes(c.schedule.shift_start)
      const b = clockToMinutes(c.schedule.shift_end)
      if (a != null && b != null && a <= t && b >= t + 30) n++
    }
    slots.push(n)
  }
  const max = Math.max(1, ...slots)
  return slots.map((count) => ({ count, max }))
}

function presentCount(emps: Employee[]): number {
  return emps.filter((e) => {
    const c = cellInfo(e.id, selectedDate.value)
    return !c.blocked && c.schedule && !(c.absence && !c.absence.half_day)
  }).length
}

function dayHasNote(date: string): boolean {
  return holidayByDate.value.has(date) || closureByDate.value.has(date)
}

// ---------- Person ----------

const personOptions = computed(() =>
  orderedEmployees.value.map((e) => ({ label: e.display_name, value: e.id })),
)

watch(
  orderedEmployees,
  (list) => {
    if (personId.value == null || !list.some((e) => e.id === personId.value)) personId.value = list[0]?.id ?? null
  },
  { immediate: true },
)

const person = computed(() => employees.value.find((e) => e.id === personId.value) ?? null)

// ---------- Menü und Vorwoche ----------

const menuRef = ref<InstanceType<typeof Menu> | null>(null)
const menuItems = computed(() => [
  { label: 'Vorwoche übernehmen', icon: 'pi pi-copy', command: () => askCopyPreviousWeek() },
  { label: 'Zur aktuellen Woche', icon: 'pi pi-calendar', command: () => goToday(), visible: !isCurrentWeek.value },
  {
    label: 'Tabellen-Ansicht (Notizen, Teamsitzungen, Excel)',
    icon: 'pi pi-table',
    command: () => emit('openTable'),
  },
])

const copyDialogVisible = ref(false)
const copyBusy = ref(false)
const copyOverwrite = ref(0)
const copySource = ref<Schedule[]>([])

async function askCopyPreviousWeek() {
  const p = shiftISOWeek(weekYear.value, week.value, -1)
  try {
    copySource.value = (await fetchSchedulesForWeek(p.year, p.week)).schedules
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Vorwoche', detail: (getApiErrorMessage(e) ?? 'Konnte nicht geladen werden.'), life: 8000 })
    return
  }
  if (!copySource.value.length) {
    toast.add({ severity: 'info', summary: 'Vorwoche', detail: `In KW ${p.week} ist nichts geplant.`, life: 5000 })
    return
  }
  copyOverwrite.value = copyTargets().filter((t) => {
    const cur = scheduleByCell.value.get(`${t.uid}_${t.date}`)
    return cur && (clockToMinutes(cur.shift_start) !== clockToMinutes(t.start) || clockToMinutes(cur.shift_end) !== clockToMinutes(t.end))
  }).length
  copyDialogVisible.value = true
}

/** Schichten der Vorwoche, um 7 Tage verschoben, ohne gesperrte Tage dieser Woche. */
function copyTargets(): { uid: number; date: string; start: string; end: string }[] {
  const planned = new Set(planEmployees.value.map((e) => e.id))
  const out: { uid: number; date: string; start: string; end: string }[] = []
  for (const s of copySource.value) {
    if (!planned.has(s.user_id)) continue
    const date = toISODateLocal(addDays(new Date(`${day10(s.schedule_date)}T12:00:00`), 7))
    if (!dates.value.includes(date)) continue
    if (cellInfo(s.user_id, date).blocked) continue
    out.push({ uid: s.user_id, date, start: s.shift_start, end: s.shift_end })
  }
  return out
}

async function runCopyPreviousWeek() {
  copyBusy.value = true
  let failed = 0
  for (const t of copyTargets()) {
    const cur = scheduleByCell.value.get(`${t.uid}_${t.date}`)
    if (cur && clockToMinutes(cur.shift_start) === clockToMinutes(t.start) && clockToMinutes(cur.shift_end) === clockToMinutes(t.end)) {
      continue
    }
    if (!(await writeCell(t.uid, t.date, t.start, t.end))) failed++
  }
  copyBusy.value = false
  copyDialogVisible.value = false
  await loadWeek()
  if (failed === 0) {
    toast.add({ severity: 'success', summary: 'Vorwoche übernommen', life: 3000 })
  }
}

const saveHint = computed(() => {
  if (saveState.value === 'saving') return 'speichert …'
  if (saveState.value === 'error') return 'nicht gespeichert'
  if (saveState.value === 'saved') return 'gespeichert'
  return ''
})
</script>

<template>
  <div class="msp" data-testid="mobile-schedule">
    <header class="msp-top">
      <div class="row">
        <button type="button" class="icon" aria-label="Vorherige Woche" data-testid="week-prev" @click="goWeek(-1)">
          <i class="pi pi-chevron-left" />
        </button>
        <div class="kw">
          <strong data-testid="week-label">KW {{ week }}</strong>
          <span>
            {{ weekRangeLabel }}
            <template v-if="saveHint"> · <em :class="'save-' + saveState">{{ saveHint }}</em></template>
          </span>
        </div>
        <button type="button" class="icon" aria-label="Nächste Woche" data-testid="week-next" @click="goWeek(1)">
          <i class="pi pi-chevron-right" />
        </button>
        <button
          type="button"
          class="icon"
          aria-label="Weitere Aktionen"
          aria-haspopup="true"
          data-testid="schedule-menu"
          @click="menuRef?.toggle($event)"
        >
          <i class="pi pi-ellipsis-h" />
        </button>
        <Menu ref="menuRef" :model="menuItems" popup />
      </div>
      <div class="seg" role="tablist">
        <button
          v-for="m in [
            { k: 'week', l: 'Woche' },
            { k: 'day', l: 'Tag' },
            { k: 'person', l: 'Person' },
          ] as const"
          :key="m.k"
          type="button"
          role="tab"
          :aria-selected="mode === m.k"
          :class="{ on: mode === m.k }"
          :data-testid="'mode-' + m.k"
          @click="mode = m.k"
        >
          {{ m.l }}
        </button>
      </div>
      <div v-if="mode === 'day'" class="days">
        <button
          v-for="(d, i) in dates"
          :key="d"
          type="button"
          class="d"
          :class="{ on: i === dayIndex, today: d === todayISO }"
          :data-testid="'day-chip-' + i"
          @click="dayIndex = i"
        >
          {{ WEEKDAYS[i] }}<b>{{ d.slice(8, 10) }}</b>
          <i v-if="dayHasNote(d)" class="dot" />
        </button>
      </div>
    </header>

    <p v-if="loadError" class="err">{{ loadError }}</p>
    <p v-else-if="loading && !planning" class="muted">Laden…</p>
    <p v-else-if="!planEmployees.length" class="muted">Keine aktiven Mitarbeitenden für den Dienstplan.</p>

    <!-- Woche -->
    <div v-else-if="mode === 'week'" class="body" :class="{ dim: loading }">
      <p v-if="!weekHasShifts && !loading" class="empty-week">
        In dieser Woche ist noch nichts geplant.
        <Button label="Vorwoche übernehmen" size="small" @click="askCopyPreviousWeek" />
      </p>
      <section v-for="(sec, si) in sections" :key="'w-' + si" class="sec">
        <h3 v-if="sec.title" class="grp">{{ sec.title }}</h3>
        <table class="wk">
          <colgroup>
            <col class="c-name" />
            <col v-for="d in dates" :key="'c' + d" />
          </colgroup>
          <thead v-if="si === 0">
            <tr>
              <th />
              <th
                v-for="(d, i) in dates"
                :key="'h' + d"
                :class="{ today: d === todayISO }"
                @click="
                  dayIndex = i;
                  mode = 'day'
                "
              >
                {{ WEEKDAYS[i] }}<small>{{ d.slice(8, 10) }}.</small>
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="emp in sec.employees" :key="emp.id">
              <td class="n">
                {{ shortName(emp.display_name) }}
                <small
                  :class="{ over: plannedLabel(emp.id).over, under: plannedLabel(emp.id).under }"
                  :data-testid="'planned-' + emp.id"
                >{{ plannedLabel(emp.id).text }}</small>
              </td>
              <td
                v-for="d in dates"
                :key="d"
                :class="'k-' + cellInfo(emp.id, d).kind + (cellInfo(emp.id, d).absence?.absence_type ? ' a-' + cellInfo(emp.id, d).absence!.absence_type : '')"
                :data-testid="`cell-${emp.id}-${d}`"
                role="button"
                tabindex="0"
                :aria-label="`${emp.display_name} ${formatGermanDate(d)}: ${cellInfo(emp.id, d).label || 'nichts geplant'}`"
                @click="openSheet(emp.id, d)"
                @keydown.enter="openSheet(emp.id, d)"
              >
                {{ cellInfo(emp.id, d).label || '+' }}
              </td>
            </tr>
          </tbody>
        </table>
      </section>
    </div>

    <!-- Tag -->
    <div v-else-if="mode === 'day'" class="body" :class="{ dim: loading }">
      <p class="day-title">
        {{ WEEKDAYS_LONG[dayIndex] }}, {{ formatGermanDate(selectedDate) }}
        <template v-if="holidayByDate.get(selectedDate)"> · Feiertag {{ holidayByDate.get(selectedDate) }}</template>
        <template v-else-if="closureByDate.get(selectedDate)"> · Schließtag {{ closureByDate.get(selectedDate) }}</template>
      </p>
      <section v-for="(sec, si) in sections" :key="'d-' + si" class="sec">
        <h3 class="grp">
          {{ sec.title || 'Alle' }}
          <i>{{ presentCount(sec.employees) }} von {{ sec.employees.length }} eingeplant</i>
        </h3>
        <div class="cov" :aria-label="`Besetzung ${sec.title}`">
          <div class="bars">
            <span
              v-for="(c, i) in coverage(sec.employees)"
              :key="i"
              :style="{ height: c.count ? `${20 + (80 * c.count) / c.max}%` : '0' }"
              :title="`${c.count} Personen`"
            />
          </div>
          <div class="hours">
            <span
              v-for="h in timelineHours"
              :key="h"
              :style="{ left: `${((h - SCHEDULE_TIMELINE_START_H) / (SCHEDULE_TIMELINE_END_H - SCHEDULE_TIMELINE_START_H)) * 100}%` }"
            >{{ h }}</span>
          </div>
        </div>
        <button
          v-for="emp in sec.employees"
          :key="emp.id"
          type="button"
          class="p"
          :data-testid="`day-row-${emp.id}`"
          @click="openSheet(emp.id, selectedDate)"
        >
          <span class="nm">
            {{ shortName(emp.display_name) }}
            <small>
              <template v-if="cellInfo(emp.id, selectedDate).schedule">
                {{ cellInfo(emp.id, selectedDate).schedule!.shift_start.slice(0, 5) }}–{{
                  cellInfo(emp.id, selectedDate).schedule!.shift_end.slice(0, 5)
                }}
              </template>
              <template v-else>{{ cellInfo(emp.id, selectedDate).note || 'nicht geplant' }}</template>
            </small>
          </span>
          <span
            v-if="cellInfo(emp.id, selectedDate).blocked || (cellInfo(emp.id, selectedDate).absence && !cellInfo(emp.id, selectedDate).absence!.half_day)"
            class="abs"
            :class="'a-' + (cellInfo(emp.id, selectedDate).absence?.absence_type ?? cellInfo(emp.id, selectedDate).kind)"
          >
            {{ cellInfo(emp.id, selectedDate).note }}
            <template v-if="cellInfo(emp.id, selectedDate).schedule"> · Schicht austragen?</template>
          </span>
          <span v-else class="trk">
            <span
              v-if="barStyle(cellInfo(emp.id, selectedDate).schedule)"
              class="bar"
              :style="barStyle(cellInfo(emp.id, selectedDate).schedule)"
            >
              {{ formatHoursShort(cellNetMinutes(emp.id, selectedDate)) }} h
            </span>
            <span
              v-for="(m, mi) in meetingStyles(emp.id)"
              :key="mi"
              class="mt"
              :style="m.style"
              :title="m.title"
            />
            <span v-if="!cellInfo(emp.id, selectedDate).schedule" class="plus">+</span>
          </span>
        </button>
      </section>
    </div>

    <!-- Person -->
    <div v-else class="body" :class="{ dim: loading }">
      <Select
        v-model="personId"
        :options="personOptions"
        option-label="label"
        option-value="value"
        class="person-select"
        filter
        aria-label="Person auswählen"
        data-testid="person-select"
      />
      <template v-if="person">
        <div class="psum">
          <span>{{ groupNameOf(person) || 'Ohne Gruppe' }}</span>
          <strong :class="{ over: plannedLabel(person.id).over }">
            <template v-if="weekTargetMinutes(person.id) > 0">
              geplant {{ formatHoursMinutes(weekPlannedMinutes(person.id)) }} von
              {{ formatHoursMinutes(weekTargetMinutes(person.id)) }}
            </template>
            <template v-else>geplant {{ formatHoursMinutes(weekPlannedMinutes(person.id)) }}</template>
          </strong>
        </div>
        <button
          v-for="(d, i) in dates"
          :key="d"
          type="button"
          class="pday"
          :class="'k-' + cellInfo(person.id, d).kind"
          :data-testid="`person-day-${i}`"
          @click="openSheet(person.id, d)"
        >
          <span class="pd-day">{{ WEEKDAYS[i] }} <small>{{ formatGermanDate(d).slice(0, 6) }}</small></span>
          <span class="pd-main">
            <template v-if="cellInfo(person.id, d).schedule">
              {{ cellInfo(person.id, d).schedule!.shift_start.slice(0, 5) }}–{{
                cellInfo(person.id, d).schedule!.shift_end.slice(0, 5)
              }}
              <small>{{ formatHoursMinutes(cellNetMinutes(person.id, d)) }}</small>
            </template>
            <template v-else-if="!cellInfo(person.id, d).note">nicht geplant</template>
            <em v-if="cellInfo(person.id, d).note">{{ cellInfo(person.id, d).note }}</em>
          </span>
          <i class="pi pi-angle-right" />
        </button>
      </template>
    </div>

    <ShiftEditSheet
      v-if="sheetEmployee"
      v-model:visible="sheetVisible"
      :employee-name="sheetEmployee.display_name"
      :date-label="sheetDateLabel"
      :group-name="groupNameOf(sheetEmployee)"
      :start="sheetCell?.schedule?.shift_start ?? ''"
      :end="sheetCell?.schedule?.shift_end ?? ''"
      :day="sheetDay"
      :quick-picks="sheetQuickPicks"
      :last-week="sheetLastWeek"
      :last-week-label="sheetLastWeekLabel"
      :meetings="sheetMeetings"
      :break-rules="planning?.break_rules ?? []"
      :week-other-minutes="weekPlannedMinutes(sheetEmployee.id, sheetDate)"
      :week-target-minutes="weekTargetMinutes(sheetEmployee.id)"
      :prev-label="navLabel(sheetNavTargets.prev)"
      :next-label="navLabel(sheetNavTargets.next)"
      :saving="sheetSaving"
      @save="onSheetSave"
      @clear="onSheetClear"
      @nav="onSheetNav"
    />

    <Dialog
      v-model:visible="copyDialogVisible"
      header="Vorwoche übernehmen"
      modal
      :style="{ width: 'min(420px, 94vw)' }"
      :closable="!copyBusy"
    >
      <p>
        Die Schichten aus KW {{ shiftISOWeek(weekYear, week, -1).week }} werden in KW {{ week }} übernommen. Tage mit
        Feiertag, Urlaub oder festem freien Tag werden übersprungen.
      </p>
      <p v-if="copyOverwrite > 0" class="warn-text" data-testid="copy-overwrite">
        Dabei werden {{ copyOverwrite }} bereits geplante {{ copyOverwrite === 1 ? 'Schicht' : 'Schichten' }}
        überschrieben.
      </p>
      <template #footer>
        <Button label="Abbrechen" text :disabled="copyBusy" @click="copyDialogVisible = false" />
        <Button label="Übernehmen" :loading="copyBusy" data-testid="copy-confirm" @click="runCopyPreviousWeek" />
      </template>
    </Dialog>

    <div v-if="undo" class="snack" role="status" data-testid="undo-snack">
      <span>{{ undo.text }}</span>
      <button type="button" data-testid="undo-btn" @click="doUndo">Rückgängig</button>
    </div>
  </div>
</template>

<style scoped>
.msp {
  margin: -0.75rem -0.75rem 0;
  padding-bottom: 5rem;
}
.msp-top {
  position: sticky;
  top: var(--layout-top-inset, 0px);
  z-index: 50;
  background: #fff;
  border-bottom: 1px solid #e2e8f0;
  padding: 0.5rem 0.75rem 0.6rem;
  margin: 0 -0.75rem 0.5rem;
}
.row {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}
.icon {
  width: 2.4rem;
  height: 2.4rem;
  border-radius: 10px;
  border: 1px solid #e2e8f0;
  background: #fff;
  color: #475569;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  flex: none;
}
.kw {
  flex: 1;
  text-align: center;
  line-height: 1.2;
}
.kw strong {
  display: block;
  font-size: 1.05rem;
}
.kw span {
  font-size: 0.75rem;
  color: #64748b;
}
.kw em {
  font-style: normal;
}
.save-error {
  color: #b91c1c;
  font-weight: 600;
}
.seg {
  display: flex;
  background: #f1f5f9;
  border-radius: 10px;
  padding: 3px;
  margin-top: 0.5rem;
}
.seg button {
  flex: 1;
  border: none;
  background: none;
  border-radius: 8px;
  padding: 0.4rem 0;
  font-size: 0.85rem;
  color: #475569;
  cursor: pointer;
}
.seg button.on {
  background: #fff;
  color: #0f172a;
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.08);
}
.days {
  display: flex;
  gap: 0.35rem;
  margin-top: 0.5rem;
}
.d {
  flex: 1;
  border: none;
  border-radius: 10px;
  padding: 0.3rem 0 0.35rem;
  background: #f1f5f9;
  color: #475569;
  font-size: 0.75rem;
  position: relative;
  cursor: pointer;
}
.d b {
  display: block;
  font-size: 0.95rem;
  color: #1e293b;
}
.d.today {
  box-shadow: inset 0 0 0 1.5px #10b981;
}
.d.on {
  background: #10b981;
  color: #fff;
}
.d.on b {
  color: #fff;
}
.dot {
  position: absolute;
  top: 4px;
  right: 6px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #f59e0b;
}
.body {
  padding: 0 0.1rem;
}
.body.dim {
  opacity: 0.6;
}
.muted {
  color: #64748b;
  padding: 0 0.25rem;
}
.err {
  color: #b91c1c;
  padding: 0 0.25rem;
}
.empty-week {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
  background: #f8fafc;
  border: 1px dashed #cbd5e1;
  border-radius: 10px;
  padding: 0.6rem 0.75rem;
  font-size: 0.88rem;
  color: #475569;
}
.sec {
  margin-bottom: 0.5rem;
}
.grp {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  font-size: 0.72rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: #64748b;
  margin: 0.7rem 0.25rem 0.3rem;
}
.grp i {
  font-style: normal;
  font-weight: 600;
  text-transform: none;
  letter-spacing: 0;
}
.wk {
  width: 100%;
  table-layout: fixed;
  border-collapse: separate;
  border-spacing: 0 3px;
  font-size: 0.78rem;
}
.c-name {
  width: 5.2rem;
}
.wk th {
  font-weight: 600;
  color: #64748b;
  font-size: 0.7rem;
  text-align: center;
  padding-bottom: 0.1rem;
  cursor: pointer;
}
.wk th small {
  display: block;
  font-weight: 500;
}
.wk th.today {
  color: #059669;
}
.wk td {
  background: #fff;
  text-align: center;
  padding: 0.55rem 0;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: clip;
  cursor: pointer;
  letter-spacing: -0.01em;
}
.wk td.n {
  text-align: left;
  padding-left: 0.5rem;
  border-radius: 10px 0 0 10px;
  font-weight: 600;
  line-height: 1.15;
  cursor: default;
  white-space: normal;
}
.wk td.n small {
  display: block;
  font-weight: 500;
  color: #64748b;
  font-size: 0.66rem;
}
.wk td.n small.over {
  color: #b45309;
}
.wk td:last-child {
  border-radius: 0 10px 10px 0;
}
.wk td.k-empty {
  color: #cbd5e1;
}
.wk td.k-fixed,
.wk td.k-holiday {
  color: #94a3b8;
  background: #f8fafc;
}
.wk td.k-blocked-absence,
.wk td.a-vacation,
.wk td.a-compensation_day {
  color: #4338ca;
  font-weight: 600;
  background: #eef2ff;
}
.wk td.a-sick {
  color: #b91c1c;
  font-weight: 600;
  background: #fef2f2;
}
.wk td.a-other {
  color: #475569;
  font-weight: 600;
  background: #f1f5f9;
}
.wk td.k-absence.a-vacation,
.wk td.k-absence.a-compensation_day {
  font-weight: 500;
}
.day-title {
  font-size: 0.85rem;
  color: #475569;
  margin: 0.2rem 0.25rem 0;
}
.cov {
  background: #fff;
  border-radius: 10px;
  padding: 0.4rem 0.6rem 0.3rem;
  margin-bottom: 0.3rem;
}
.bars {
  display: flex;
  gap: 1px;
  height: 18px;
  align-items: flex-end;
}
.bars span {
  flex: 1;
  border-radius: 2px;
  background: #6ee7b7;
}
.hours {
  position: relative;
  height: 0.8rem;
  font-size: 0.6rem;
  color: #94a3b8;
  margin-top: 2px;
}
.hours span {
  position: absolute;
  transform: translateX(-50%);
}
.hours span:first-child {
  transform: none;
}
.p {
  width: 100%;
  border: none;
  background: #fff;
  border-radius: 10px;
  padding: 0.5rem 0.6rem;
  margin-bottom: 0.25rem;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  text-align: left;
  font: inherit;
  cursor: pointer;
}
.nm {
  width: 4.8rem;
  flex: none;
  font-size: 0.82rem;
  font-weight: 600;
  line-height: 1.15;
}
.nm small {
  display: block;
  font-weight: 500;
  color: #64748b;
  font-size: 0.68rem;
}
.trk {
  flex: 1;
  height: 1.6rem;
  position: relative;
  border-radius: 6px;
  background-color: #f8fafc;
  background-image: repeating-linear-gradient(
    90deg,
    #e2e8f0 0 1px,
    transparent 1px calc(100% / 7)
  );
}
.bar {
  position: absolute;
  top: 3px;
  bottom: 3px;
  background: #bfdbfe;
  border: 1.5px solid #3b82f6;
  border-radius: 6px;
  font-size: 0.66rem;
  font-weight: 600;
  color: #1e3a8a;
  display: flex;
  align-items: center;
  justify-content: center;
  white-space: nowrap;
  overflow: hidden;
}
.mt {
  position: absolute;
  top: 0;
  bottom: 0;
  background: rgba(245, 158, 11, 0.28);
  border-left: 2px solid #f59e0b;
  pointer-events: none;
}
.plus {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #cbd5e1;
}
.abs {
  flex: 1;
  text-align: center;
  font-size: 0.78rem;
  font-weight: 600;
  border-radius: 6px;
  padding: 0.3rem 0;
  background: #f1f5f9;
  color: #475569;
}
.abs.a-vacation,
.abs.a-compensation_day {
  background: #eef2ff;
  color: #4338ca;
}
.abs.a-sick {
  background: #fef2f2;
  color: #b91c1c;
}
.person-select {
  width: 100%;
  margin-top: 0.25rem;
}
.psum {
  display: flex;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 0.25rem 0.75rem;
  font-size: 0.85rem;
  color: #475569;
  margin: 0.6rem 0.25rem;
}
.psum .over {
  color: #b45309;
}
.pday {
  width: 100%;
  border: none;
  background: #fff;
  border-radius: 10px;
  padding: 0.7rem 0.75rem;
  margin-bottom: 0.35rem;
  display: flex;
  align-items: center;
  gap: 0.75rem;
  text-align: left;
  font: inherit;
  cursor: pointer;
}
.pd-day {
  width: 3.4rem;
  font-weight: 700;
}
.pd-day small {
  display: block;
  font-weight: 500;
  color: #64748b;
  font-size: 0.72rem;
}
.pd-main {
  flex: 1;
  font-variant-numeric: tabular-nums;
}
.pd-main small {
  color: #64748b;
  margin-left: 0.4rem;
}
.pd-main em {
  display: block;
  font-style: normal;
  font-size: 0.8rem;
  color: #6366f1;
}
.pday.k-empty .pd-main {
  color: #94a3b8;
}
.pday i {
  color: #94a3b8;
}
.warn-text {
  color: #9a3412;
}
.snack {
  position: fixed;
  left: 0.75rem;
  right: 0.75rem;
  bottom: calc(0.9rem + env(safe-area-inset-bottom, 0px));
  z-index: 1200;
  background: #1e293b;
  color: #fff;
  border-radius: 12px;
  padding: 0.75rem 0.9rem;
  font-size: 0.85rem;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.75rem;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.3);
}
.snack button {
  border: none;
  background: none;
  color: #6ee7b7;
  font-weight: 700;
  font-size: 0.88rem;
  cursor: pointer;
  padding: 0;
}
</style>
