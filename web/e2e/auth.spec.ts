import { test, expect } from '@playwright/test'

import { loginAsAdmin } from './helpers/auth'

test('admin login and deep link after auth', async ({ page }) => {
  await loginAsAdmin(page)
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible()

  await page.goto('/corrections')
  await expect(page.getByRole('heading', { name: 'Korrekturen' })).toBeVisible()
  await expect(page.getByTestId('corrections-table')).toBeVisible()
})
