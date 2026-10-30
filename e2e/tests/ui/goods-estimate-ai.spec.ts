import type { APIRequestContext, Page } from '@playwright/test'
import { test, expect, as } from '../../support/fixtures'
import { E2E } from '../../support/users'
import { CODE_PLASTIC, CODE_TOYS, COL, cell, createEstimate, deleteEstimate, item, mockNbk, mockTrademarksEmpty, seedDraft } from '../../support/goods-ui'

// ИИ на этапах сметы — только на подставных ответах (page.route): подбор кода агентом для строки,
// подбор цен под кассу «По товарам». Настоящие DeepSeek и Gemini не вызываются; без подстановки
// фикстура отвечает 503 «ИИ отключён».
test.use(as('invoicer'))

const created: number[] = []
test.afterEach(async ({ apiAs }) => {
  const api = await apiAs('invoicer')
  for (const id of created.splice(0)) await deleteEstimate(api, id)
})

test.beforeEach(async ({ page }) => {
  await mockNbk(page)
  await mockTrademarksEmpty(page)
})

async function openSeeded(page: Page, api: APIRequestContext, items: Record<string, unknown>[]) {
  const estimate = await createEstimate(api)
  created.push(estimate.id)
  await seedDraft(api, estimate.id, items)
  await page.goto(`/estimates/goods/${estimate.id}`)
  await expect(cell(page, COL.name, 0)).toBeVisible()
  return estimate.id
}

const SESSION = 777
const now = () => new Date().toISOString()

// Подставной агент: сессия строки, ответ «Подобрать код» и применение. Возвращает тела запросов.
async function mockAgent(page: Page, estimateId: number, itemId: string, suggested: string) {
  const calls: Record<string, unknown[]> = { start: [], begin: [], apply: [] }
  const agentItem = { name: `${E2E} товар 1`, supplierName: '', code: CODE_PLASTIC, quantity: '10', places: '1', grossKg: '5', netKg: '4', pricePerUnit: '2', unit: 'шт' }
  const base = { id: SESSION, mode: 'row', estimateId, itemId, error: '', item: agentItem, steps: 0, runs: 0, costUsd: '0', tokens: 0, model: 'mock', createdAt: now(), updatedAt: now() }
  const input = { id: 1, kind: 'input', content: { item: agentItem, photos: 0, ai: false }, createdAt: now() }
  const match = { id: 2, kind: 'match', content: { matches: [], skipped: false }, createdAt: now() }
  const result = {
    description: `${E2E}: пластиковая игрушка-самокат для детей`,
    candidates: [
      { code: suggested, verdict: 'fits', quote: 'Трехколесные велосипеды, самокаты', quoteSource: 'name', reason: 'Это детский самокат', check: '', pageOpened: true, nomenclature: null },
      { code: CODE_PLASTIC, verdict: 'no', quote: '', quoteSource: '', reason: 'Не изделие из пластмассы общего назначения', check: '', pageOpened: false, nomenclature: null },
    ],
    name: 'Самокат детский', notes: '', droppedCodes: [], quotesRemoved: 0, confidence: 'high', confidenceReason: 'Совпадает наименование позиции',
  }
  let session: Record<string, unknown> | null = null

  await page.route(/\/api\/tnved-agent\/status/, (route) => route.fulfill({ json: { configured: true, model: 'mock', vision: false, dailyLimit: 100, usedToday: 3 } }))
  await page.route(/\/api\/tnved-agent\/sessions(\/\d+(\/\w+)?)?(\?.*)?$/, async (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    if (request.method() === 'GET' && path.endsWith('/sessions')) {
      return route.fulfill({ json: { items: session ? [{ sessionId: SESSION, itemId, status: session.status, bestCode: session.status === 'done' ? suggested : '', verdict: session.status === 'done' ? 'fits' : '' }] : [] } })
    }
    if (request.method() === 'POST' && path.endsWith('/sessions')) {
      calls.start.push(request.postDataJSON())
      session = { ...base, status: 'ready', result: null, messages: [input, match] }
      return route.fulfill({ json: session })
    }
    if (request.method() === 'POST' && path.endsWith('/begin')) {
      calls.begin.push(path)
      session = { ...base, status: 'done', runs: 1, steps: 3, result, messages: [input, match, { id: 3, kind: 'start', content: {}, createdAt: now() },
        { id: 4, kind: 'tool', content: { tool: 'search_nomenclature', label: 'Поиск в номенклатуре', detail: 'самокат' }, createdAt: now() },
        { id: 5, kind: 'result', content: result, createdAt: now() }] }
      return route.fulfill({ json: session })
    }
    if (request.method() === 'POST' && path.endsWith('/apply')) {
      calls.apply.push(request.postDataJSON())
      return route.fulfill({ json: { id: 1, code: suggested } })
    }
    if (request.method() === 'GET' && session) return route.fulfill({ json: session })
    return route.fulfill({ status: 404, json: { error: { message: 'нет в подставном агенте' } } })
  })
  return calls
}

test('чат строки: поиск в базе → «Подобрать код» → предложение → «Применить код» меняет код строки', async ({ page, apiAs }) => {
  const api = await apiAs('invoicer')
  const estimate = await createEstimate(api)
  created.push(estimate.id)
  await seedDraft(api, estimate.id, [item(0, { code: CODE_PLASTIC })])
  const calls = await mockAgent(page, estimate.id, 'e2e-row-0', CODE_TOYS)
  await page.goto(`/estimates/goods/${estimate.id}`)
  await expect(cell(page, COL.code, 0)).toContainText('3926 90 970 9')

  // Значок агента — у кода строки.
  await cell(page, COL.code, 0).locator('[data-act="agent"]').click({ force: true })
  const panel = page.getByRole('complementary', { name: 'Подбор кода' })
  await expect(panel).toBeVisible()
  await expect(panel).toContainText('Строка 1 из 1')
  await expect(panel).toContainText('Подборов сегодня: 3 из 100')
  expect(calls.start).toHaveLength(1)
  expect(calls.start[0]).toMatchObject({ estimateId: estimate.id, itemId: 'e2e-row-0', item: { name: `${E2E} товар 1`, code: CODE_PLASTIC } })

  await panel.getByRole('button', { name: 'Подобрать код' }).click()
  expect(calls.begin).toHaveLength(1)
  await expect(panel.getByRole('heading', { name: 'Предложение агента' })).toBeVisible()
  await expect(panel).toContainText('Отличается от кода из файла')
  await expect(panel).toContainText('Не подходят: 3926 90 970 9')

  await panel.getByRole('button', { name: 'Применить код' }).first().click()
  await expect(panel).toContainText('✓ Код применён к строке')
  expect(calls.apply).toHaveLength(1)
  expect(calls.apply[0]).toMatchObject({ code: CODE_TOYS })

  await expect(cell(page, COL.code, 0)).toContainText('9503 00 700 0')
  await expect(cell(page, COL.duty, 0)).toContainText('10%')

  // Esc закрывает панель.
  await page.keyboard.press('Escape')
  await expect(panel).toBeHidden()
})

test('«Подобрать коды ИИ» для строк без кода: подтверждение и запуск с auto', async ({ page, apiAs }) => {
  const api = await apiAs('invoicer')
  const estimate = await createEstimate(api)
  created.push(estimate.id)
  await seedDraft(api, estimate.id, [item(0, { code: CODE_PLASTIC }), item(1)])
  const calls = await mockAgent(page, estimate.id, 'e2e-row-1', CODE_TOYS)
  await page.goto(`/estimates/goods/${estimate.id}`)

  const dialogs: string[] = []
  page.on('dialog', (dialog) => { dialogs.push(dialog.message()); void dialog.accept() })
  await page.getByRole('button', { name: 'Подобрать коды ИИ (1)' }).click()
  await expect.poll(() => calls.start.length).toBe(1)
  expect(dialogs[0]).toContain('Подобрать коды для 1 строк без кода?')
  expect(calls.start[0]).toMatchObject({ itemId: 'e2e-row-1', auto: true })
})

test('ИИ недоступен (сервер ответил ошибкой) — чат строки показывает ошибку', async ({ page, apiAs }) => {
  // Только статус «настроен»; запуск подбора отвечает 503 из общей фикстуры.
  await page.route(/\/api\/tnved-agent\/status/, (route) => route.fulfill({ json: { configured: true, model: 'mock', vision: false, dailyLimit: 100, usedToday: 0 } }))
  await page.route(/\/api\/tnved-agent\/sessions\?estimateId=/, (route) => route.fulfill({ json: { items: [] } }))
  const api = await apiAs('invoicer')
  await openSeeded(page, api, [item(0, { code: CODE_PLASTIC })])
  await cell(page, COL.code, 0).locator('[data-act="agent"]').click({ force: true })
  const panel = page.getByRole('complementary', { name: 'Подбор кода' })
  await expect(panel.getByRole('alert')).toContainText('ИИ отключён в E2E')
})

test('агент не настроен на сервере — кнопка подбора неактивна и есть пояснение', async ({ page, apiAs }) => {
  await page.route(/\/api\/tnved-agent\/status/, (route) => route.fulfill({ json: { configured: false, model: '', vision: false, dailyLimit: 0, usedToday: 0 } }))
  await page.route(/\/api\/tnved-agent\/sessions\?estimateId=/, (route) => route.fulfill({ json: { items: [] } }))
  const api = await apiAs('invoicer')
  await openSeeded(page, api, [item(0, { code: CODE_PLASTIC })])
  await cell(page, COL.code, 0).locator('[data-act="agent"]').click({ force: true })
  const panel = page.getByRole('complementary', { name: 'Подбор кода' })
  await expect(panel).toContainText('Подбор кода не настроен')
  await expect(panel.getByRole('button', { name: 'Подобрать код' })).toBeDisabled()
})

// ── Подбор цен под кассу (этап «Результат») ──

// Две строки с кодами и без цены: этап «Результат» после «Рассчитать».
async function resultWithoutPrices(page: Page, api: APIRequestContext) {
  await openSeeded(page, api, [
    item(0, { code: CODE_PLASTIC, pairs: '100', grossKg: '55', netKg: '50', pricePerPair: '', sourcePricePerPair: '' }),
    item(1, { code: CODE_TOYS, pairs: '20', grossKg: '44', netKg: '40', pricePerPair: '', sourcePricePerPair: '' }),
  ])
  await page.getByRole('button', { name: /^Рассчитать/ }).click()
  await expect(page.getByRole('region', { name: 'Итоги расчёта' })).toBeVisible()
  await page.getByLabel('Касса растаможки, USD').fill('1000')
}

// Остаток кассы «ещё N» после подбора. По умолчанию недостача 35–45 USD (BUDGET_MIN_GAP/MAX_GAP;
// в docs/STATUS.md до сих пор написано 20–30).
const GAP_MIN = 35
const GAP_MAX = 45
async function restUsd(page: Page) {
  const text = await page.locator('.budget-rest').innerText()
  expect(text).toMatch(/^ещё /)
  return Number(text.replace(/[^\d,]/g, '').replace(',', '.'))
}

test('математический подбор под кассу (без ИИ): итог меньше кассы на 35–45 USD', async ({ page, apiAs }) => {
  const api = await apiAs('invoicer')
  await resultWithoutPrices(page, api)
  let aiCalls = 0
  page.on('request', (request) => { if (request.url().includes('/price-weights')) aiCalls++ })

  await page.getByRole('button', { name: 'Другие способы подбора цен' }).click()
  const settings = page.getByRole('dialog', { name: 'Подбор цен' })
  await expect(settings.getByRole('radio', { name: 'Меньше кассы' })).toHaveAttribute('aria-checked', 'true')
  await expect(settings.getByLabel('Разница с кассой от, USD')).toHaveValue(String(GAP_MIN))
  await expect(settings.getByLabel('Разница с кассой до, USD')).toHaveValue(String(GAP_MAX))
  await settings.getByRole('button', { name: 'Математический подбор' }).click()

  await expect(page.locator('.budget-rest')).toBeVisible({ timeout: 30_000 })
  const rest = await restUsd(page)
  expect(rest).toBeGreaterThanOrEqual(GAP_MIN)
  expect(rest).toBeLessThanOrEqual(GAP_MAX)
  expect(aiCalls, 'математический подбор не зовёт ИИ').toBe(0)

  // Отмена подбора возвращает пустые цены.
  await page.getByRole('button', { name: 'Другие способы подбора цен' }).click()
  await page.getByRole('dialog', { name: 'Подбор цен' }).getByRole('button', { name: 'Отменить подбор цен' }).click()
  await expect(page.getByRole('region', { name: 'Итоги расчёта' })).toContainText('расчёт не завершён')
})

test('подбор «По товарам» (ИИ, подставной ответ): цены с пояснением и точкой «Цена оценена ИИ»', async ({ page, apiAs }) => {
  const asked: unknown[] = []
  await page.route(/\/api\/estimates\/price-weights/, async (route) => {
    const body = route.request().postDataJSON() as { groups: Array<{ code: string; products: Array<{ id: string }> }> }
    asked.push(body)
    const prices = body.groups.flatMap((group) => group.products.map((product) => ({ id: product.id, code: group.code, unitPrice: group.code === CODE_TOYS ? 20 : 2, reason: `${E2E} оценка цены` })))
    await route.fulfill({ json: { prices } })
  })
  const api = await apiAs('invoicer')
  await resultWithoutPrices(page, api)

  await page.getByRole('button', { name: 'Подобрать цены ИИ' }).click()
  await expect(page.locator('.budget-rest')).toBeVisible({ timeout: 30_000 })
  expect(asked).toHaveLength(1)
  const rest = await restUsd(page)
  expect(rest).toBeGreaterThanOrEqual(GAP_MIN)
  expect(rest).toBeLessThanOrEqual(GAP_MAX)
  await expect(page.getByRole('img', { name: 'Цена оценена ИИ' })).toHaveCount(2)
  await expect(page.getByRole('tab', { name: /Цена оценена\s*2/ })).toBeVisible()
})

test('подбор «По товарам» при недоступном ИИ: сообщение и цены подобраны обычным способом', async ({ page, apiAs }) => {
  const api = await apiAs('invoicer')
  await resultWithoutPrices(page, api)
  await page.getByRole('button', { name: 'Подобрать цены ИИ' }).click()
  await expect(page.locator('.budget-notice')).toContainText('Подбор по товарам недоступен (ИИ отключён в E2E), цены подобраны обычным способом', { timeout: 30_000 })
  const rest = await restUsd(page)
  expect(rest).toBeGreaterThanOrEqual(GAP_MIN)
  expect(rest).toBeLessThanOrEqual(GAP_MAX)
})

test('подбор без кассы: курсор уходит в поле «Касса», запросов нет', async ({ page, apiAs }) => {
  const api = await apiAs('invoicer')
  await resultWithoutPrices(page, api)
  await page.getByLabel('Касса растаможки, USD').fill('')
  let calls = 0
  page.on('request', (request) => { if (request.url().includes('/api/goods/calculate') || request.url().includes('/price-weights')) calls++ })
  await page.getByRole('button', { name: 'Подобрать цены ИИ' }).click({ force: true })
  await expect(page.getByLabel('Касса растаможки, USD')).toBeFocused()
  expect(calls).toBe(0)
})
