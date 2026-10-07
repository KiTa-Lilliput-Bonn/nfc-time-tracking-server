import { test, expect } from '@playwright/test'

import { apiLogin, authHeaders, loginAsAdmin } from './helpers/auth'
import { E2E_SCHEDULE_DATE } from './helpers/dates'
import { seedEmployee, seedScheduleShift } from './helpers/seed'
import { uniqueLabel } from './helpers/ui'

test.use({ viewport: { width: 390, height: 844 }, hasTouch: true })

test('Dienstplan am Handy: KiBiz-Stunden je Gruppe und Kinderzahl für einen Tag anpassen', async ({ page, request }) => {
  const token = await apiLogin(request)
  const headers = authHeaders(token)
  const groupName = uniqueLabel('E2E KiBiz')
  expect((await request.post('/api/v1/groups', { headers, data: { name: groupName } })).ok()).toBeTruthy()
  const groups = (await (await request.get('/api/v1/groups', { headers })).json()) as { groups: { id: number; name: string }[] }
  const groupId = groups.groups.find((g) => g.name === groupName)!.id

  const fk = await seedEmployee(request, token, { username: `e2e.kibiz.${Date.now()}`, display_name: uniqueLabel('E2E Fachkraft') })
  const ek = await seedEmployee(request, token, { username: `e2e.kibiz2.${Date.now()}`, display_name: uniqueLabel('E2E Ohne Angabe') })
  for (const e of [fk, ek]) {
    expect((await request.patch(`/api/v1/employees/${e.id}`, { headers, data: { group_id: groupId } })).ok()).toBeTruthy()
  }
  expect(
    (await request.put(`/api/v1/planning/qualifications/${fk.id}`, { headers, data: { qualification: 'fachkraft' } })).ok(),
  ).toBeTruthy()
  expect(
    (
      await request.put('/api/v1/planning/child-patterns', {
        headers,
        data: { group_id: groupId, valid_from: '2020-01-01', counts: [{ group_form: 'III', care_hours: 35, count: 25 }] },
      })
    ).ok(),
  ).toBeTruthy()
  await seedScheduleShift(request, token, fk.id, E2E_SCHEDULE_DATE, '08:00', '14:00')
  await seedScheduleShift(request, token, ek.id, E2E_SCHEDULE_DATE, '08:00', '12:00')

  await loginAsAdmin(page)
  await page.evaluate(() => localStorage.removeItem('nfc.schedule.preferTable'))
  await page.goto('/schedule')
  await expect(page.getByTestId('mobile-schedule')).toBeVisible()

  // Woche: Kinder und Fachkraft-/Gesamtstunden der Gruppe, Hinweis auf fehlende Qualifikation
  await expect(page.getByTestId(`kibiz-week-${groupId}-children`)).toHaveText(/25 Kinder/)
  await expect(page.getByTestId(`kibiz-week-${groupId}-fk`)).toContainText('Fachkraft')
  await expect(page.getByTestId(`unqualified-${ek.id}`)).toBeVisible()
  await expect(page.getByTestId(`unqualified-${fk.id}`)).toHaveCount(0)

  // Tag: Kinderzahl nur für diesen Tag erhöhen
  await page.getByTestId('mode-day').click()
  const dayIdx = (new Date(`${E2E_SCHEDULE_DATE}T12:00:00`).getDay() + 6) % 7
  await page.getByTestId(`day-chip-${dayIdx}`).click()
  await page.getByTestId(`kibiz-day-${groupId}-children`).click()
  const sheet = page.getByTestId('children-sheet')
  await expect(sheet).toBeVisible()
  await sheet.getByRole('button', { name: 'III · 35 h mehr' }).click()
  await page.getByTestId('children-scope-day').click()
  await page.getByTestId('children-save').click()
  await expect(sheet).toBeHidden()
  await expect(page.getByTestId(`kibiz-day-${groupId}-children`)).toHaveText(/26 Kinder ✎/)

  await page.getByTestId('mode-week').click()
  await expect(page.getByTestId(`kibiz-week-${groupId}-children`)).toHaveText(/25–26 Kinder/)

  // Zurücksetzen auf das Muster
  await page.getByTestId('mode-day').click()
  await page.getByTestId(`day-chip-${dayIdx}`).click()
  await page.getByTestId(`kibiz-day-${groupId}-children`).click()
  await page.getByTestId('children-reset').click()
  await expect(page.getByTestId(`kibiz-day-${groupId}-children`)).toHaveText(/^\s*25 Kinder\s*$/)

  // Planungsgrundlagen: Qualifikation nachtragen
  await page.goto('/schedule/basis?tab=people')
  const saved = page.waitForResponse((r) => r.url().includes(`/planning/qualifications/${ek.id}`) && r.ok())
  await page.getByTestId(`qualification-${ek.id}`).selectOption('ergaenzungskraft')
  await saved
  await page.goto('/schedule')
  await expect(page.getByTestId(`kibiz-week-${groupId}-total`)).toBeVisible()
  await expect(page.getByTestId(`unqualified-${ek.id}`)).toHaveCount(0)
})
