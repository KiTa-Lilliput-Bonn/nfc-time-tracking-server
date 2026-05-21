import type { APIRequestContext } from '@playwright/test'

import { authHeaders } from './auth'
import {
  E2E_MONTH,
  E2E_SCHEDULE_DATE,
  E2E_WEEK,
  E2E_WEEK_YEAR,
  E2E_WORK_DATE,
  E2E_YEAR,
} from './dates'

export interface SeededEmployee {
  id: number
  display_name: string
  username: string
}

export async function seedEmployee(
  request: APIRequestContext,
  token: string,
  opts: { username: string; display_name: string },
): Promise<SeededEmployee> {
  const res = await request.post('/api/v1/employees', {
    headers: authHeaders(token),
    data: {
      username: opts.username,
      display_name: opts.display_name,
      role: 'user',
    },
  })
  if (!res.ok()) {
    throw new Error(`create employee failed: ${res.status()} ${await res.text()}`)
  }
  const body = (await res.json()) as { user: { id: number; display_name: string; username: string } }
  return {
    id: body.user.id,
    display_name: body.user.display_name,
    username: body.user.username,
  }
}

export async function seedWeeklyHours(
  request: APIRequestContext,
  token: string,
  employeeId: number,
  hoursPerWeek: number,
  validFrom = '2020-01-01',
): Promise<void> {
  const res = await request.put(`/api/v1/employees/${employeeId}/weekly-hours`, {
    headers: authHeaders(token),
    data: { hours_per_week: hoursPerWeek, valid_from: validFrom },
  })
  if (!res.ok()) {
    throw new Error(`weekly hours failed: ${res.status()} ${await res.text()}`)
  }
}

export async function seedManualWorkPeriod(
  request: APIRequestContext,
  token: string,
  employeeId: number,
  workDate: string,
  punchIn: string,
  punchOut: string,
): Promise<{ id: number }> {
  const res = await request.post(`/api/v1/employees/${employeeId}/work-periods`, {
    headers: authHeaders(token),
    data: {
      work_date: workDate,
      punch_in: punchIn,
      punch_out: punchOut,
    },
  })
  if (!res.ok()) {
    throw new Error(`work period failed: ${res.status()} ${await res.text()}`)
  }
  const body = (await res.json()) as { id: number }
  return { id: body.id }
}

export async function seedScheduleShift(
  request: APIRequestContext,
  token: string,
  employeeId: number,
  scheduleDate: string,
  shiftStart: string,
  shiftEnd: string,
): Promise<void> {
  const res = await request.post('/api/v1/schedules', {
    headers: authHeaders(token),
    data: {
      user_id: employeeId,
      schedule_date: scheduleDate,
      shift_start: shiftStart,
      shift_end: shiftEnd,
    },
  })
  if (!res.ok()) {
    throw new Error(`schedule failed: ${res.status()} ${await res.text()}`)
  }
}

export async function fetchMonthBalance(
  request: APIRequestContext,
  token: string,
  employeeId: number,
  month = E2E_MONTH,
  year = E2E_YEAR,
): Promise<{ worked_hours: number; balance_hours: number }> {
  const res = await request.get(`/api/v1/employees/${employeeId}/balance`, {
    headers: authHeaders(token),
    params: { month: String(month), year: String(year) },
  })
  if (!res.ok()) {
    throw new Error(`balance failed: ${res.status()} ${await res.text()}`)
  }
  return (await res.json()) as { worked_hours: number; balance_hours: number }
}

export async function seedBalanceScenario(
  request: APIRequestContext,
  token: string,
  employeeId: number,
): Promise<{ worked_hours: number; balance_hours: number }> {
  await seedWeeklyHours(request, token, employeeId, 40)
  await seedManualWorkPeriod(
    request,
    token,
    employeeId,
    E2E_WORK_DATE,
    `${E2E_WORK_DATE}T08:00:00.000Z`,
    `${E2E_WORK_DATE}T16:00:00.000Z`,
  )
  return fetchMonthBalance(request, token, employeeId)
}

export async function seedEmployeeAbsence(
  request: APIRequestContext,
  token: string,
  employeeId: number,
  absenceDate: string,
  absenceType: string,
): Promise<void> {
  const res = await request.post(`/api/v1/employees/${employeeId}/absences`, {
    headers: authHeaders(token),
    data: { absence_date: absenceDate, absence_type: absenceType, half_day: false },
  })
  if (!res.ok()) {
    throw new Error(`absence failed: ${res.status()} ${await res.text()}`)
  }
}

export { E2E_SCHEDULE_DATE, E2E_WEEK, E2E_WEEK_YEAR }
