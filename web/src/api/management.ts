import { api } from '@/api/client'
import type {
  Absence,
  AndroidLanHealthStatus,
  AndroidLanSyncStampsRangeBody,
  AndroidLanSyncStampsRangeResult,
  ClosureDay,
  CompensationDayClaim,
  CompensationDayClaimStatus,
  Employee,
  HolidayCredit,
  Holiday,
  MonthBalance,
  NFCTag,
  Schedule,
  ScheduleExcelImportReport,
  TeamMeeting,
  TeamOverviewRow,
  TimeCorrection,
  VacationBalance,
  VacationEntitlement,
  WeeklyHours,
  WorkPeriod,
} from '@/types/api'

export async function createEmployee(body: {
  username: string
  display_name: string
  role?: string
}) {
  const { data } = await api.post<{ user: Employee; temporary_password: string }>('/employees', body)
  return data
}

export async function postEmployeeResetPassword(id: number) {
  const { data } = await api.post<{ user: Employee; temporary_password: string }>(
    `/employees/${id}/reset-password`,
  )
  return data
}

export async function patchEmployee(
  id: number,
  body: {
    display_name?: string
    active?: boolean
    role?: string
    opening_hours_balance?: number
    opening_vacation_days?: number
    /** Zuweisung ändern oder mit null entfernen */
    group_id?: number | null
    fixed_non_work_weekdays?: number[]
  },
) {
  const { data } = await api.patch<Employee>(`/employees/${id}`, body)
  return data
}

export async function fetchEmployeeSchedule(employeeId: number, from: string, to: string) {
  const { data } = await api.get<{ from: string; to: string; schedules: Schedule[] | null }>(
    `/employees/${employeeId}/schedule`,
    { params: { from, to } },
  )
  return {
    from: data.from,
    to: data.to,
    schedules: data.schedules ?? [],
  }
}

export async function fetchEmployeeTimes(employeeId: number, from: string, to: string) {
  const { data } = await api.get<{
    work_periods: WorkPeriod[] | null
    worked_hours: number
    holidays?: HolidayCredit[] | null
  }>(
    `/employees/${employeeId}/times`,
    { params: { from, to } },
  )
  // Go encodes nil slices as JSON null
  return {
    ...data,
    work_periods: data.work_periods ?? [],
    worked_hours: data.worked_hours ?? 0,
    holidays: data.holidays ?? [],
  }
}

export async function fetchEmployeeBalance(employeeId: number, month: number, year: number) {
  const { data } = await api.get<MonthBalance>(`/employees/${employeeId}/balance`, {
    params: { month, year },
  })
  return data
}

export async function fetchEmployeeVacation(employeeId: number) {
  const { data } = await api.get<VacationBalance>(`/employees/${employeeId}/vacation`)
  return data
}

export async function fetchEmployeeAbsences(employeeId: number, from: string, to: string) {
  const { data } = await api.get<{ absences: Absence[] | null }>(`/employees/${employeeId}/absences`, {
    params: { from, to },
  })
  return data.absences ?? []
}

export async function fetchEmployeeCorrections(employeeId: number, from: string, to: string) {
  const { data } = await api.get<{ corrections: TimeCorrection[] | null }>(
    `/employees/${employeeId}/corrections`,
    { params: { from, to } },
  )
  return data.corrections ?? []
}

export async function createEmployeeAbsence(
  employeeId: number,
  body: { absence_date: string; absence_type: string; half_day: boolean },
) {
  const { data } = await api.post<Absence>(`/employees/${employeeId}/absences`, body)
  return data
}

export async function deleteEmployeeAbsence(employeeId: number, absenceId: number) {
  await api.delete(`/employees/${employeeId}/absences/${absenceId}`)
}

export async function fetchEmployeeCompensationDayClaims(
  employeeId: number,
  status?: CompensationDayClaimStatus,
) {
  const { data } = await api.get<{ compensation_day_claims: CompensationDayClaim[] | null }>(
    `/employees/${employeeId}/compensation-day-claims`,
    { params: status ? { status } : undefined },
  )
  return data.compensation_day_claims ?? []
}

export async function waiveEmployeeCompensationDayClaim(employeeId: number, claimId: number) {
  await api.post(`/employees/${employeeId}/compensation-day-claims/${claimId}/waive`)
}

export async function createCorrection(
  employeeId: number,
  body: {
    work_period_id: number
    corrected_in: string
    corrected_out: string
    reason: string
  },
) {
  const { data } = await api.post<TimeCorrection>(`/employees/${employeeId}/corrections`, body)
  return data
}

export async function createManualWorkPeriod(
  employeeId: number,
  body: {
    work_date: string
    punch_in: string
    punch_out: string | null
    /** Ignoriert vom Server; manuelle Zeiten sind immer Arbeitsintervalle. */
    is_break?: boolean
  },
) {
  const { data } = await api.post<WorkPeriod>(`/employees/${employeeId}/work-periods`, body)
  return data
}

export async function deleteManualWorkPeriod(employeeId: number, wpId: number) {
  await api.delete(`/employees/${employeeId}/work-periods/${wpId}`)
}

export async function fetchWeeklyHours(employeeId: number) {
  const { data } = await api.get<{ weekly_hours: WeeklyHours[] | null }>(
    `/employees/${employeeId}/weekly-hours`,
  )
  return data.weekly_hours ?? []
}

export async function deleteWeeklyHours(employeeId: number, weeklyHoursId: number) {
  await api.delete(`/employees/${employeeId}/weekly-hours/${weeklyHoursId}`)
}

export async function putWeeklyHours(
  employeeId: number,
  body: { hours_per_week: number; valid_from: string },
) {
  const { data } = await api.put<WeeklyHours>(`/employees/${employeeId}/weekly-hours`, body)
  return data
}

export async function fetchVacationEntitlements(employeeId: number) {
  const { data } = await api.get<{ vacation_entitlements: VacationEntitlement[] }>(
    `/employees/${employeeId}/vacation-entitlement`,
  )
  return data.vacation_entitlements
}

export async function putVacationEntitlement(
  employeeId: number,
  body: { days_per_year: number; valid_from: string },
) {
  const { data } = await api.put<VacationEntitlement>(
    `/employees/${employeeId}/vacation-entitlement`,
    body,
  )
  return data
}

export async function deleteVacationEntitlement(employeeId: number, vacationEntitlementId: number) {
  await api.delete(`/employees/${employeeId}/vacation-entitlement/${vacationEntitlementId}`)
}

export async function fetchNFCTags(employeeId: number) {
  const { data } = await api.get<{ nfc_tags: NFCTag[] }>(`/employees/${employeeId}/nfc-tags`)
  return data.nfc_tags
}

export async function assignNFCTag(employeeId: number, body: { tag_uid: string; assigned_from: string }) {
  const { data } = await api.post<NFCTag>(`/employees/${employeeId}/nfc-tags`, body)
  return data
}

export async function fetchSchedulesForWeek(year: number, week: number) {
  const { data } = await api.get<{
    schedules: Schedule[]
    week_notes?: string
    absences?: Absence[] | null
    week_holidays?: Holiday[] | null
    team_meetings?: TeamMeeting[] | null
  }>('/schedules', {
    params: { year, week },
  })
  return {
    schedules: data.schedules ?? [],
    weekNotes: data.week_notes ?? '',
    absences: data.absences ?? [],
    weekHolidays: data.week_holidays ?? [],
    teamMeetings: data.team_meetings ?? [],
  }
}

export async function putTeamMeeting(
  id: number,
  body: { time_start: string; time_end: string; user_ids: number[] },
) {
  const { data } = await api.put<TeamMeeting>(`/team-meetings/${id}`, body)
  return data
}

export async function postTeamMeeting(body: {
  year: number
  week: number
  kind: 'kt' | 'gt'
  time_start: string
  time_end: string
  user_ids: number[]
}) {
  const { data } = await api.post<TeamMeeting>('/team-meetings', body)
  return data
}

export async function importScheduleExcel(file: File) {
  const fd = new FormData()
  fd.append('file', file)
  const { data } = await api.post<ScheduleExcelImportReport>('/schedules/import-excel', fd)
  return data
}

export interface ScheduleExportDefaults {
  start_year: number
  start_week: number
  end_year: number
  end_week: number
}

export async function fetchScheduleExportDefaults() {
  const { data } = await api.get<ScheduleExportDefaults>('/schedules/export-defaults')
  return data
}

export async function fetchScheduleExportExcel(
  fromYear: number,
  fromWeek: number,
  toYear: number,
  toWeek: number,
) {
  const { data } = await api.get<Blob>('/schedules/export-excel', {
    params: {
      from_year: fromYear,
      from_week: fromWeek,
      to_year: toYear,
      to_week: toWeek,
    },
    responseType: 'blob',
  })
  return data
}

export async function putScheduleWeekNotes(year: number, week: number, notes: string) {
  const { data } = await api.put<{ notes: string }>('/schedules/week-notes', { year, week, notes })
  return data.notes
}

export async function createSchedule(body: {
  user_id: number
  schedule_date: string
  shift_start: string
  shift_end: string
}) {
  const { data } = await api.post<Schedule>('/schedules', body)
  return data
}

export async function updateSchedule(
  id: number,
  body: { shift_start: string; shift_end: string },
) {
  const { data } = await api.put<Schedule>(`/schedules/${id}`, body)
  return data
}

export async function deleteSchedule(id: number) {
  await api.delete(`/schedules/${id}`)
}

export async function fetchClosureDays() {
  const { data } = await api.get<{ closure_days: ClosureDay[] | null }>('/closure-days')
  // Go encodes nil slices as JSON null
  return data.closure_days ?? []
}

/** Nur GET – Bearbeitung über Admin-Feiertage (Superadmin). */
export async function fetchHolidays(year: number) {
  const { data } = await api.get<{ holidays: Holiday[] | null }>('/holidays', { params: { year } })
  // Go encodes nil slices as JSON null
  return data.holidays ?? []
}

export async function createClosureDay(body: { closure_date: string; name: string }) {
  const { data } = await api.post<ClosureDay>('/closure-days', body)
  return data
}

export async function deleteClosureDay(id: number) {
  await api.delete(`/closure-days/${id}`)
}

export async function fetchTeamOverview(year: number) {
  const { data } = await api.get<{
    as_of: string
    vacation_year: number
    rows: TeamOverviewRow[] | null
  }>('/dashboard/team-overview', {
    params: { vacation_year: year },
  })
  return {
    as_of: data.as_of,
    vacation_year: data.vacation_year,
    rows: data.rows ?? [],
  }
}

/** Erreichbarkeit des Android-LAN-Geräts (Stamps); nur Leitung/Superadmin. */
export async function fetchAndroidLanHealthStatus(): Promise<AndroidLanHealthStatus> {
  const { data } = await api.get<AndroidLanHealthStatus>('/android-lan/health-status')
  return data
}

/** Manueller Stempel-Abgleich für einen Berlin-Kalendertagbereich (max. 14 Tage inklusive). */
export async function postAndroidLanSyncStampsRange(
  body: AndroidLanSyncStampsRangeBody,
): Promise<AndroidLanSyncStampsRangeResult> {
  const { data } = await api.post<AndroidLanSyncStampsRangeResult>('/android-lan/sync-stamps-range', body)
  return data
}
