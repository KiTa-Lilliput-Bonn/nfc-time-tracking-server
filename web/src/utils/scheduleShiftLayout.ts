/** Sichtbares Zeitfenster für Dienstplan-Timeline (Stunden, Endstunde exklusiv). */
export const SCHEDULE_TIMELINE_START_H = 6
export const SCHEDULE_TIMELINE_END_H = 20

export function timelineHourLabels(gridStartH: number, gridEndH: number): number[] {
  const out: number[] = []
  for (let h = gridStartH; h < gridEndH; h++) out.push(h)
  return out
}

/** Anzeige HH:MM aus Dienstplan-Feldern (ohne Datum). */
export function normalizeShiftClock(s: string): string {
  const t = s.trim()
  const m = t.match(/^(\d{1,2}):(\d{2})/)
  if (m) {
    const h = Number.parseInt(m[1]!, 10)
    const min = m[2]!
    if (Number.isFinite(h)) return `${String(h).padStart(2, '0')}:${min}`
  }
  return t
}

/**
 * Horizontale Balkenposition im Raster [gridStartH, gridEndH] in Prozent der Track-Breite.
 * Schicht wird auf das Raster geklemmt; bei ungültigen Zeiten `null`.
 */
export function horizontalBarPercentages(
  _workDate: string,
  shiftStart: string,
  shiftEnd: string,
  gridStartH = SCHEDULE_TIMELINE_START_H,
  gridEndH = SCHEDULE_TIMELINE_END_H,
  opts?: { minWidthPct?: number },
): { leftPct: number; widthPct: number } | null {
  const a = shiftStart.trim()
  const b = shiftEnd.trim()
  if (!a || !b) return null
  const partsA = a.split(':').map((x) => Number.parseInt(x.replace(/\D/g, ''), 10))
  const partsB = b.split(':').map((x) => Number.parseInt(x.replace(/\D/g, ''), 10))
  if (partsA.length < 2 || partsB.length < 2) return null
  const inM = partsA[0]! * 60 + (partsA[1] ?? 0)
  const outM = partsB[0]! * 60 + (partsB[1] ?? 0)
  if (outM <= inM) return null

  const g0 = gridStartH * 60
  const g1 = gridEndH * 60
  const gridMinutes = g1 - g0
  if (gridMinutes <= 0) return null

  const s = Math.max(g0, Math.min(g1, inM))
  const e = Math.max(g0, Math.min(g1, outM))
  if (e <= s) return null

  const dur = e - s
  const minWidthPct = opts?.minWidthPct ?? 0.75
  let widthPct = (dur / gridMinutes) * 100
  const leftPct = ((s - g0) / gridMinutes) * 100
  if (widthPct < minWidthPct) widthPct = minWidthPct
  return { leftPct, widthPct }
}

export function shiftTimeRangeLabel(shiftStart: string, shiftEnd: string): string | null {
  const a = shiftStart.trim()
  const b = shiftEnd.trim()
  if (!a || !b) return null
  return `${normalizeShiftClock(a)}–${normalizeShiftClock(b)}`
}
