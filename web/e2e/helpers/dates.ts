/**
 * Rolling anchors for the current ISO week (Playwright: timezoneId UTC, TZ=UTC).
 * Keeps seeded API data inside the default UI week filter (Mon–Sun).
 */
function pad2(n: number): string {
  return String(n).padStart(2, '0')
}

function toIsoDateLocal(d: Date): string {
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`
}

function startOfISOWeek(d: Date): Date {
  const x = new Date(d)
  const day = x.getDay()
  const diff = day === 0 ? -6 : 1 - day
  x.setDate(x.getDate() + diff)
  x.setHours(0, 0, 0, 0)
  return x
}

function isoWeekAndYear(d: Date): { year: number; week: number } {
  const x = new Date(d.getFullYear(), d.getMonth(), d.getDate())
  const day = (x.getDay() + 6) % 7
  x.setDate(x.getDate() - day + 3)
  const isoYear = x.getFullYear()
  const jan4 = new Date(isoYear, 0, 4)
  const jan4Day = (jan4.getDay() + 6) % 7
  const week1Mon = new Date(jan4)
  week1Mon.setDate(jan4.getDate() - jan4Day)
  const diff = Math.round((x.getTime() - week1Mon.getTime()) / 86400000)
  const week = 1 + Math.floor(diff / 7)
  return { year: isoYear, week }
}

const today = new Date()
today.setHours(0, 0, 0, 0)
const weekMonday = startOfISOWeek(today)
/** Latest day in the current ISO week that is not after today (API rejects future work_date). */
const anchor = today.getTime() < weekMonday.getTime() ? weekMonday : today
const { year: isoYear, week: isoWeek } = isoWeekAndYear(anchor)

export const E2E_YEAR = anchor.getFullYear()
export const E2E_MONTH = anchor.getMonth() + 1
export const E2E_WORK_DATE = toIsoDateLocal(anchor)
const gapAnchor = new Date(anchor)
gapAnchor.setDate(gapAnchor.getDate() - 1)
/** Gestern (UTC): für Dienstplan-Lücken (nur Tage vor heute). */
export const E2E_GAP_DATE = toIsoDateLocal(gapAnchor)
export const E2E_ABSENCE_DATE = E2E_WORK_DATE
export const E2E_SCHEDULE_DATE = E2E_WORK_DATE
export const E2E_WEEK_YEAR = isoYear
export const E2E_WEEK = isoWeek

export function germanDateFromIso(iso: string): string {
  const [y, m, d] = iso.split('-')
  return `${d}.${m}.${y}`
}
