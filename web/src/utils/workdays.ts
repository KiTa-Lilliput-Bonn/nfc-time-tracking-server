import type { Holiday } from '@/types/api'
import { addDays, parseISODate, toISODateLocal } from '@/utils/dates'

export function normalizeISODate(s: string): string {
  const m = String(s).match(/^(\d{4})-(\d{2})-(\d{2})/)
  return m ? `${m[1]}-${m[2]}-${m[3]}` : String(s)
}

/** Alle Kalendertage von–bis inklusive (YYYY-MM-DD lokal). */
export function enumerateInclusiveCalendarISO(from: Date, to: Date): string[] {
  const out: string[] = []
  let cur = new Date(from.getFullYear(), from.getMonth(), from.getDate())
  const end = new Date(to.getFullYear(), to.getMonth(), to.getDate())
  while (cur.getTime() <= end.getTime()) {
    out.push(toISODateLocal(cur))
    cur = addDays(cur, 1)
  }
  return out
}

/** Werktage (Mo–Fr) ohne Feiertage/Schließtage und ohne fixe freie Wochentage (Date.getDay(): 1=Mo … 5=Fr). */
export function enumerateWorkdayISO(
  from: Date,
  to: Date,
  holidaySet: Set<string>,
  closureSet: Set<string>,
  fixedNonWorkWeekdays?: Set<number>,
): string[] {
  const fixed = fixedNonWorkWeekdays ?? new Set<number>()
  const out: string[] = []
  let cur = new Date(from.getFullYear(), from.getMonth(), from.getDate())
  const end = new Date(to.getFullYear(), to.getMonth(), to.getDate())
  while (cur.getTime() <= end.getTime()) {
    const dow = cur.getDay()
    const iso = toISODateLocal(cur)
    if (
      dow !== 0 &&
      dow !== 6 &&
      !holidaySet.has(iso) &&
      !closureSet.has(iso) &&
      !fixed.has(dow)
    ) {
      out.push(iso)
    }
    cur = addDays(cur, 1)
  }
  return out
}

/**
 * True, wenn jeder Kalendertag strikt zwischen zwei Urlaubstagen nur aus Wochenende,
 * Feiertag/Schließtag oder fix frei (Mo–Fr, Date.getDay()) besteht — dann kann die
 * Abwesenheitsübersicht die Tage als einen Urlaubszeitraum anzeigen.
 */
export function vacationDisplayGapOnlySkippable(
  lastVacationISO: string,
  nextVacationISO: string,
  holidayOrClosure: Set<string>,
  fixedNonWorkWeekdays: Set<number>,
): boolean {
  if (nextVacationISO <= lastVacationISO) return false
  const end = parseISODate(nextVacationISO)
  const d = parseISODate(lastVacationISO)
  d.setDate(d.getDate() + 1)
  while (d < end) {
    const iso = toISODateLocal(d)
    const dow = d.getDay()
    const weekend = dow === 0 || dow === 6
    const fixedFree = dow >= 1 && dow <= 5 && fixedNonWorkWeekdays.has(dow)
    if (!weekend && !holidayOrClosure.has(iso) && !fixedFree) return false
    d.setDate(d.getDate() + 1)
  }
  return true
}

export function countSkippedNonWorkdays(
  allDatesISO: string[],
  holidaySet: Set<string>,
  closureSet: Set<string>,
  fixedNonWorkWeekdays?: Set<number>,
): number {
  const fixed = fixedNonWorkWeekdays ?? new Set<number>()
  let n = 0
  for (const iso of allDatesISO) {
    const d = new Date(`${iso}T00:00:00`)
    const dow = d.getDay()
    if (dow === 0 || dow === 6 || holidaySet.has(iso) || closureSet.has(iso) || fixed.has(dow)) n++
  }
  return n
}

export async function holidayDateSetForRange(
  from: Date,
  to: Date,
  fetchHolidays: (year: number) => Promise<Holiday[]>,
): Promise<Set<string>> {
  const minY = Math.min(from.getFullYear(), to.getFullYear())
  const maxY = Math.max(from.getFullYear(), to.getFullYear())
  const set = new Set<string>()
  for (let y = minY; y <= maxY; y++) {
    const hol = await fetchHolidays(y)
    for (const h of hol) set.add(normalizeISODate(h.holiday_date))
  }
  return set
}

