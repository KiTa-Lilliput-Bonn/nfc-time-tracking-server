import type { Absence, BreakRule } from '@/types/api'

/** „07:30“ → 450 Minuten; ungültig → null. */
export function clockToMinutes(s: string | undefined | null): number | null {
  const m = String(s ?? '').trim().match(/^(\d{1,2}):(\d{2})/)
  if (!m) return null
  const h = Number.parseInt(m[1]!, 10)
  const min = Number.parseInt(m[2]!, 10)
  if (!Number.isFinite(h) || !Number.isFinite(min) || h > 23 || min > 59) return null
  return h * 60 + min
}

export function minutesToClock(total: number): string {
  const t = Math.max(0, Math.min(23 * 60 + 59, Math.round(total)))
  return `${String(Math.floor(t / 60)).padStart(2, '0')}:${String(t % 60).padStart(2, '0')}`
}

const FRACTIONS: Record<number, string> = { 15: '¼', 30: '½', 45: '¾' }

/** Kurze Uhrzeit für die kompakte Wochenansicht: 07:00 → „7“, 07:30 → „7½“, 07:10 → „7:10“. */
export function shortClock(s: string): string {
  const m = clockToMinutes(s)
  if (m == null) return s.trim()
  const h = Math.floor(m / 60)
  const min = m % 60
  if (min === 0) return String(h)
  const frac = FRACTIONS[min]
  if (frac) return `${h}${frac}`
  return `${h}:${String(min).padStart(2, '0')}`
}

/** „7–14“, „7½–13½“; leer, wenn eine Seite fehlt. */
export function shortShiftLabel(start: string, end: string): string {
  if (!start.trim() || !end.trim()) return ''
  return `${shortClock(start)}–${shortClock(end)}`
}

/** Bruttominuten einer Schicht (0 bei ungültig oder Ende ≤ Beginn). */
export function shiftGrossMinutes(start: string, end: string): number {
  const a = clockToMinutes(start)
  const b = clockToMinutes(end)
  if (a == null || b == null || b <= a) return 0
  return b - a
}

/**
 * Pflichtpause in Minuten nach denselben progressiven Regeln wie der Server
 * (`timecalc.progressiveRequiredBreak`): je Schwelle wächst die Pause minutengenau bis zum absoluten Wert.
 */
export function requiredBreakMinutes(grossMinutes: number, rules: BreakRule[]): number {
  if (!rules.length || grossMinutes <= 0) return 0
  const sorted = [...rules].sort((a, b) =>
    a.min_work_hours === b.min_work_hours
      ? a.break_minutes - b.break_minutes
      : a.min_work_hours - b.min_work_hours,
  )
  let required = 0
  let prevRequired = 0
  for (const r of sorted) {
    const incremental = Math.max(0, r.break_minutes - prevRequired)
    const threshold = Math.round(r.min_work_hours * 60)
    const over = Math.max(0, grossMinutes - threshold)
    required += Math.min(over, incremental)
    prevRequired = r.break_minutes
  }
  return required
}

/** Geplante Arbeitszeit einer Schicht nach Pausenabzug (Minuten). */
export function shiftNetMinutes(start: string, end: string, rules: BreakRule[]): number {
  const gross = shiftGrossMinutes(start, end)
  return gross - requiredBreakMinutes(gross, rules)
}

/** Stunden als „7:30 h“ bzw. „35 h“. */
export function formatHoursMinutes(minutes: number): string {
  const sign = minutes < 0 ? '−' : ''
  const m = Math.abs(Math.round(minutes))
  const h = Math.floor(m / 60)
  const r = m % 60
  return r === 0 ? `${sign}${h} h` : `${sign}${h}:${String(r).padStart(2, '0')} h`
}

/** Kompakt für Namenszeilen: 1800 → „30“, 1830 → „30½“, 1810 → „30,2“. */
export function formatHoursShort(minutes: number): string {
  const m = Math.round(minutes)
  if (m % 60 === 0) return String(m / 60)
  if (m % 30 === 0) return `${Math.floor(m / 60)}½`
  return (m / 60).toLocaleString('de-DE', { maximumFractionDigits: 1 })
}

/** Regulärer Arbeitstag (Mo–Fr ohne fix freie Wochentage); getDay()-Zählung 1=Mo … 5=Fr. */
export function regularWorkdaysPerWeek(fixedNonWorkWeekdays: number[]): number {
  const fixed = new Set(fixedNonWorkWeekdays.filter((d) => d >= 1 && d <= 5))
  return Math.max(1, 5 - fixed.size)
}

export interface DayTargetInput {
  /** getDay(): 1=Mo … 5=Fr */
  weekday: number
  fixedNonWorkWeekdays: number[]
  /** Feiertag oder Schließtag */
  closed: boolean
  absence?: Pick<Absence, 'absence_type' | 'half_day'> | null
}

/**
 * Soll-Minuten, die an diesem Tag noch zu verplanen sind: Tagessoll (Wochenstunden ÷ reguläre Arbeitstage),
 * 0 an fix freien Tagen, Feier-/Schließtagen und ganztägigen Abwesenheiten, die Hälfte bei halbtägigen.
 */
export function plannableDayTargetMinutes(hoursPerWeek: number, d: DayTargetInput): number {
  if (hoursPerWeek <= 0) return 0
  if (d.fixedNonWorkWeekdays.includes(d.weekday)) return 0
  if (d.closed) return 0
  const daily = (hoursPerWeek * 60) / regularWorkdaysPerWeek(d.fixedNonWorkWeekdays)
  if (d.absence) return d.absence.half_day ? daily / 2 : 0
  return daily
}

export interface ShiftPattern {
  start: string
  end: string
  count: number
}

/** Häufigste Schichten (Beginn/Ende) absteigend nach Anzahl, bei Gleichstand frühere zuerst. */
export function frequentShifts(shifts: { shift_start: string; shift_end: string }[], limit = 3): ShiftPattern[] {
  const counts = new Map<string, ShiftPattern>()
  for (const s of shifts) {
    const a = clockToMinutes(s.shift_start)
    const b = clockToMinutes(s.shift_end)
    if (a == null || b == null || b <= a) continue
    const key = `${minutesToClock(a)}-${minutesToClock(b)}`
    const cur = counts.get(key)
    if (cur) cur.count++
    else counts.set(key, { start: minutesToClock(a), end: minutesToClock(b), count: 1 })
  }
  return [...counts.values()]
    .sort((x, y) => y.count - x.count || x.start.localeCompare(y.start) || x.end.localeCompare(y.end))
    .slice(0, limit)
}

export function absenceLabel(a: Pick<Absence, 'absence_type' | 'half_day'>): string {
  const half = a.half_day ? ' ½' : ''
  switch (a.absence_type) {
    case 'vacation':
      return `Urlaub${half}`
    case 'sick':
      return `krank${half}`
    case 'compensation_day':
      return `Ausgleich${half}`
    default:
      return `abwesend${half}`
  }
}
