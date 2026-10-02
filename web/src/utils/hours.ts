/** Anzeige von Stunden/Tagen für Mitarbeitende: Stunden als „h:mm“, Tage mit deutschem Komma. */

/** 7.5 → „7:30 h“, -1.25 → „−1:15 h“; signed=true ergänzt „+“ bei positiven Werten. */
export function formatHoursHM(hours: number, opts: { signed?: boolean } = {}): string {
  if (!Number.isFinite(hours)) return '—'
  const totalMin = Math.round(Math.abs(hours) * 60)
  const h = Math.floor(totalMin / 60)
  const m = totalMin % 60
  const sign = hours < 0 && totalMin > 0 ? '−' : opts.signed && totalMin > 0 ? '+' : ''
  return `${sign}${h}:${String(m).padStart(2, '0')} h`
}

/** Minuten → „7:30 h“. */
export function formatMinutesHM(minutes: number): string {
  return formatHoursHM(minutes / 60)
}

/** 26.5 → „26,5“ */
export function formatDays(days: number): string {
  if (!Number.isFinite(days)) return '—'
  return days.toLocaleString('de-DE', { minimumFractionDigits: 0, maximumFractionDigits: 1 })
}

/** CSS-Klasse für Saldo-Werte. */
export function balanceClass(hours: number): string {
  if (Math.round(hours * 60) === 0) return ''
  return hours > 0 ? 'pos' : 'neg'
}
