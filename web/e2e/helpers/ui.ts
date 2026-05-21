import type { Page } from '@playwright/test'

import { E2E_MONTH, E2E_YEAR } from './dates'

export function uniqueLabel(prefix: string): string {
  return `${prefix} ${Date.now()}`
}

/** Closes open PrimeVue overlays (DatePicker panels, Select lists). */
export async function closeOverlays(page: Page): Promise<void> {
  await page.keyboard.press('Escape')
  await page.keyboard.press('Escape')
}

/** Sets Von/Bis on pages with the shared toolbar.range DatePickers (dd.mm.yy). */
export async function setToolbarMonthRange(page: Page, year = E2E_YEAR, month = E2E_MONTH): Promise<void> {
  const lastDay = new Date(year, month, 0).getDate()
  const from = `01.${String(month).padStart(2, '0')}.${String(year).slice(2)}`
  const to = `${String(lastDay).padStart(2, '0')}.${String(month).padStart(2, '0')}.${String(year).slice(2)}`
  const inputs = page.locator('.toolbar .range input')
  await inputs.nth(0).fill(from)
  await inputs.nth(0).press('Tab')
  await inputs.nth(1).fill(to)
  await inputs.nth(1).press('Tab')
  await closeOverlays(page)
}

export function isoToGermanShort(iso: string): string {
  const [y, m, d] = iso.split('-')
  return `${d}.${m}.${y.slice(2)}`
}

export async function selectPrimeOption(page: Page, optionLabel: string): Promise<void> {
  const option = page.locator(`[role="option"][aria-label="${optionLabel}"]`).first()
  await option.waitFor({ state: 'visible' })
  await option.click()
}
