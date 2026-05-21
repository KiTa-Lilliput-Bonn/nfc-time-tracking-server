import { SCHEDULE_TIMELINE_END_H, SCHEDULE_TIMELINE_START_H } from '@/utils/scheduleShiftLayout'

/** Raster für Ziehen/Klicken im Dienstplan (Minuten). */
export const SCHEDULE_SNAP_MINUTES = 15
export const SCHEDULE_MIN_DURATION_MIN = 15
/** Dauer beim Klick auf leere Spur (Minuten). */
export const SCHEDULE_PLACE_DURATION_MIN = 180

export function gridBoundsMinutes(
  gridStartH = SCHEDULE_TIMELINE_START_H,
  gridEndH = SCHEDULE_TIMELINE_END_H,
): { g0: number; g1: number } {
  return { g0: gridStartH * 60, g1: gridEndH * 60 }
}

/** `HH:MM` → Minuten seit Mitternacht; ungültig → `null`. */
export function parseClockToMinutes(clock: string): number | null {
  const t = clock.trim()
  const m = t.match(/^(\d{1,2}):(\d{2})/)
  if (!m) return null
  const h = Number.parseInt(m[1]!, 10)
  const min = Number.parseInt(m[2]!, 10)
  if (!Number.isFinite(h) || !Number.isFinite(min) || min < 0 || min > 59) return null
  return h * 60 + min
}

export function minutesToHHMM(totalMinutes: number): string {
  const m = Math.max(0, Math.floor(totalMinutes))
  const h = Math.floor(m / 60)
  const min = m % 60
  return `${String(h).padStart(2, '0')}:${String(min).padStart(2, '0')}`
}

/** Nächstes Vielfaches von `step` ab `g0` (auf ganze Minuten). */
export function snapMinutesNearestOnGrid(
  minutesAbs: number,
  g0: number,
  g1: number,
  step = SCHEDULE_SNAP_MINUTES,
): number {
  const snapped = Math.round((minutesAbs - g0) / step) * step + g0
  return Math.max(g0, Math.min(g1, snapped))
}

/**
 * Horizontale Mausposition → Minuten im sichtbaren Raster [g0, g1] (linear, ohne Snap).
 */
export function clientXToTimelineMinutes(
  rect: DOMRectReadOnly,
  clientX: number,
  gridStartH = SCHEDULE_TIMELINE_START_H,
  gridEndH = SCHEDULE_TIMELINE_END_H,
): number {
  const { g0, g1 } = gridBoundsMinutes(gridStartH, gridEndH)
  const w = rect.width
  if (w <= 0) return g0
  const t = (clientX - rect.left) / w
  const clamped = Math.max(0, Math.min(1, t))
  return g0 + clamped * (g1 - g0)
}

export function readShiftIntervalMinutes(
  shiftStart: string,
  shiftEnd: string,
  gridStartH = SCHEDULE_TIMELINE_START_H,
  gridEndH = SCHEDULE_TIMELINE_END_H,
): { startMin: number; endMin: number } | null {
  const a = parseClockToMinutes(shiftStart)
  const b = parseClockToMinutes(shiftEnd)
  if (a == null || b == null || b <= a) return null
  const { g0, g1 } = gridBoundsMinutes(gridStartH, gridEndH)
  if (a >= g1 || b <= g0) return null
  return { startMin: a, endMin: b }
}

/**
 * Leere Spur: 3 h ab gesnapptem Klickpunkt, in [g0, g1] begrenzt (Start nach links schieben, falls nötig).
 */
export function placeThreeHourShiftMinutes(
  clickedMinutesAbs: number,
  gridStartH = SCHEDULE_TIMELINE_START_H,
  gridEndH = SCHEDULE_TIMELINE_END_H,
): { startMin: number; endMin: number } | null {
  const { g0, g1 } = gridBoundsMinutes(gridStartH, gridEndH)
  const span = g1 - g0
  if (span < SCHEDULE_MIN_DURATION_MIN) return null
  const dur = Math.min(SCHEDULE_PLACE_DURATION_MIN, span)
  const m = Math.max(g0, Math.min(g1, clickedMinutesAbs))
  let start = snapMinutesNearestOnGrid(m, g0, g1)
  start = Math.min(start, g1 - dur)
  start = Math.max(g0, Math.round((start - g0) / SCHEDULE_SNAP_MINUTES) * SCHEDULE_SNAP_MINUTES + g0)
  start = Math.min(start, g1 - dur)
  let end = start + dur
  if (end > g1) {
    end = g1
    start = Math.max(g0, end - dur)
    start = Math.max(g0, Math.round((start - g0) / SCHEDULE_SNAP_MINUTES) * SCHEDULE_SNAP_MINUTES + g0)
    end = Math.min(g1, start + dur)
  }
  if (end - start < SCHEDULE_MIN_DURATION_MIN) return null
  return { startMin: start, endMin: end }
}

/** Schichtblock verschieben: Delta aus Zeiger vs. Anker, dann clamp + 15-Min-Raster. */
export function clampMoveShift(
  origStartMin: number,
  origEndMin: number,
  pointerClientX: number,
  anchorClientX: number,
  rect: DOMRectReadOnly,
  gridStartH = SCHEDULE_TIMELINE_START_H,
  gridEndH = SCHEDULE_TIMELINE_END_H,
): { startMin: number; endMin: number } {
  const { g0, g1 } = gridBoundsMinutes(gridStartH, gridEndH)
  const pm = clientXToTimelineMinutes(rect, pointerClientX, gridStartH, gridEndH)
  const am = clientXToTimelineMinutes(rect, anchorClientX, gridStartH, gridEndH)
  const rawDelta = pm - am
  let s = origStartMin + rawDelta
  let e = origEndMin + rawDelta
  if (s < g0) {
    const d = g0 - s
    s += d
    e += d
  }
  if (e > g1) {
    const d = e - g1
    s -= d
    e -= d
  }
  const dur = origEndMin - origStartMin
  s = Math.round((s - g0) / SCHEDULE_SNAP_MINUTES) * SCHEDULE_SNAP_MINUTES + g0
  e = s + dur
  if (e > g1) {
    s = g1 - dur
    e = g1
  }
  if (s < g0) {
    s = g0
    e = s + dur
  }
  if (e > g1) e = g1
  if (e - s < SCHEDULE_MIN_DURATION_MIN) {
    return { startMin: origStartMin, endMin: origEndMin }
  }
  return { startMin: s, endMin: e }
}

export function clampResizeStart(
  fixedEndMin: number,
  pointerClientX: number,
  rect: DOMRectReadOnly,
  gridStartH = SCHEDULE_TIMELINE_START_H,
  gridEndH = SCHEDULE_TIMELINE_END_H,
): number {
  const { g0, g1 } = gridBoundsMinutes(gridStartH, gridEndH)
  const pm = clientXToTimelineMinutes(rect, pointerClientX, gridStartH, gridEndH)
  let ns = snapMinutesNearestOnGrid(pm, g0, g1)
  ns = Math.min(ns, fixedEndMin - SCHEDULE_MIN_DURATION_MIN)
  ns = Math.max(ns, g0)
  ns = Math.round((ns - g0) / SCHEDULE_SNAP_MINUTES) * SCHEDULE_SNAP_MINUTES + g0
  ns = Math.min(ns, fixedEndMin - SCHEDULE_MIN_DURATION_MIN)
  return Math.max(g0, ns)
}

export function clampResizeEnd(
  fixedStartMin: number,
  pointerClientX: number,
  rect: DOMRectReadOnly,
  gridStartH = SCHEDULE_TIMELINE_START_H,
  gridEndH = SCHEDULE_TIMELINE_END_H,
): number {
  const { g0, g1 } = gridBoundsMinutes(gridStartH, gridEndH)
  const pm = clientXToTimelineMinutes(rect, pointerClientX, gridStartH, gridEndH)
  let ne = snapMinutesNearestOnGrid(pm, g0, g1)
  ne = Math.max(ne, fixedStartMin + SCHEDULE_MIN_DURATION_MIN)
  ne = Math.min(ne, g1)
  ne = Math.round((ne - g0) / SCHEDULE_SNAP_MINUTES) * SCHEDULE_SNAP_MINUTES + g0
  ne = Math.max(ne, fixedStartMin + SCHEDULE_MIN_DURATION_MIN)
  return Math.min(g1, ne)
}
