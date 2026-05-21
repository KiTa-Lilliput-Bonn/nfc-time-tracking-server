import { formatGermanDate } from '@/utils/dates'

function looksLikeAbsenceDuplicate(msg: string): boolean {
  const m = msg.toLowerCase()
  // SQLite-Formulierung variiert; ausreichend: Unique auf Tabelle absences.
  return m.includes('unique constraint failed') && m.includes('absences')
}

export function friendlyAbsenceCreateError(params: {
  apiMessage?: string
  isoDate: string
  absenceLabel: string
}): string | undefined {
  const msg = params.apiMessage?.trim()
  if (!msg) return undefined

  if (looksLikeAbsenceDuplicate(msg)) {
    return `Für ${formatGermanDate(params.isoDate)} ist bereits eine Abwesenheit eingetragen.`
  }

  // Fallback to original backend/API message.
  return msg
}

