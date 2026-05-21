const ISO_YMD_PREFIX = /^(\d{4})-(\d{2})-(\d{2})(?:$|[T\s])/

/** Kalendertag zur Anzeige: dd.mm.yyyy (API liefert meist YYYY-MM-DD). */
export function formatGermanDate(s: string): string {
  const m = s.match(ISO_YMD_PREFIX)
  if (m) return `${m[3]}.${m[2]}.${m[1]}`
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return s
  const dd = String(d.getDate()).padStart(2, '0')
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const yyyy = d.getFullYear()
  return `${dd}.${mm}.${yyyy}`
}

/** Zeitstempel zur Anzeige: dd.mm.yyyy HH:mm (24 h, lokale Zeitzone). */
export function formatGermanDateTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const dd = String(d.getDate()).padStart(2, '0')
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const yyyy = d.getFullYear()
  const hh = String(d.getHours()).padStart(2, '0')
  const mi = String(d.getMinutes()).padStart(2, '0')
  return `${dd}.${mm}.${yyyy} ${hh}:${mi}`
}

/** Uhrzeit zur Anzeige: HH:mm (24 h, lokale Zeitzone). */
export function formatGermanTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const hh = String(d.getHours()).padStart(2, '0')
  const mi = String(d.getMinutes()).padStart(2, '0')
  return `${hh}:${mi}`
}

const pad2 = (n: number) => String(n).padStart(2, '0')

/** Lokalen HH:mm-String für time-Input aus einem ISO-Zeitstempel. */
export function isoToTimeInputValue(iso: string, fallback = '08:00'): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return fallback
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
}

/**
 * Parst Werte von HTML time-Inputs ("HH:MM" / "HH:MM:SS") und tolerante freie Eingabe "H:MM" / "HH".
 */
export function parseTimeHHMM(s: string): { h: number; m: number } | null {
  const t = s.trim()
  if (!t) return null
  if (/^\d{1,2}$/.test(t)) {
    const h = Number(t)
    if (h < 0 || h > 23) return null
    return { h, m: 0 }
  }
  const parts = t.split(':').map((p) => p.trim().replace(/\D/g, ''))
  if (parts.length < 2) return null
  const h = Number.parseInt(parts[0]!, 10)
  const mm = Number.parseInt(parts[1]!.slice(0, 2), 10) || 0
  if (Number.isNaN(h) || Number.isNaN(mm)) return null
  if (h < 0 || h > 23 || mm < 0 || mm > 59) return null
  return { h, m: mm }
}

/**
 * Kalendertag aus work_date (YYYY-MM-DD oder mit Zeitanteil, z. B. …T00:00:00Z).
 * Wichtig: Nicht per split("-") mappen (liefert bei "…-15T12:00:00" NaN für den Tag).
 */
export function parseWorkDateYMD(s: string | undefined | null): { y: number; m: number; d: number } | null {
  if (s == null || String(s).trim() === '') return null
  const t = String(s)
  const m = t.match(/^(\d{4})-(\d{1,2})-(\d{1,2})/)
  if (!m) return null
  const y = Number(m[1])
  const mo = Number(m[2])
  const d0 = Number(m[3])
  if (!Number.isFinite(y) || !Number.isFinite(mo) || !Number.isFinite(d0)) return null
  if (mo < 1 || mo > 12 || d0 < 1 || d0 > 31) return null
  const x = new Date(y, mo - 1, d0)
  if (Number.isNaN(x.getTime()) || x.getFullYear() !== y || x.getMonth() !== mo - 1 || x.getDate() !== d0) {
    return null
  }
  return { y, m: mo, d: d0 }
}

/**
 * ISO corrected_in / corrected_out: am selben Kalendertag muss Gehen (Uhr) strikt nach Kommen (Uhr) liegen.
 */
export function buildCorrectionTimeInstants(
  workDate: string,
  inHHMM: string,
  outHHMM: string,
): { corrected_in: string; corrected_out: string } | null {
  const a = parseTimeHHMM(inHHMM)
  const b = parseTimeHHMM(outHHMM)
  if (!a || !b) return null
  const cal = parseWorkDateYMD(workDate)
  if (!cal) return null
  const { y, m, d: day } = cal
  const tIn = new Date(y, m - 1, day, a.h, a.m, 0, 0)
  const tOutSameDay = new Date(y, m - 1, day, b.h, b.m, 0, 0)
  if (Number.isNaN(tIn.getTime()) || Number.isNaN(tOutSameDay.getTime())) return null
  if (tOutSameDay.getTime() === tIn.getTime()) return null
  if (tOutSameDay.getTime() <= tIn.getTime()) return null
  return { corrected_in: tIn.toISOString(), corrected_out: tOutSameDay.toISOString() }
}

/** YYYY-MM-DD in local calendar */
export function toISODateLocal(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

export function parseISODate(s: string): Date {
  const [y, m, d] = s.split('-').map(Number)
  return new Date(y, m - 1, d)
}

/** Lokaler Zeitpunkt aus Kalendertag (YYYY-MM-DD) und Uhrzeit HH:mm → ISO-String (wie bei Korrekturen). */
export function localInstantISOFromDateAndClock(workDate: string, hhmm: string): string | null {
  const p = parseTimeHHMM(hhmm)
  const cal = parseWorkDateYMD(workDate)
  if (!p || !cal) return null
  const { y, m, d } = cal
  return new Date(y, m - 1, d, p.h, p.m, 0, 0).toISOString()
}

/** Späterer der beiden ISO-Zeitstempel (für effektiven Beginn = max(Stempel, Schichtbeginn)). */
export function maxInstantISO(a: string, b: string): string {
  return new Date(a).getTime() >= new Date(b).getTime() ? a : b
}

export function startOfMonth(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), 1)
}

export function endOfMonth(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth() + 1, 0)
}

/** Monday-based ISO week: get Monday of the week containing `d` */
export function startOfISOWeek(d: Date): Date {
  const x = new Date(d)
  const day = x.getDay()
  const diff = day === 0 ? -6 : 1 - day
  x.setDate(x.getDate() + diff)
  x.setHours(0, 0, 0, 0)
  return x
}

export function endOfISOWeek(d: Date): Date {
  const mon = startOfISOWeek(d)
  const sun = new Date(mon)
  sun.setDate(sun.getDate() + 6)
  return sun
}

export function addDays(d: Date, n: number): Date {
  const x = new Date(d)
  x.setDate(x.getDate() + n)
  return x
}

/** ISO week number (1–53) and ISO week-year for a local calendar date */
export function isoWeekAndYear(d: Date): { year: number; week: number } {
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

/** Local-date Monday of ISO week `week` in ISO year `isoYear` */
export function mondayOfISOWeek(isoYear: number, week: number): Date {
  const jan4 = new Date(isoYear, 0, 4)
  const day = (jan4.getDay() + 6) % 7
  const w1 = new Date(jan4)
  w1.setDate(jan4.getDate() - day)
  const mon = new Date(w1)
  mon.setDate(w1.getDate() + (week - 1) * 7)
  return mon
}

export function shiftISOWeek(isoYear: number, week: number, deltaWeeks: number): { year: number; week: number } {
  const mon = mondayOfISOWeek(isoYear, week)
  mon.setDate(mon.getDate() + deltaWeeks * 7)
  return isoWeekAndYear(mon)
}

/** -1 if a before b, 0 if equal, 1 if a after b (ISO week-year + KW). */
export function compareISOWeek(
  a: { year: number; week: number },
  b: { year: number; week: number },
): number {
  const aMon = mondayOfISOWeek(a.year, a.week).getTime()
  const bMon = mondayOfISOWeek(b.year, b.week).getTime()
  if (aMon < bMon) return -1
  if (aMon > bMon) return 1
  return 0
}
