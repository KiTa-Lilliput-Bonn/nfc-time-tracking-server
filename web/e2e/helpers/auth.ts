import type { APIRequestContext, Page } from '@playwright/test'

import { E2E_ADMIN_PASSWORD, E2E_ADMIN_USER } from './env'

export async function apiLogin(request: APIRequestContext): Promise<string> {
  const res = await request.post('/api/v1/auth/login', {
    data: { username: E2E_ADMIN_USER, password: E2E_ADMIN_PASSWORD },
  })
  if (!res.ok()) {
    throw new Error(`login failed: ${res.status()} ${await res.text()}`)
  }
  const body = (await res.json()) as { token: string }
  return body.token
}

export function authHeaders(token: string): Record<string, string> {
  return { Authorization: `Bearer ${token}` }
}

export async function loginAsAdmin(page: Page): Promise<void> {
  await page.goto('/login')
  await page.getByTestId('login-user').fill(E2E_ADMIN_USER)
  await page.locator('[data-testid="login-password"] input').fill(E2E_ADMIN_PASSWORD)
  await page.getByTestId('login-submit').click()
  await page.waitForURL('**/dashboard')
}
