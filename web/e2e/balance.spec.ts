import { test, expect } from '@playwright/test'

import { apiLogin, loginAsAdmin } from './helpers/auth'
import { E2E_MONTH, E2E_YEAR } from './helpers/dates'
import { seedBalanceScenario, seedEmployee } from './helpers/seed'

test('employee balance tab shows seeded month saldo', async ({ page, request }) => {
  const token = await apiLogin(request)
  const emp = await seedEmployee(request, token, {
    username: `e2e.bal.${Date.now()}`,
    display_name: 'E2E Saldo',
  })
  const bal = await seedBalanceScenario(request, token, emp.id)

  await loginAsAdmin(page)
  await page.goto(`/employees/${emp.id}`)

  await page.getByTestId('employee-tab-balance').click()
  await page.getByTestId('employee-bal-month').locator('input').fill(String(E2E_MONTH))
  await page.getByTestId('employee-bal-year').locator('input').fill(String(E2E_YEAR))

  const card = page.getByTestId('balance-card')
  await expect(card).toBeVisible()
  await expect(card.getByTestId('balance-worked')).toContainText(bal.worked_hours.toFixed(2))
  await expect(card.getByTestId('balance-month')).toContainText(bal.balance_hours.toFixed(2))
})
