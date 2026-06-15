import { test, expect, type Dialog } from '@playwright/test'

import { apiLogin, loginAsAdmin } from './helpers/auth'
import { E2E_WORK_DATE, germanDateFromIso } from './helpers/dates'
import { seedEmployee, seedImportedWorkPeriod, seedManualWorkPeriod } from './helpers/seed'
import { selectPrimeOption, uniqueLabel } from './helpers/ui'

test('manual work period and correction persist after reload', async ({ page, request }) => {
  const token = await apiLogin(request)
  const displayName = uniqueLabel('E2E Korrektur')
  const emp = await seedEmployee(request, token, {
    username: `e2e.corr.${Date.now()}`,
    display_name: displayName,
  })
  await seedManualWorkPeriod(
    request,
    token,
    emp.id,
    E2E_WORK_DATE,
    `${E2E_WORK_DATE}T08:00:00.000Z`,
    `${E2E_WORK_DATE}T16:00:00.000Z`,
  )

  await loginAsAdmin(page)
  await page.goto('/corrections')

  await page.getByTestId('corrections-employee-select').click()
  await selectPrimeOption(page, displayName)

  const dateLabel = germanDateFromIso(E2E_WORK_DATE)
  await expect(page.getByTestId('corrections-table')).toContainText(dateLabel)

  await page.getByRole('button', { name: 'Korrigieren' }).click()
  const corrDialog = page.getByRole('dialog', { name: 'Zeit korrigieren' })
  await corrDialog.locator('input[type="time"]').first().fill('09:00')
  await corrDialog.locator('input[type="time"]').nth(1).fill('17:00')
  await corrDialog.locator('input[type="text"]').fill('E2E Korrekturgrund')
  await page.getByTestId('corrections-save-correction').click()
  await expect(corrDialog).toBeHidden()

  await expect(page.getByTestId('corrections-table')).toContainText('09:00')
  await expect(page.getByTestId('corrections-table')).toContainText('17:00')

  await page.reload()
  await page.getByTestId('corrections-employee-select').click()
  await selectPrimeOption(page, displayName)
  await expect(page.getByTestId('corrections-table')).toContainText('09:00')
  await expect(page.getByTestId('corrections-table')).toContainText('17:00')
})

test('imported stamp can be disabled and shows in corrections table', async ({ page, request }) => {
  const token = await apiLogin(request)
  const displayName = uniqueLabel('E2E Deaktiv')
  const emp = await seedEmployee(request, token, {
    username: `e2e.dis.${Date.now()}`,
    display_name: displayName,
  })
  await seedImportedWorkPeriod(
    request,
    token,
    emp.id,
    E2E_WORK_DATE,
    `${E2E_WORK_DATE}T08:00:00.000Z`,
    `${E2E_WORK_DATE}T16:00:00.000Z`,
  )

  await loginAsAdmin(page)
  await page.goto('/corrections')

  await page.getByTestId('corrections-employee-select').click()
  await selectPrimeOption(page, displayName)

  const dateLabel = germanDateFromIso(E2E_WORK_DATE)
  await expect(page.getByTestId('corrections-table')).toContainText(dateLabel)
  await expect(page.getByTestId('corrections-disable-btn')).toBeVisible()

  const handleDialog = async (dialog: Dialog) => {
    if (dialog.type() === 'prompt') {
      await dialog.accept('Falscher Stempel')
    } else {
      await dialog.accept()
      page.off('dialog', handleDialog)
    }
  }
  page.on('dialog', handleDialog)
  await page.getByTestId('corrections-disable-btn').click()

  await expect(page.getByTestId('corrections-table')).toContainText('Deaktiviert')
  await expect(page.getByTestId('corrections-table')).toContainText('Falscher Stempel')

  await page.reload()
  await page.getByTestId('corrections-employee-select').click()
  await selectPrimeOption(page, displayName)
  await expect(page.getByTestId('corrections-table')).toContainText('Deaktiviert')
})

test('manual work period has no deactivate button', async ({ page, request }) => {
  const token = await apiLogin(request)
  const displayName = uniqueLabel('E2E Manuell')
  const emp = await seedEmployee(request, token, {
    username: `e2e.man.${Date.now()}`,
    display_name: displayName,
  })
  await seedManualWorkPeriod(
    request,
    token,
    emp.id,
    E2E_WORK_DATE,
    `${E2E_WORK_DATE}T08:00:00.000Z`,
    `${E2E_WORK_DATE}T16:00:00.000Z`,
  )

  await loginAsAdmin(page)
  await page.goto('/corrections')
  await page.getByTestId('corrections-employee-select').click()
  await selectPrimeOption(page, displayName)

  await expect(page.getByTestId('corrections-disable-btn')).toHaveCount(0)
})
