import { defineConfig } from '@playwright/test'

const configuredBaseUrl = process.env.PLAYWRIGHT_BASE_URL
const localBaseUrl = 'http://127.0.0.1:5173'
const configuredWorkers = Number(process.env.PLAYWRIGHT_WORKERS)
const workers =
  Number.isSafeInteger(configuredWorkers) && configuredWorkers > 0
    ? configuredWorkers
    : 2

export default defineConfig({
  testDir: './e2e',
  testIgnore: process.env.PLAYWRIGHT_REAL_ENTITY_VALUES
    ? []
    : ['**/real-entity-values.spec.ts'],
  outputDir: './test-results',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  workers,
  reporter: [
    ['list'],
    ['html', { outputFolder: 'playwright-report', open: 'never' }],
  ],
  use: {
    baseURL: configuredBaseUrl ?? localBaseUrl,
    locale: 'zh-CN',
    timezoneId: 'Asia/Shanghai',
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium-desktop',
      use: {
        browserName: 'chromium',
        viewport: { width: 1440, height: 900 },
      },
    },
    {
      name: 'chromium-mobile',
      use: {
        browserName: 'chromium',
        hasTouch: true,
        isMobile: true,
        viewport: { width: 390, height: 844 },
      },
    },
    {
      name: 'chromium-tablet-768',
      use: {
        browserName: 'chromium',
        viewport: { width: 768, height: 1024 },
      },
    },
    {
      name: 'chromium-tablet-1024',
      use: {
        browserName: 'chromium',
        viewport: { width: 1024, height: 768 },
      },
    },
  ],
  webServer: configuredBaseUrl
    ? undefined
    : {
        command: 'bun run dev -- --host 127.0.0.1 --port 5173',
        url: localBaseUrl,
        reuseExistingServer: !process.env.CI,
        timeout: 120_000,
      },
})
