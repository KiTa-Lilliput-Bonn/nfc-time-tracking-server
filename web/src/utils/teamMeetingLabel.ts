import type { TeamMeeting } from '@/types/api'
import { shiftTimeRangeLabel } from '@/utils/scheduleShiftLayout'

export function teamMeetingBarTag(m: TeamMeeting): string {
  if (m.kind === 'other') {
    const label = (m.label ?? 'Sonstiges').trim()
    return label || 'Sonstiges'
  }
  return m.kind === 'kt' ? 'KT' : 'GT'
}

export function teamMeetingBarLabel(m: TeamMeeting): string {
  return `${teamMeetingBarTag(m)} ${shiftTimeRangeLabel(m.time_start, m.time_end)}`
}

export function teamMeetingListLabel(m: TeamMeeting, formatDate: (iso: string) => string): string {
  const tag = teamMeetingBarTag(m)
  return `${tag} ${formatDate(m.meeting_date)} ${m.time_start}–${m.time_end}`
}
