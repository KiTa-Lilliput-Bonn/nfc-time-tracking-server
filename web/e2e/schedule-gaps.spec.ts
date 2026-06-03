import { test, expect } from '@playwright/test'

import { apiLogin, loginAsAdmin } from './helpers/auth'
import { E2E_GAP_DATE, germanDateFromIso } from './helpers/dates'
import { seedEmployee, seedScheduleShift, seedWeeklyHours } from './helpers/seed'
import { selectPrimeOption, uniqueLabel } from './helpers/ui'

test('dashboard warns and deep link opens manual time dialog', async ({ page, request }) => {
  const token = await apiLogin(request)
  const displayName = uniqueLabel('E2E Gap')
  const emp = await seedEmployee(request, token, {
    username: `e2e.gap.${Date.now()}`,
    display_name: displayName,
  })
  await seedWeeklyHours(request, token, emp.id, 40)
  await seedScheduleShift(request, token, emp.id, E2E_GAP_DATE, '08:00', '16:00')

  await loginAsAdmin(page)
  await page.goto('/dashboard')

  const warn = page.getByTestId('dashboard-schedule-gaps-warn')
  await expect(warn).toBeVisible()
  await warn.getByRole('link').click()
  await expect(page).toHaveURL(/\/schedule-gaps/)

  const table = page.getByTestId('schedule-gaps-table')
  await expect(table).toContainText(displayName)
  await expect(table).toContainText(germanDateFromIso(E2E_GAP_DATE))

  await page.getByTestId('schedule-gap-time-btn').first().click()
  await expect(page).toHaveURL(/\/schedule-gaps/)
  const manualDialog = page.getByRole('dialog', { name: 'Manuelle Arbeitszeit' })
  await expect(manualDialog).toBeVisible()
  await expect(manualDialog).toContainText(displayName)
  await expect(manualDialog.locator('input[type="time"]').first()).toHaveValue('08:00')
  await expect(manualDialog.locator('input[type="time"]').nth(1)).toHaveValue('16:00')
})
