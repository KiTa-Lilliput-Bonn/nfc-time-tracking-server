import { test, expect } from '@playwright/test'

import { apiLogin, loginAsAdmin } from './helpers/auth'
import { E2E_SCHEDULE_DATE } from './helpers/dates'
import { seedEmployee, seedEmployeeAbsence, seedWeeklyHours } from './helpers/seed'
import { uniqueLabel } from './helpers/ui'

test.use({ viewport: { width: 390, height: 844 }, hasTouch: true })

test('Dienstplan am Handy: Schicht anlegen, rückgängig machen, Soll und Krankmeldung', async ({ page, request }) => {
  const token = await apiLogin(request)
  const emp = await seedEmployee(request, token, {
    username: `e2e.msched.${Date.now()}`,
    display_name: uniqueLabel('E2E Handyplan'),
  })
  await seedWeeklyHours(request, token, emp.id, 35)
  const sick = await seedEmployee(request, token, {
    username: `e2e.msick.${Date.now()}`,
    display_name: uniqueLabel('E2E Krank'),
  })
  await seedEmployeeAbsence(request, token, sick.id, E2E_SCHEDULE_DATE, 'sick')

  await loginAsAdmin(page)
  await page.evaluate(() => localStorage.removeItem('nfc.schedule.preferTable'))
  await page.goto('/schedule')
  await expect(page.getByTestId('mobile-schedule')).toBeVisible()

  // Ganze Woche ohne seitliches Wischen
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)

  const cell = page.getByTestId(`cell-${emp.id}-${E2E_SCHEDULE_DATE}`)
  await expect(cell).toHaveText('+')
  await expect(page.getByTestId(`cell-${sick.id}-${E2E_SCHEDULE_DATE}`)).toHaveText('krank')

  await cell.click()
  const sheet = page.getByTestId('shift-sheet')
  await expect(sheet).toBeVisible()
  await page.getByTestId('shift-start').fill('08:00')
  await page.getByTestId('shift-end').fill('14:00')
  await page.getByTestId('shift-save').click()
  await expect(sheet).toBeHidden()
  await expect(cell).toHaveText('8–14')
  await expect(page.getByTestId(`planned-${emp.id}`)).toHaveText(/^\d+(½|,\d)? \/ \d+(½|,\d)? h$/)

  // Bleibt nach dem Neuladen erhalten
  await page.reload()
  await expect(cell).toHaveText('8–14')

  // Frei geben und rückgängig machen
  await cell.click()
  await page.getByTestId('shift-clear').click()
  await expect(cell).toHaveText('+')
  await page.getByTestId('undo-btn').click()
  await expect(cell).toHaveText('8–14')
  await page.reload()
  await expect(cell).toHaveText('8–14')

  // Tagesansicht zeigt die Schicht
  await page.getByTestId('mode-day').click()
  const dayIdx = (new Date(`${E2E_SCHEDULE_DATE}T12:00:00`).getDay() + 6) % 7
  await page.getByTestId(`day-chip-${dayIdx}`).click()
  await expect(page.getByTestId(`day-row-${emp.id}`)).toContainText('08:00–14:00')

  // Tabellen-Ansicht bleibt erreichbar und lässt sich zurückschalten
  await page.getByTestId('schedule-menu').click()
  await page.getByRole('menuitem', { name: /Tabellen-Ansicht/ }).click()
  await expect(page.getByTestId('schedule-to-mobile')).toBeVisible()
  await page.getByTestId('schedule-to-mobile').click()
  await expect(page.getByTestId('mobile-schedule')).toBeVisible()
})
