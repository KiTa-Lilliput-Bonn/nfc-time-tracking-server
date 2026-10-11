import { test, expect } from '@playwright/test'

import { apiLogin, authHeaders, loginAsAdmin } from './helpers/auth'
import { E2E_SCHEDULE_DATE } from './helpers/dates'
import { seedEmployee, seedScheduleShift } from './helpers/seed'
import { uniqueLabel } from './helpers/ui'

test('Dienstplan am Desktop: Kinder und KiBiz-Stunden je Gruppe und Tag', async ({ page, request }) => {
  const token = await apiLogin(request)
  const headers = authHeaders(token)
  const groupName = uniqueLabel('E2E KiBiz Desktop')
  expect((await request.post('/api/v1/groups', { headers, data: { name: groupName } })).ok()).toBeTruthy()
  const groups = (await (await request.get('/api/v1/groups', { headers })).json()) as { groups: { id: number; name: string }[] }
  const groupId = groups.groups.find((g) => g.name === groupName)!.id

  const fk = await seedEmployee(request, token, { username: `e2e.kibizd.${Date.now()}`, display_name: uniqueLabel('E2E Fachkraft D') })
  const other = await seedEmployee(request, token, { username: `e2e.kibizd2.${Date.now()}`, display_name: uniqueLabel('E2E Ohne D') })
  for (const e of [fk, other]) {
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
  await seedScheduleShift(request, token, other.id, E2E_SCHEDULE_DATE, '08:00', '12:00')

  await loginAsAdmin(page)
  await page.goto('/schedule')
  await expect(page.getByTestId('schedule-grid')).toBeVisible()

  // Woche: Kinder und Stunden im Gruppenkopf, je Tag eine Zeile
  await expect(page.getByTestId(`kibiz-week-${groupId}-children`)).toHaveText(/25 Kinder/)
  await expect(page.getByTestId(`kibiz-week-${groupId}-fk`)).toContainText('Fachkraft')
  await expect(page.getByTestId(`unqualified-${other.id}`)).toBeVisible()
  const dayIdx = (new Date(`${E2E_SCHEDULE_DATE}T12:00:00`).getDay() + 6) % 7
  const dayCell = `kibiz-day-${groupId}-${dayIdx}`
  await expect(page.getByTestId(`${dayCell}-fk`)).toContainText('FK')

  // Kinderzahl nur für diesen Tag anpassen
  await page.getByTestId(`${dayCell}-children`).click()
  const sheet = page.getByTestId('children-sheet')
  await expect(sheet).toBeVisible()
  await sheet.getByRole('button', { name: 'III · 35 h mehr' }).click()
  await page.getByTestId('children-save').click()
  await expect(sheet).toBeHidden()
  await expect(page.getByTestId(`${dayCell}-children`)).toHaveText(/26 ✎/)
  await expect(page.getByTestId(`kibiz-week-${groupId}-children`)).toHaveText(/25–26 Kinder/)
})
