import { test, expect, type APIRequestContext, type Page } from '@playwright/test'

import { authHeaders } from './helpers/auth'
import { E2E_ADMIN_PASSWORD, E2E_ADMIN_USER } from './helpers/env'
import { E2E_WORK_DATE } from './helpers/dates'
import { seedManualWorkPeriod } from './helpers/seed'
import { uniqueLabel } from './helpers/ui'

interface SessionUser {
  id: number
  username: string
  display_name: string
  role: string
  must_change_password: boolean
}

interface Session {
  token: string
  user: SessionUser
}

let admin: Session

async function apiSession(request: APIRequestContext, username: string, password: string): Promise<Session> {
  const res = await request.post('/api/v1/auth/login', { data: { username, password } })
  expect(res.ok(), await res.text()).toBeTruthy()
  const body = (await res.json()) as Session
  return { token: body.token, user: { ...body.user, must_change_password: false } }
}

/** Legt Mitarbeitende an und meldet sie per API an (ein Login, wegen Login-Rate-Limit pro IP). */
async function seedEmployeeSession(request: APIRequestContext, displayName: string): Promise<Session> {
  const username = `e2e.req.${Date.now()}`
  const res = await request.post('/api/v1/employees', {
    headers: authHeaders(admin.token),
    data: { username, display_name: displayName, role: 'user' },
  })
  expect(res.ok()).toBeTruthy()
  const body = (await res.json()) as { temporary_password: string }
  return apiSession(request, username, body.temporary_password)
}

/** Übernimmt eine API-Sitzung in den Browser, ohne UI-Login. */
async function useSession(page: Page, s: Session, path: string) {
  await page.goto('/login')
  await page.evaluate((sess) => {
    localStorage.setItem('nfc_token', sess.token)
    localStorage.setItem('nfc_user', JSON.stringify(sess.user))
  }, s)
  await page.goto(path)
}

test.beforeAll(async ({ request }) => {
  admin = await apiSession(request, E2E_ADMIN_USER, E2E_ADMIN_PASSWORD)
})

test('employee time correction needs Leitung approval; rejection needs a comment', async ({ page, request }) => {
  const adminToken = admin.token
  const displayName = uniqueLabel('E2E Antrag')
  const emp = await seedEmployeeSession(request, displayName)
  const wp = await seedManualWorkPeriod(
    request,
    adminToken,
    emp.user.id,
    E2E_WORK_DATE,
    `${E2E_WORK_DATE}T08:00:00.000Z`,
    `${E2E_WORK_DATE}T16:00:00.000Z`,
  )

  // Der frühere Direktweg ist zu.
  const direct = await request.post('/api/v1/me/corrections', {
    headers: authHeaders(emp.token),
    data: { work_period_id: wp.id, corrected_in: `${E2E_WORK_DATE}T07:00:00.000Z`, corrected_out: `${E2E_WORK_DATE}T16:00:00.000Z`, reason: 'x' },
  })
  expect(direct.ok()).toBeFalsy()

  await useSession(page, emp, '/my/requests')
  await page.locator('#rq-date').fill(E2E_WORK_DATE)
  await expect(page.locator('.period').first()).toContainText('08:00')
  await page.locator('#rq-in').fill('07:30')
  await page.locator('#rq-reason').fill('Stempeln vergessen')
  await page.getByTestId('request-submit').click()
  await expect(page.getByTestId('my-request').first()).toContainText('Offen')
  await expect(page.getByTestId('my-request').first()).toContainText('08:00–16:00 → 07:30–16:00')

  // Noch nicht wirksam.
  let corr = await request.get(`/api/v1/employees/${emp.user.id}/corrections`, {
    headers: authHeaders(adminToken),
    params: { from: E2E_WORK_DATE, to: E2E_WORK_DATE },
  })
  expect(((await corr.json()) as { corrections: unknown[] | null }).corrections ?? []).toHaveLength(0)

  await useSession(page, admin, '/requests')
  await expect(page.getByTestId('pending-requests-badge')).toBeVisible()
  const card = page.getByTestId('leitung-request').filter({ hasText: displayName })
  await card.getByTestId('request-reject').click()
  await expect(page.getByTestId('reject-confirm')).toBeDisabled()
  await page.getByTestId('reject-comment').fill('Laut Dienstplan Beginn 8:00')
  await page.getByTestId('reject-confirm').click()
  await expect(card).toBeHidden()

  corr = await request.get(`/api/v1/employees/${emp.user.id}/corrections`, {
    headers: authHeaders(adminToken),
    params: { from: E2E_WORK_DATE, to: E2E_WORK_DATE },
  })
  expect(((await corr.json()) as { corrections: unknown[] | null }).corrections ?? []).toHaveLength(0)

  const mine = await request.get('/api/v1/me/requests', { headers: authHeaders(emp.token) })
  const list = ((await mine.json()) as { requests: { status: string; decision_comment: string }[] }).requests
  expect(list[0].status).toBe('rejected')
  expect(list[0].decision_comment).toBe('Laut Dienstplan Beginn 8:00')
})

test('approved vacation request creates vacation days', async ({ page, request }) => {
  const adminToken = admin.token
  const displayName = uniqueLabel('E2E Urlaub')
  const emp = await seedEmployeeSession(request, displayName)

  // Nächster Montag bis Mittwoch.
  const d = new Date()
  d.setDate(d.getDate() + ((8 - d.getDay()) % 7 || 7))
  const iso = (x: Date) => x.toISOString().slice(0, 10)
  const from = iso(d)
  d.setDate(d.getDate() + 2)
  const to = iso(d)

  const created = await request.post('/api/v1/me/requests', {
    headers: authHeaders(emp.token),
    data: { kind: 'vacation', date_from: from, date_to: to, reason: '' },
  })
  expect(created.status(), await created.text()).toBe(201)
  const req = (await created.json()) as { vacation_dates: string[] }

  await useSession(page, admin, '/requests')
  const card = page.getByTestId('leitung-request').filter({ hasText: displayName })
  await card.getByTestId('request-approve').click()
  await expect(card).toBeHidden()

  const abs = await request.get(`/api/v1/employees/${emp.user.id}/absences`, {
    headers: authHeaders(adminToken),
    params: { from, to },
  })
  const absences = ((await abs.json()) as { absences: { absence_type: string }[] | null }).absences ?? []
  expect(absences).toHaveLength(req.vacation_dates.length)
  expect(absences.every((a) => a.absence_type === 'vacation')).toBeTruthy()
})
