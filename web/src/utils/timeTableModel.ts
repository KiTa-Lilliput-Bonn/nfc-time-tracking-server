import type {
  Absence,
  AbsenceCredit,
  BreakRule,
  FixedNonWorkWeekdays,
  ScheduleBoundSetting,
  HolidayCredit,
  TeamMeeting,
  TimeCorrection,
  WeeklyHours,
  WorkPeriod,
} from '@/types/api'
import { localInstantISOFromDateAndClock, maxInstantISO } from '@/utils/dates'
import { fixedNonWorkWeekdaysForDate, normalizeISODate } from '@/utils/workdays'

export interface TimeTableRow {
  /** Eindeutiger Schlüssel für Tabellenzeilen (z. B. Tag + Perioden-ID). */
  rowKey: string
  workDate: string
  /** Arbeitsperiode dieser Zeile; null bei reinem Tageseintrag ohne Intervall. */
  primaryPeriodId: number | null
  effectiveIn: string | null
  /** Frühestes Kommen (Korrektur oder Stempel), ohne Schicht-Schnitt — nur zur Anzeige neben effektivem Beginn. */
  stampInEarliest: string | null
  effectiveOut: string | null
  gross: number
  net: number
  notes: string
  /** Dynamisch: manuelle Zeit vor Dienstplanbeginn (nicht in Korrektur-Grund gespeichert). */
  rowHint: string | null
  /** Für Korrektur-Dialog: typischerweise eine Periode pro Zeile. */
  candidates: WorkPeriod[]
}

export interface CalendarSegment {
  workPeriodId: number
  workDate: string
  effectiveIn: string
  effectiveOut: string | null
  isBreak: boolean
  /** Unterscheidet mehrere Anzeige-Segmente einer Periode (Schicht + Sitzung). */
  segmentSuffix?: string
  /** Dynamisch: manuelle Zeit vor Dienstplanbeginn wird gezählt. */
  preShiftHint?: string | null
}

export const MANUAL_PRE_SHIFT_HINT = 'Zeit vor Dienstplanbeginn wird gezählt'

/** Hinweis nur wenn manuelle Periode vor aktuellem Schichtbeginn liegt (zur Laufzeit berechnet). */
export function manualPreShiftCountedHint(
  period: WorkPeriod,
  rawIn: string,
  workDate: string,
  scheduleByDate?: Record<string, { shift_start: string; shift_end: string }>,
  scheduleBoundHistory?: ScheduleBoundSetting[],
): string | null {
  if (period.source !== 'manual') return null
  if (!scheduleBoundForDate(scheduleBoundHistory, workDate)) return null
  const shiftStart = scheduleByDate?.[workDate]?.shift_start?.trim()
  if (!shiftStart) return null
  const shiftIso = localInstantISOFromDateAndClock(workDate, shiftStart)
  if (!shiftIso) return null
  if (new Date(rawIn).getTime() < new Date(shiftIso).getTime()) {
    return MANUAL_PRE_SHIFT_HINT
  }
  return null
}

const DEFAULT_BREAK_RULES: BreakRule[] = [
  { min_work_hours: 6.0, break_minutes: 30 },
  { min_work_hours: 9.0, break_minutes: 45 },
]

export function absenceByDate(absences?: Absence[]): Record<string, Absence> {
  const m: Record<string, Absence> = {}
  for (const a of absences ?? []) {
    m[calendarDayKey(a.absence_date)] = a
  }
  return m
}

export function holidayByDate(holidays?: HolidayCredit[]): Record<string, HolidayCredit> {
  const m: Record<string, HolidayCredit> = {}
  for (const h of holidays ?? []) {
    m[calendarDayKey(h.holiday_date)] = h
  }
  return m
}

export function absenceCreditByDate(credits?: AbsenceCredit[]): Record<string, AbsenceCredit> {
  const m: Record<string, AbsenceCredit> = {}
  for (const c of credits ?? []) {
    m[calendarDayKey(c.absence_date)] = c
  }
  return m
}

export function correctionByWorkPeriod(corrections?: TimeCorrection[]): Map<number, TimeCorrection> {
  const m = new Map<number, TimeCorrection>()
  const list = [...(corrections ?? [])].sort((a, b) => {
    const ta = new Date(a.created_at).getTime()
    const tb = new Date(b.created_at).getTime()
    if (ta !== tb) return tb - ta
    return (b.id ?? 0) - (a.id ?? 0)
  })
  for (const c of list) {
    if (!m.has(c.work_period_id)) m.set(c.work_period_id, c)
  }
  return m
}

function round2(x: number) {
  return Math.round(x * 100) / 100
}

function roundDownMinutes(totalMinutes: number, gridMin: number) {
  if (gridMin <= 0) return totalMinutes
  return Math.floor(totalMinutes / gridMin) * gridMin
}

function calcBreakDeductionMinutes(grossWorkMinutes: number, stampedBreakMinutes: number, rules: BreakRule[]) {
  if (!rules?.length) return 0
  const grossH = grossWorkMinutes / 60
  let required = 0
  let bestThreshold = 0
  for (const r of rules) {
    if (grossH + 1e-9 >= r.min_work_hours && r.min_work_hours >= bestThreshold) {
      bestThreshold = r.min_work_hours
      required = r.break_minutes
    }
  }
  if (required <= 0) return 0
  if (stampedBreakMinutes >= required) return 0
  return required - stampedBreakMinutes
}

function hoursBetween(aISO: string, bISO: string) {
  const a = new Date(aISO).getTime()
  const b = new Date(bISO).getTime()
  if (!Number.isFinite(a) || !Number.isFinite(b) || b <= a) return 0
  return round2((b - a) / 3_600_000)
}

function scheduledWorkdaysPerWeek(fixed: number[] | undefined): number {
  const s = new Set<number>()
  for (const w of fixed ?? []) {
    if (w >= 1 && w <= 5) s.add(w)
  }
  return Math.max(1, 5 - s.size)
}

function dailyHoursFromWeekly(hoursPerWeek: number, fixed: number[] | undefined): number {
  if (!hoursPerWeek || hoursPerWeek <= 0) return 0
  return hoursPerWeek / scheduledWorkdaysPerWeek(fixed)
}

function weeklyHoursForDate(workDate: string, rows: WeeklyHours[] | undefined): WeeklyHours | null {
  if (!rows?.length) return null
  const day = normalizeISODate(workDate)
  let best: WeeklyHours | null = null
  for (const r of rows) {
    const vf = normalizeISODate(r.valid_from)
    if (vf <= day) {
      if (!best || vf > normalizeISODate(best.valid_from) || (vf === normalizeISODate(best.valid_from) && r.id > best.id)) {
        best = r
      }
    }
  }
  return best
}

/** Default true: ohne Historie ist jeder Tag an den Dienstplan gebunden. */
export function scheduleBoundForDate(
  rows: ScheduleBoundSetting[] | undefined,
  workDate: string,
): boolean {
  if (!rows?.length) return true
  const day = normalizeISODate(workDate)
  let best: ScheduleBoundSetting | null = null
  for (const r of rows) {
    const vf = normalizeISODate(r.valid_from)
    if (vf <= day) {
      if (!best || vf > normalizeISODate(best.valid_from)) best = r
    }
  }
  return best ? best.schedule_bound : true
}

function calendarDayKey(s: string): string {
  return normalizeISODate(s) || s
}

function absenceCreditHoursFallback(abs: Absence | undefined, dailyHours: number): number {
  if (!abs || dailyHours <= 0) return 0
  if (abs.absence_type === 'vacation' || abs.absence_type === 'sick' || abs.absence_type === 'other') {
    return abs.half_day ? dailyHours / 2 : dailyHours
  }
  return 0
}

export function pickPrimaryWorkPeriod(periods: WorkPeriod[]) {
  const man = periods.find((p) => p.source === 'manual')
  return man ?? periods[0] ?? null
}

export function buildTimeTableRows(opts: {
  periods: WorkPeriod[]
  absences?: Absence[]
  corrections?: TimeCorrection[]
  holidays?: HolidayCredit[]
  absenceCredits?: AbsenceCredit[]
  scheduleByDate?: Record<string, { shift_start: string; shift_end: string }>
  breakRules?: BreakRule[]
  roundingMinutes?: number
  weeklyHours?: WeeklyHours[]
  fixedNonWorkWeekdays?: number[]
  fixedNonWorkWeekdaysHistory?: FixedNonWorkWeekdays[]
  scheduleBoundHistory?: ScheduleBoundSetting[]
}): TimeTableRow[] {
  const rules: BreakRule[] =
    opts.breakRules && opts.breakRules.length ? opts.breakRules : DEFAULT_BREAK_RULES
  const roundMin = opts.roundingMinutes ?? 15
  const absMap = absenceByDate(opts.absences)
  const holMap = holidayByDate(opts.holidays)
  const absCreditMap = absenceCreditByDate(opts.absenceCredits)
  const corrMap = correctionByWorkPeriod(opts.corrections ?? [])

  const byDate = new Map<string, WorkPeriod[]>()
  for (const p of opts.periods) {
    const key = calendarDayKey(p.work_date)
    const arr = byDate.get(key) ?? []
    arr.push(p)
    byDate.set(key, arr)
  }
  for (const a of opts.absences ?? []) {
    const key = calendarDayKey(a.absence_date)
    if (!byDate.has(key)) byDate.set(key, [])
  }
  for (const h of opts.holidays ?? []) {
    const key = calendarDayKey(h.holiday_date)
    if (!byDate.has(key)) byDate.set(key, [])
  }

  const dates = [...byDate.keys()].sort((a, b) => a.localeCompare(b))
  const out: TimeTableRow[] = []

  for (const workDate of dates) {
    const dayPeriods = (byDate.get(workDate) ?? []).slice().sort((a, b) => a.punch_in.localeCompare(b.punch_in))
    const candidates = dayPeriods
      .filter((p) => !p.is_break)
      .slice()
      .sort((a, b) => a.punch_in.localeCompare(b.punch_in))
    const abs = absMap[workDate]
    const schDay = opts.scheduleByDate?.[workDate]

    let hasManual = false
    let hasOpen = false
    const corrReasons: string[] = []

    for (const p of dayPeriods) {
      if (p.source === 'manual') hasManual = true
      const corr = corrMap.get(p.id)
      if (corr?.reason) corrReasons.push(corr.reason)
      if (!p.is_break) {
        const effOut = corr?.corrected_out ?? p.punch_out
        if (!effOut) hasOpen = true
      }
    }

    const h = holMap[workDate]
    const holCredit = h && Number.isFinite(h.credit_hours) ? h.credit_hours : 0
    const wh = weeklyHoursForDate(workDate, opts.weeklyHours)
    const hpw = wh?.hours_per_week ?? 0
    const fnw =
      opts.fixedNonWorkWeekdaysHistory != null
        ? fixedNonWorkWeekdaysForDate(workDate, opts.fixedNonWorkWeekdaysHistory)
        : opts.fixedNonWorkWeekdays
    const dailyAbs = hpw > 0 ? dailyHoursFromWeekly(hpw, fnw) : 0
    const absCred = absCreditMap[workDate]
    const absCredit =
      absCred && Number.isFinite(absCred.credit_hours)
        ? absCred.credit_hours
        : absenceCreditHoursFallback(abs, dailyAbs)
    const credit = holCredit + absCredit

    let notesFirst = ''
    if (hasManual) notesFirst = 'manuell'
    if (holMap[workDate]) {
      const hh = holMap[workDate]!
      const label = `Feiertag: ${hh.name}`
      notesFirst = notesFirst ? `${notesFirst}; ${label}` : label
    }
    if (abs) {
      const t =
        abs.absence_type === 'vacation'
          ? 'Urlaub'
          : abs.absence_type === 'sick'
            ? 'Krank'
            : abs.absence_type === 'compensation_day'
              ? 'Ausgleichstag'
              : 'Abwesend'
      const label = t + (abs.half_day ? ' (½)' : '')
      notesFirst = notesFirst ? `${notesFirst}; ${label}` : label
    }
    if (hasOpen) notesFirst = notesFirst ? `${notesFirst}; offen` : 'offen'
    const uniqReasons = [...new Set(corrReasons.filter((x) => x.trim()))]
    if (uniqReasons.length) {
      const r = uniqReasons.length === 1 ? uniqReasons[0]! : uniqReasons.join(' | ')
      notesFirst = notesFirst ? `${notesFirst}; Korrektur: ${r}` : `Korrektur: ${r}`
    }

    if (candidates.length === 0) {
      out.push({
        rowKey: `${workDate}-day`,
        workDate,
        primaryPeriodId: null,
        effectiveIn: null,
        stampInEarliest: null,
        effectiveOut: null,
        gross: round2(credit),
        net: round2(credit),
        notes: notesFirst,
        rowHint: null,
        candidates: [],
      })
      continue
    }

    const dayRows: TimeTableRow[] = []
    for (const p of candidates) {
      const corr = corrMap.get(p.id)
      const rawIn = corr ? corr.corrected_in : p.punch_in
      let effectiveIn = rawIn
      const effectiveOut = corr ? corr.corrected_out : p.punch_out
      const rowHint = manualPreShiftCountedHint(
        p,
        rawIn,
        workDate,
        opts.scheduleByDate,
        opts.scheduleBoundHistory,
      )
      if (
        p.source !== 'manual' &&
        scheduleBoundForDate(opts.scheduleBoundHistory, workDate) &&
        schDay?.shift_start?.trim()
      ) {
        const shiftIso = localInstantISOFromDateAndClock(workDate, schDay.shift_start.trim())
        if (shiftIso) effectiveIn = maxInstantISO(effectiveIn, shiftIso)
      }

      if (!effectiveOut) {
        dayRows.push({
          rowKey: `${workDate}-wp${p.id}`,
          workDate,
          primaryPeriodId: p.id,
          effectiveIn,
          stampInEarliest: rawIn,
          effectiveOut: null,
          gross: 0,
          net: 0,
          notes: '',
          rowHint,
          candidates: [p],
        })
        continue
      }

      const blockMin = Math.round(hoursBetween(effectiveIn, effectiveOut) * 60)
      let blockDed = calcBreakDeductionMinutes(blockMin, 0, rules)
      if (blockDed > blockMin) blockDed = blockMin
      let netMin = blockMin - blockDed
      if (netMin < 0) netMin = 0
      netMin = roundDownMinutes(netMin, roundMin)

      dayRows.push({
        rowKey: `${workDate}-wp${p.id}`,
        workDate,
        primaryPeriodId: p.id,
        effectiveIn,
        stampInEarliest: rawIn,
        effectiveOut,
        gross: round2(blockMin / 60),
        net: round2(netMin / 60),
        notes: '',
        rowHint,
        candidates: [p],
      })
    }

    const credIdx = dayRows.findIndex((r) => r.effectiveOut)
    if (credit !== 0 && dayRows.length) {
      const idx = credIdx >= 0 ? credIdx : 0
      dayRows[idx]!.gross = round2(dayRows[idx]!.gross + credit)
      dayRows[idx]!.net = round2(dayRows[idx]!.net + credit)
    }

    if (notesFirst) {
      if (dayRows.length) dayRows[0]!.notes = notesFirst
    }
    out.push(...dayRows)
  }

  return out
}

export type BuildCalendarSegmentsOptions = {
  /** Wenn false: Stempelbeginn nicht an Schichtstart klammern (Ist-Anzeige). Default true. */
  applyShiftClamp?: boolean
  /** Nur mit applyShiftClamp: Schnittmengen Schichtfenster ∪ Sitzungsfenster wie Backend daycalc. */
  teamMeetings?: TeamMeeting[]
  scheduleBoundHistory?: ScheduleBoundSetting[]
}

function mergeIntervalsMs(parts: [number, number][]): [number, number][] {
  if (!parts.length) return []
  const s = [...parts].sort((a, b) => a[0] - b[0])
  const out: [number, number][] = []
  let cs = s[0]![0]
  let ce = s[0]![1]
  for (let i = 1; i < s.length; i++) {
    const [a, b] = s[i]!
    if (a <= ce) ce = Math.max(ce, b)
    else {
      out.push([cs, ce])
      cs = a
      ce = b
    }
  }
  out.push([cs, ce])
  return out
}

function intersectMs(a0: number, a1: number, b0: number, b1: number): [number, number] | null {
  const s = Math.max(a0, b0)
  const e = Math.min(a1, b1)
  if (!(s < e)) return null
  return [s, e]
}

/** Union aus Schicht ∩ Stempel und je Sitzung ∩ Stempel (lokal, ms seit Epoche). */
function workedDisplayIntervalsMs(
  workDate: string,
  punchIn: string,
  punchOut: string,
  schDay: { shift_start: string; shift_end: string } | undefined,
  meetings: TeamMeeting[],
): [number, number][] {
  const pin = new Date(punchIn).getTime()
  const pout = new Date(punchOut).getTime()
  if (!(pin < pout)) return []
  const parts: [number, number][] = []
  if (schDay?.shift_start?.trim() && schDay.shift_end?.trim()) {
    const ss = localInstantISOFromDateAndClock(workDate, schDay.shift_start.trim())
    const se = localInstantISOFromDateAndClock(workDate, schDay.shift_end.trim())
    if (ss && se) {
      const t0 = new Date(ss).getTime()
      const t1 = new Date(se).getTime()
      if (t0 < t1) {
        const x = intersectMs(pin, pout, t0, t1)
        if (x) parts.push(x)
      }
    }
  }
  for (const m of meetings) {
    const a = localInstantISOFromDateAndClock(workDate, m.time_start.trim())
    const b = localInstantISOFromDateAndClock(workDate, m.time_end.trim())
    if (!a || !b) continue
    const t0 = new Date(a).getTime()
    const t1 = new Date(b).getTime()
    if (!(t0 < t1)) continue
    const x = intersectMs(pin, pout, t0, t1)
    if (x) parts.push(x)
  }
  return mergeIntervalsMs(parts)
}

/** Segmente für Kalenderblöcke (effektive Zeiten wie in der Tabelle). */
export function buildCalendarSegmentsForDay(
  workDate: string,
  dayPeriods: WorkPeriod[],
  corrByWp: Map<number, TimeCorrection>,
  scheduleByDate?: Record<string, { shift_start: string; shift_end: string }>,
  options?: BuildCalendarSegmentsOptions,
): CalendarSegment[] {
  let applyShiftClamp = options?.applyShiftClamp !== false
  if (applyShiftClamp) {
    applyShiftClamp = scheduleBoundForDate(options?.scheduleBoundHistory, workDate)
  }
  const schDay = scheduleByDate?.[workDate]
  const meetings = options?.teamMeetings ?? []
  const sorted = [...dayPeriods].sort((a, b) => a.punch_in.localeCompare(b.punch_in))
  const out: CalendarSegment[] = []
  for (const p of sorted) {
    const corr = corrByWp.get(p.id)
    const rawIn = corr ? corr.corrected_in : p.punch_in
    const effectiveOut = corr ? corr.corrected_out : p.punch_out
    const preShiftHint = manualPreShiftCountedHint(
      p,
      rawIn,
      workDate,
      scheduleByDate,
      options?.scheduleBoundHistory,
    )
    if (p.is_break || !effectiveOut) {
      out.push({
        workPeriodId: p.id,
        workDate: p.work_date,
        effectiveIn: rawIn,
        effectiveOut,
        isBreak: p.is_break,
        preShiftHint,
      })
      continue
    }
    if (applyShiftClamp && meetings.length > 0 && p.source !== 'manual') {
      const intervals = workedDisplayIntervalsMs(workDate, rawIn, effectiveOut, schDay, meetings)
      if (intervals.length > 0) {
        intervals.forEach(([a, b], idx) => {
          out.push({
            workPeriodId: p.id,
            workDate: p.work_date,
            effectiveIn: new Date(a).toISOString(),
            effectiveOut: new Date(b).toISOString(),
            isBreak: false,
            segmentSuffix: intervals.length > 1 ? `-p${idx}` : '',
            preShiftHint: idx === 0 ? preShiftHint : null,
          })
        })
        continue
      }
    }
    let effectiveIn = rawIn
    if (applyShiftClamp && p.source !== 'manual' && schDay?.shift_start?.trim()) {
      const shiftIso = localInstantISOFromDateAndClock(workDate, schDay.shift_start.trim())
      if (shiftIso) effectiveIn = maxInstantISO(effectiveIn, shiftIso)
    }
    out.push({
      workPeriodId: p.id,
      workDate: p.work_date,
      effectiveIn,
      effectiveOut,
      isBreak: p.is_break,
      preShiftHint,
    })
  }
  return out
}

/** Geplante Schicht mit Start und Ende (Dienstplan). */
export function hasPlannedShift(
  workDate: string,
  scheduleByDate?: Record<string, { shift_start: string; shift_end: string }>,
): boolean {
  const s = scheduleByDate?.[workDate]
  if (!s) return false
  return Boolean(s.shift_start?.trim() && s.shift_end?.trim())
}

/** Abwesenheit, die den Tag nicht als „Frei“ erscheinen lässt. */
export function hasBlockingAbsenceForCalendar(abs?: Absence): boolean {
  if (!abs) return false
  return (
    abs.absence_type === 'vacation' ||
    abs.absence_type === 'sick' ||
    abs.absence_type === 'other' ||
    abs.absence_type === 'compensation_day'
  )
}

/** Werktag ohne Schicht, ohne Ist-Arbeit (nicht-Pause), ohne Feiertag und ohne blockierende Abwesenheit. */
export function isFreeWorkday(opts: {
  workDate: string
  periods: WorkPeriod[]
  absences?: Absence[]
  holidays?: HolidayCredit[]
  scheduleByDate?: Record<string, { shift_start: string; shift_end: string }>
}): boolean {
  const holMap = holidayByDate(opts.holidays)
  if (holMap[opts.workDate]) return false
  const absMap = absenceByDate(opts.absences)
  if (hasBlockingAbsenceForCalendar(absMap[opts.workDate])) return false
  if (hasPlannedShift(opts.workDate, opts.scheduleByDate)) return false
  const nonBreak = opts.periods.filter((p) => p.work_date === opts.workDate && !p.is_break)
  return nonBreak.length === 0
}

export function dayStatusClass(
  workDate: string,
  absences?: Absence[],
  holidays?: HolidayCredit[],
): string {
  const holMap = holidayByDate(holidays)
  if (holMap[workDate]) return 'col-holiday'
  const absMap = absenceByDate(absences)
  const abs = absMap[workDate]
  if (abs?.absence_type === 'vacation') return 'col-vacation'
  if (abs?.absence_type === 'sick') return 'col-sick'
  if (abs?.absence_type === 'compensation_day') return 'col-compensation'
  if (abs?.absence_type === 'other') return 'col-other'
  return ''
}
