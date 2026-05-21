import { api } from '@/api/client'
import type {
  Absence,
  ClosureDay,
  HolidayCredit,
  MonthBalance,
  Schedule,
  TeamMeeting,
  TimeCorrection,
  VacationBalance,
  WorkPeriod,
} from '@/types/api'

export interface MeTimesResponse {
  from: string
  to: string
  work_periods: WorkPeriod[] | null
  worked_hours: number
  holidays?: HolidayCredit[] | null
}

export interface MeScheduleResponse {
  from: string
  to: string
  schedules: Schedule[] | null
  team_meetings?: TeamMeeting[] | null
}

export interface MeAbsencesResponse {
  from: string
  to: string
  absences: Absence[] | null
}

export interface MeCorrectionsResponse {
  from: string
  to: string
  corrections: TimeCorrection[] | null
}

export async function fetchMeTimes(from: string, to: string) {
  const { data } = await api.get<MeTimesResponse>('/me/times', { params: { from, to } })
  return {
    ...data,
    work_periods: data.work_periods ?? [],
    holidays: data.holidays ?? [],
  }
}

export async function fetchMeBalance(month: number, year: number) {
  const { data } = await api.get<MonthBalance>('/me/balance', { params: { month, year } })
  return data
}

export async function fetchMeVacation() {
  const { data } = await api.get<VacationBalance>('/me/vacation')
  return data
}

export async function fetchMeProfile() {
  const { data } = await api.get<{ fixed_non_work_weekdays?: number[] }>('/me/profile')
  return data
}

/** Schließtage (GET erlaubt für alle eingeloggten Nutzer). */
export async function fetchClosureDaysForMe() {
  const { data } = await api.get<{ closure_days: ClosureDay[] | null }>('/closure-days')
  return data.closure_days ?? []
}

export async function fetchMeSchedule(from: string, to: string) {
  const { data } = await api.get<MeScheduleResponse>('/me/schedule', { params: { from, to } })
  return {
    ...data,
    schedules: data.schedules ?? [],
    team_meetings: data.team_meetings ?? [],
  }
}

export async function fetchMeAbsences(from: string, to: string) {
  const { data } = await api.get<MeAbsencesResponse>('/me/absences', { params: { from, to } })
  return {
    ...data,
    absences: data.absences ?? [],
  }
}

export async function fetchMeCorrections(from: string, to: string) {
  const { data } = await api.get<MeCorrectionsResponse>('/me/corrections', { params: { from, to } })
  return {
    ...data,
    corrections: data.corrections ?? [],
  }
}

export async function createMeCorrection(body: {
  work_period_id: number
  corrected_in: string
  corrected_out: string
  reason: string
}) {
  const { data } = await api.post<TimeCorrection>('/me/corrections', body)
  return data
}
