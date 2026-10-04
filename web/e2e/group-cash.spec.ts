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

function monthsAgo(n: number): string {
  const d = new Date()
  d.setDate(1)
  d.setMonth(d.getMonth() - n)
  return d.toISOString().slice(0, 7)
}

const PDF = Buffer.from('%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF\n')

test('Kassenwart bucht Monatsbetrag, Ansparkonto und Ausgabe mit Beleg; Leitung liest nur', async ({ page, request }) => {
  const admin = await apiSession(request, E2E_ADMIN_USER, E2E_ADMIN_PASSWORD)
  const groupName = uniqueLabel('Kasse Gruppe')
  const grp = await request.post('/api/v1/groups', { headers: authHeaders(admin.token), data: { name: groupName } })
  expect(grp.ok()).toBeTruthy()
  const groupId = ((await grp.json()) as { id: number }).id

  const username = `e2e.cash.${Date.now()}`
  const emp = await request.post('/api/v1/employees', {
    headers: authHeaders(admin.token),
    data: { username, display_name: uniqueLabel('Kassenwart'), role: 'user' },
  })
  expect(emp.ok()).toBeTruthy()
  const empBody = (await emp.json()) as { user: { id: number }; temporary_password: string }
  const keeper = await apiSession(request, username, empBody.temporary_password)

  // Ohne Zuweisung kein Zugriff.
  const denied = await request.get(`/api/v1/cash-boxes/${groupId}`, { headers: authHeaders(keeper.token) })
  expect(denied.status()).toBe(403)

  // Leitung weist den Kassenwart zu.
  await useSession(page, admin, `/cash-boxes/${groupId}`)
  await expect(page.getByTestId('cash-box-name')).toHaveText(groupName)
  await expect(page.getByTestId('cash-new-expense')).toHaveCount(0)
  await page.getByTestId('cash-keepers-edit').click()
  await page.getByTestId('cash-keepers-select').click()
  await page.getByRole('option', { name: keeper.user.display_name }).click()
  await page.keyboard.press('Escape')
  await page.getByTestId('cash-keepers-save').click()
  await expect(page.getByText(`Kassenwarte: ${keeper.user.display_name}`)).toBeVisible()

  // Kassenwart: 100 € pro Monat seit zwei Monaten → 200 € im Ansparkonto.
  await useSession(page, keeper, '/cash-boxes')
  await expect(page).toHaveURL(new RegExp(`/cash-boxes/${groupId}$`))
  await expect(page.getByTestId('nav-cash-boxes')).toBeVisible()
  await page.locator('#al-from').fill(monthsAgo(2))
  await page.locator('#al-amount').fill('100,00')
  await page.getByTestId('cash-allowance-save').click()
  await expect(page.getByTestId('cash-savings')).toHaveText(/200,00\s€/)

  // Monatsbetrag des laufenden Monats.
  await page.getByTestId('cash-new-income').click()
  await expect(page.getByTestId('cash-entry-hint')).toContainText('100,00')
  await page.locator('#ce-amount').fill('100')
  await page.getByTestId('cash-entry-save').click()
  await expect(page.getByTestId('cash-entry')).toHaveCount(1)

  // Entnahme aus dem Ansparkonto: mehr als vorhanden geht nicht.
  await page.getByTestId('cash-new-income').click()
  await page.getByTestId('cash-entry-source').getByText('Aus Ansparkonto').click()
  await page.locator('#ce-amount').fill('250')
  await page.getByTestId('cash-entry-save').click()
  await expect(page.getByText('Im Ansparkonto sind nur 200,00 € verfügbar.')).toBeVisible()
  await page.locator('#ce-amount').fill('200')
  await page.getByTestId('cash-entry-save').click()
  await expect(page.getByTestId('cash-entry')).toHaveCount(2)
  await expect(page.getByTestId('cash-savings')).toHaveText(/0,00\s€/)

  // Ausgabe mit zwei Belegen.
  await page.getByTestId('cash-new-expense').click()
  await page.locator('#ce-amount').fill('250,50')
  await page.locator('#ce-desc').fill('Ausflug Zoo')
  await page.getByTestId('cash-file-input').setInputFiles([
    { name: 'Eintritt.pdf', mimeType: 'application/pdf', buffer: PDF },
    { name: 'Bus.pdf', mimeType: 'application/pdf', buffer: PDF },
  ])
  await expect(page.getByTestId('cash-pending-file')).toHaveCount(2)
  await page.getByTestId('cash-entry-save').click()
  const zoo = page.getByTestId('cash-entry').filter({ hasText: 'Ausflug Zoo' })
  await expect(zoo.getByTestId('cash-receipt')).toHaveCount(2)
  await expect(page.getByTestId('cash-balance')).toHaveText(/49,50\s€/)

  // Leitung sieht alles, kann Belege öffnen, aber nicht buchen.
  await useSession(page, admin, `/cash-boxes/${groupId}`)
  await expect(page.getByTestId('cash-balance')).toHaveText(/49,50\s€/)
  await expect(page.getByTestId('cash-entry-edit')).toHaveCount(0)
  const receipts = (await (
    await request.get(`/api/v1/cash-boxes/${groupId}`, { headers: authHeaders(admin.token) })
  ).json()) as { entries: { receipts: { id: number }[] }[] }
  const rid = receipts.entries[0].receipts[0].id
  const file = await request.get(`/api/v1/cash-boxes/${groupId}/receipts/${rid}`, { headers: authHeaders(admin.token) })
  expect(file.headers()['content-type']).toBe('application/pdf')
  const write = await request.post(`/api/v1/cash-boxes/${groupId}/entries`, {
    headers: authHeaders(admin.token),
    data: { kind: 'expense', entry_date: new Date().toISOString().slice(0, 10), amount_cents: 100, description: 'x' },
  })
  expect(write.status()).toBe(403)
})
