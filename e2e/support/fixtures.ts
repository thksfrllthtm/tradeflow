import { existsSync } from 'node:fs'
import { test as base, expect, request, type APIRequestContext, type Page } from '@playwright/test'
import { authFile, MAX_WORKERS, workerInvoicer, type UserKey } from './users'

// Запросы, которые идут в платные модели (DeepSeek, Gemini). В браузерных тестах они по
// умолчанию отвечают 503 «ИИ отключён»; тест, которому нужен ответ ИИ, подставляет свой
// page.route — он регистрируется позже и поэтому главнее.
export const paidAI = /\/api\/(tnved-agent\/(sessions|reference)|ceramics\/(analyze|price)|estimates\/price-weights|translator\/(documents\/\d+\/translate|segments\/\d+\/retranslate))/

export async function blockPaidAI(page: Page) {
  const blocked: string[] = []
  await page.route(paidAI, (route) => {
    if (route.request().method() === 'GET') return route.fallback()
    blocked.push(route.request().url())
    return route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: { message: 'ИИ отключён в E2E', code: 'ai_not_configured' } }) })
  })
  return blocked
}

// Файл сессии роли; для инвойсера — учётка этого процесса (см. workerInvoicer), если setup её создал.
function stateFor(who: UserKey, parallelIndex: number) {
  if (who !== 'invoicer' || parallelIndex >= MAX_WORKERS) return authFile(who)
  const file = authFile(workerInvoicer(parallelIndex).key)
  return existsSync(file) ? file : authFile(who)
}

type Fixtures = {
  // Клиент API с сессией нужной роли; anon — без cookie.
  apiAs: (who: UserKey | 'anon') => Promise<APIRequestContext>
  aiBlocked: string[]
}

export const test = base.extend<Fixtures>({
  apiAs: async ({ baseURL }, use, testInfo) => {
    const opened: APIRequestContext[] = []
    await use(async (who) => {
      const context = await request.newContext({ baseURL, storageState: who === 'anon' ? undefined : stateFor(who, testInfo.parallelIndex) })
      opened.push(context)
      return context
    })
    await Promise.all(opened.map((context) => context.dispose()))
  },
  aiBlocked: [async ({ page }, use) => { await use(await blockPaidAI(page)) }, { auto: true }],
})

// test.use(as('invoicer')) — открыть страницы под этой ролью (инвойсер — учётка этого процесса).
// Для browser.newContext() нужен готовый путь: { storageState: authFile(...) }.
export const as = (who: UserKey) => ({
  storageState: async ({}, use: (path: string) => Promise<void>, testInfo: { parallelIndex: number }) => use(stateFor(who, testInfo.parallelIndex)),
})

export { expect }
