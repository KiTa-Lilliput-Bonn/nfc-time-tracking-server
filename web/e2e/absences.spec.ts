import { test, expect } from '@playwright/test'

import { apiLogin, loginAsAdmin } from './helpers/auth'
import { E2E_ABSENCE_DATE, germanDateFromIso } from './helpers/dates'
import { seedEmployee, seedEmployeeAbsence } from './helpers/seed'
import { uniqueLabel } from './helpers/ui'

test('absence appears in list after save', async ({ page, request }) => {
  const token = await apiLogin(request)
  const displayName = uniqueLabel('E2E Abwesenheit')
  const employee = await seedEmployee(request, token, {
    username: `e2e.abs.${Date.now()}`,
    display_name: displayName,
  })
  await seedEmployeeAbsence(request, token, employee.id, E2E_ABSENCE_DATE, 'sick')

  await loginAsAdmin(page)
  await page.goto('/absences')

  const label = germanDateFromIso(E2E_ABSENCE_DATE)
  await expect(page.getByTestId('absences-table')).toContainText(label)
  await expect(page.getByTestId('absences-table')).toContainText(displayName)
  await expect(page.getByTestId('absences-table')).toContainText('Krank')

  await page.reload()
  await expect(page.getByTestId('absences-table')).toContainText(label)
})
