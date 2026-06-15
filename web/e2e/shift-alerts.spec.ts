import { test, expect, type Page } from '@playwright/test'

import { apiLogin, loginAsAdmin } from './helpers/auth'
import { E2E_ALERT_DATE, germanDateFromIso } from './helpers/dates'
import { seedEmployee, seedManualWorkPeriod, seedWeeklyHours } from './helpers/seed'
import { uniqueLabel } from './helpers/ui'

function longShiftPunchTimes(workDate: string): { punchIn: string; punchOut: string } {
  return {
    punchIn: `${workDate}T06:00:00.000Z`,
    punchOut: `${workDate}T18:30:00.000Z`,
  }
}

function shiftAlertRow(page: Page, displayName: string) {
  return page.getByTestId('shift-alerts-table').locator('tr').filter({ hasText: displayName })
}

async function expectShiftAlertRowGone(page: Page, displayName: string) {
  await expect(shiftAlertRow(page, displayName)).toHaveCount(0)
}

test('dashboard warns and list allows dismiss', async ({ page, request }) => {
  const token = await apiLogin(request)
  const displayName = uniqueLabel('E2E ShiftAlert')
  const emp = await seedEmployee(request, token, {
    username: `e2e.shift.${Date.now()}`,
    display_name: displayName,
  })
  await seedWeeklyHours(request, token, emp.id, 40)
  const { punchIn, punchOut } = longShiftPunchTimes(E2E_ALERT_DATE)
  await seedManualWorkPeriod(request, token, emp.id, E2E_ALERT_DATE, punchIn, punchOut)

  await loginAsAdmin(page)
  await page.goto('/dashboard')

  const warn = page.getByTestId('dashboard-shift-alerts-warn')
  await expect(warn).toBeVisible()
  await warn.getByRole('link').click()
  await expect(page).toHaveURL(/\/shift-alerts/)

  const table = page.getByTestId('shift-alerts-table')
  await expect(table).toContainText(displayName)
  await expect(table).toContainText(germanDateFromIso(E2E_ALERT_DATE))

  const row = shiftAlertRow(page, displayName)
  await row.getByTestId('shift-alert-dismiss-btn').click()
  await expectShiftAlertRowGone(page, displayName)

  await page.reload()
  await expectShiftAlertRowGone(page, displayName)
})

test('correction removes shift alert', async ({ page, request }) => {
  const token = await apiLogin(request)
  const displayName = uniqueLabel('E2E ShiftFix')
  const emp = await seedEmployee(request, token, {
    username: `e2e.shiftfix.${Date.now()}`,
    display_name: displayName,
  })
  await seedWeeklyHours(request, token, emp.id, 40)
  const { punchIn, punchOut } = longShiftPunchTimes(E2E_ALERT_DATE)
  await seedManualWorkPeriod(request, token, emp.id, E2E_ALERT_DATE, punchIn, punchOut)

  await loginAsAdmin(page)
  await page.goto('/shift-alerts')

  const table = page.getByTestId('shift-alerts-table')
  await expect(table).toContainText(displayName)

  const row = shiftAlertRow(page, displayName)
  await row.getByTestId('shift-alert-correct-btn').click()
  const dialog = page.getByRole('dialog', { name: 'Zeit korrigieren' })
  await expect(dialog).toBeVisible()
  await dialog.locator('input[type="time"]').nth(1).fill('16:00')
  await dialog.locator('input[type="text"]').fill('Korrektur E2E')
  await dialog.getByRole('button', { name: 'Speichern' }).click()
  await expect(dialog).toBeHidden()

  await expectShiftAlertRowGone(page, displayName)
})
