import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, devices } from '@playwright/test'

// Align Node helpers (seed) with browser/UI (Corrections default week uses local Date).
process.env.TZ = 'UTC'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(__dirname, '..')
const serverPort = process.env.NFC_SERVER_PORT ?? '8091'
const baseURL = process.env.PLAYWRIGHT_BASE_URL ?? `http://127.0.0.1:${serverPort}`
const e2eDb = path.join(os.tmpdir(), `nfc-e2e-${process.pid}.db`)

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  timeout: 60_000,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : [['list']],
  use: {
    ...devices['Desktop Chrome'],
    baseURL,
    timezoneId: 'UTC',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  webServer: {
    command: `bash ${path.join(repoRoot, 'scripts/e2e-web-server.sh')}`,
    url: `${baseURL}/api/v1/health`,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
    env: {
      ...process.env,
      NFC_DATABASE_PATH: e2eDb,
      NFC_SERVER_PORT: serverPort,
      NFC_TEST_MODE: '1',
      NFC_TEST_ADMIN_PASSWORD: process.env.NFC_TEST_ADMIN_PASSWORD ?? 'e2e-admin-password-min8',
      NFC_AUTH_JWT_SECRET: process.env.NFC_AUTH_JWT_SECRET ?? 'e2e-jwt-secret-min-32-chars-long!!',
      NFC_SERVER_HOST: '127.0.0.1',
      TZ: 'UTC',
    },
  },
})
