import { defineConfig, devices } from '@playwright/test'

// E2E-тесты против уже запущенного стека (`docker compose up` или `... watch`).
// Frontend проксирует /api на backend, поэтому всё идёт через один адрес.
const baseURL = process.env.E2E_BASE_URL ?? 'http://localhost:5173'

export default defineConfig({
  testDir: './tests',
  outputDir: './test-results',
  fullyParallel: true,
  workers: process.env.CI ? 2 : 4,
  retries: 0,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL,
    locale: 'ru-RU',
    timezoneId: 'Asia/Almaty',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'setup', testMatch: /global\.setup\.ts/, teardown: 'teardown' },
    { name: 'api', testDir: './tests/api', dependencies: ['setup'] },
    { name: 'ui', testDir: './tests/ui', dependencies: ['setup'], use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } } },
    { name: 'teardown', testMatch: /global\.teardown\.ts/ },
  ],
})
