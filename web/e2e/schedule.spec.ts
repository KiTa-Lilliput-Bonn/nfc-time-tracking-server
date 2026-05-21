import { test, expect } from '@playwright/test'

import { apiLogin, loginAsAdmin } from './helpers/auth'
import {
  E2E_SCHEDULE_DATE,
  E2E_WEEK,
  E2E_WEEK_YEAR,
  germanDateFromIso,
} from './helpers/dates'
import { seedEmployee } from './helpers/seed'
import { closeOverlays, uniqueLabel } from './helpers/ui'

test('schedule shift persists after autosave and reload', async ({ page, request }) => {
  const token = await apiLogin(request)
  const displayName = uniqueLabel('E2E Dienstplan')
  const emp = await seedEmployee(request, token, {
    username: `e2e.sched.${Date.now()}`,
    display_name: displayName,
  })

  await loginAsAdmin(page)
  await page.goto('/schedule')

  await page.getByTestId('schedule-week-year').locator('input').fill(String(E2E_WEEK_YEAR))
  await page.getByTestId('schedule-week-year').locator('input').press('Tab')
  await page.getByTestId('schedule-week').locator('input').fill(String(E2E_WEEK))
  await page.getByTestId('schedule-week').locator('input').press('Tab')
  await closeOverlays(page)

  await expect(page.getByTestId('schedule-grid')).toBeVisible()
  await expect(page.getByRole('cell', { name: displayName }).first()).toBeVisible()

  const dayLabel = germanDateFromIso(E2E_SCHEDULE_DATE)
  const startInput = page.getByLabel(`Schichtbeginn ${displayName} ${dayLabel}`).first()
  const endInput = page.getByLabel(`Schichtende ${displayName} ${dayLabel}`).first()

  await expect(startInput).toBeEnabled({ timeout: 15_000 })
  await startInput.fill('08:00')
  await endInput.fill('16:00')
  await endInput.press('Tab')

  await expect(page.getByText('Gespeichert', { exact: false })).toBeVisible({ timeout: 20_000 })

  await page.reload()
  await page.getByTestId('schedule-week-year').locator('input').fill(String(E2E_WEEK_YEAR))
  await page.getByTestId('schedule-week-year').locator('input').press('Tab')
  await page.getByTestId('schedule-week').locator('input').fill(String(E2E_WEEK))
  await page.getByTestId('schedule-week').locator('input').press('Tab')
  await closeOverlays(page)

  await expect(page.getByTestId('schedule-grid')).toBeVisible()
  await expect(page.getByRole('cell', { name: displayName }).first()).toBeVisible()
  await expect(startInput).toBeEnabled({ timeout: 15_000 })

  await expect(startInput).toHaveValue('08:00', { timeout: 15_000 })
  await expect(endInput).toHaveValue('16:00', { timeout: 15_000 })
})
