import type { ChangeRequest, ChangeRequestStatus } from '@/types/api'
import { formatGermanDate, formatGermanTime } from '@/utils/dates'

export const requestKindLabel: Record<ChangeRequest['kind'], string> = {
  time_correction: 'Zeitkorrektur',
  time_entry: 'Zeit nachtragen',
  vacation: 'Urlaub',
}

export const requestStatusLabel: Record<ChangeRequestStatus, string> = {
  pending: 'Offen',
  approved: 'Genehmigt',
  rejected: 'Abgelehnt',
  withdrawn: 'Zurückgezogen',
}

export const requestStatusSeverity: Record<ChangeRequestStatus, 'warn' | 'success' | 'danger' | 'secondary'> = {
  pending: 'warn',
  approved: 'success',
  rejected: 'danger',
  withdrawn: 'secondary',
}

function span(a?: string | null, b?: string | null): string {
  return `${a ? formatGermanTime(a) : '—'}–${b ? formatGermanTime(b) : '—'}`
}

function formatDays(n: number): string {
  return n.toLocaleString('de-DE', { maximumFractionDigits: 1 })
}

/** Kurzbeschreibung, was der Antrag ändert (eine Zeile). */
export function requestSummary(r: ChangeRequest): string {
  switch (r.kind) {
    case 'time_correction':
      return `${formatGermanDate(r.work_date ?? '')}: ${span(r.original_in, r.original_out)} → ${span(r.punch_in, r.punch_out)}`
    case 'time_entry':
      return `${formatGermanDate(r.work_date ?? '')}: ${span(r.punch_in, r.punch_out)} nachtragen`
    case 'vacation': {
      const range =
        r.date_from === r.date_to
          ? formatGermanDate(r.date_from ?? '')
          : `${formatGermanDate(r.date_from ?? '')}–${formatGermanDate(r.date_to ?? '')}`
      const days = r.vacation_days != null ? ` · ${formatDays(r.vacation_days)} ${r.vacation_days === 1 ? 'Tag' : 'Tage'}` : ''
      return `${range}${r.half_day ? ' (halber Tag)' : ''}${days}`
    }
  }
}
