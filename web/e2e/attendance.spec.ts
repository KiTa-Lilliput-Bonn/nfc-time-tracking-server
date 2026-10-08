import { test, expect, type APIRequestContext, type Page } from '@playwright/test'

import { authHeaders } from './helpers/auth'
import { E2E_ADMIN_PASSWORD, E2E_ADMIN_USER } from './helpers/env'
import { uniqueLabel } from './helpers/ui'

interface Session {
  token: string
  user: { id: number; username: string; display_name: string; role: string; must_change_password: boolean }
}

async function apiSession(request: APIRequestContext, username: string, password: string): Promise<Session> {
  const res = await request.post('/api/v1/auth/login', { data: { username, password } })
  expect(res.ok(), await res.text()).toBeTruthy()
  const body = (await res.json()) as Session
  return { token: body.token, user: { ...body.user, must_change_password: false } }
}

async function useSession(page: Page, s: Session, path: string) {
  await page.goto('/login')
  await page.evaluate((sess) => {
    localStorage.setItem('nfc_token', sess.token)
    localStorage.setItem('nfc_user', JSON.stringify(sess.user))
  }, s)
  await page.goto(path)
}

test('Gruppenaccount hakt Kinder ab, meldet Fehlen und prüft bei Evakuierung alle im Haus', async ({ page, request }) => {
  const admin = await apiSession(request, E2E_ADMIN_USER, E2E_ADMIN_PASSWORD)
  const groupName = uniqueLabel('Füchse')
  const grp = await request.post('/api/v1/groups', { headers: authHeaders(admin.token), data: { name: groupName } })
  expect(grp.ok()).toBeTruthy()
  const groupId = ((await grp.json()) as { id: number }).id

  // Eingestempelte Mitarbeiterin (ohne Ausstempeln) für die Evakuierungsliste.
  const staffName = uniqueLabel('Erzieherin')
  const emp = await request.post('/api/v1/employees', {
    headers: authHeaders(admin.token),
    data: { username: `e2e.att.${Date.now()}`, display_name: staffName, role: 'user' },
  })
  expect(emp.ok()).toBeTruthy()
  const empId = ((await emp.json()) as { user: { id: number } }).user.id
  const access = await request.get('/api/v1/attendance/access', { headers: authHeaders(admin.token) })
  const today = ((await access.json()) as { today: string }).today
  const seeded = await request.post('/api/v1/test/seed-imported-work-period', {
    headers: authHeaders(admin.token),
    data: { employee_id: empId, work_date: today, punch_in: `${today}T00:01:00Z`, open: true },
  })
  expect(seeded.ok(), await seeded.text()).toBeTruthy()

  // Leitung legt Kinder und den Gruppenaccount an.
  await useSession(page, admin, '/attendance/manage')
  await page.getByTestId(`manage-add-${groupId}`).fill('Anna Adler\nBen Bär\nCarla Christ')
  await page.getByTestId(`manage-add-btn-${groupId}`).click()
  const section = page.getByTestId(`manage-group-${groupId}`)
  await expect(section.getByText('Carla Christ')).toBeVisible()
  const user = `flur-${Date.now()}`
  await page.getByTestId(`manage-account-user-${groupId}`).fill(user)
  await page.getByTestId(`manage-account-create-${groupId}`).click()
  const pwBox = page.getByTestId('manage-account-password')
  await expect(pwBox).toBeVisible()
  const password = (await pwBox.locator('code').textContent())!.trim()

  // Anmeldung am Flur-Tablet: landet direkt in der Anwesenheitsliste der eigenen Gruppe.
  await page.evaluate(() => localStorage.clear())
  await page.goto('/login')
  await page.getByTestId('login-user').fill(user)
  await page.locator('[data-testid="login-password"] input').fill(password)
  await page.getByTestId('login-submit').click()
  await page.waitForURL('**/attendance')
  await expect(page.getByTestId('nav-attendance')).toHaveCount(0)
  await expect(page.getByRole('link', { name: 'Mein Saldo' })).toHaveCount(0)

  const card = (name: string) => page.locator('[data-testid^="att-child-"]', { hasText: name })
  await expect(card('Anna Adler')).toHaveAttribute('data-status', 'expected')
  await card('Anna Adler').getByRole('button', { name: 'Gekommen' }).click()
  await expect(card('Anna Adler')).toHaveAttribute('data-status', 'present')
  await expect(card('Anna Adler').getByTestId('att-state')).toContainText('da seit')

  // Versehentlich getippt: rückgängig.
  await card('Ben Bär').getByRole('button', { name: 'Gekommen' }).click()
  await expect(card('Ben Bär')).toHaveAttribute('data-status', 'present')
  await page.getByTestId('att-undo').getByRole('button', { name: 'Rückgängig' }).click()
  await expect(card('Ben Bär')).toHaveAttribute('data-status', 'expected')

  await card('Ben Bär').getByRole('button', { name: 'Gekommen' }).click()
  await card('Ben Bär').getByRole('button', { name: 'Gegangen' }).click()
  await expect(card('Ben Bär')).toHaveAttribute('data-status', 'gone')

  // Carla ist heute krank gemeldet.
  await card('Carla Christ').getByRole('button', { name: /bearbeiten/ }).click()
  const sheet = page.getByTestId('child-sheet')
  await sheet.getByTestId('notice-add').click()
  await sheet.getByTestId('notice-reason-sick').click()
  await sheet.getByTestId('notice-save').click()
  await expect(sheet.getByTestId('sheet-notice')).toContainText('Krank')
  await page.keyboard.press('Escape')
  await expect(card('Carla Christ')).toHaveAttribute('data-status', 'absent')
  await expect(card('Carla Christ').getByTestId('att-state')).toHaveText('Krank')

  // Evakuierung: nur Anna (Ben ist gegangen) und die eingestempelte Mitarbeiterin, ohne Stempelzeit.
  await page.getByTestId('att-evacuation').click()
  await page.waitForURL('**/attendance/evacuation')
  await expect(page.getByTestId('evac-total')).toHaveText('2')
  await expect(page.getByText('Anna Adler')).toBeVisible()
  await expect(page.getByText('Ben Bär')).toHaveCount(0)
  const staffRow = page.getByTestId(`evac-staff-${empId}`)
  await expect(staffRow).toHaveText(staffName)
  await page.getByText('Anna Adler').click()
  await staffRow.click()
  await expect(page.getByTestId('evac-done')).toHaveText('2')

  // Andere Seiten und APIs bleiben gesperrt.
  await page.goto('/dashboard')
  await page.waitForURL('**/attendance')
  const token = await page.evaluate(() => localStorage.getItem('nfc_token'))
  const denied = await request.get('/api/v1/me/times', { headers: authHeaders(token!) })
  expect(denied.status()).toBe(403)
})
