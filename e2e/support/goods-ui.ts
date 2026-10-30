import type { APIRequestContext, Locator, Page } from '@playwright/test'
import { expect } from '@playwright/test'
import { E2E, uid } from './users'

// Помощники UI-тестов смет товаров (/estimates/goods).

// Коды из номенклатуры локальной БД: обычная процентная пошлина.
export const CODE_PLASTIC = '3926909709' // 6,5%
export const CODE_TOYS = '9503007000' // 10%
export const USD_RATE = '500'

export async function createEstimate(api: APIRequestContext, name = `${E2E}-${uid()}`) {
  const response = await api.post('/api/goods-estimates', { data: { name, client: `${E2E} клиент`, assessmentDate: today() } })
  expect(response.status(), await response.text()).toBe(201)
  const estimate = (await response.json()).estimate as { id: number; name: string }
  return estimate
}

export async function deleteEstimate(api: APIRequestContext, id: number) {
  await api.delete(`/api/goods-estimates/${id}`)
}

export const today = () => new Date(Date.now() + 5 * 3600_000).toISOString().slice(0, 10)

// Строка сметы в том виде, в каком её собирает этап «Файл и колонки».
export function item(index: number, overrides: Record<string, unknown> = {}) {
  return {
    id: `e2e-row-${index}`, sourceRow: index + 2, name: `${E2E} товар ${index + 1}`, supplierName: `${E2E} товар ${index + 1}`, standardName: '', code: '',
    pairs: '10', places: '1', pairsPerPlace: '', grossKg: '5', grossWeightUnit: 'kg', netKg: '4', netWeightUnit: 'kg', netSource: 'excel', packageWeightKg: '0.3',
    coefficient: '', deliveryUsd: '', deliveryManual: false, deliveryIncluded: false, excluded: false, exclusionReason: '', deliveryAlreadyInCost: false,
    pricePerPair: '2', sourcePricePerPair: '2', costInput: '', costCurrency: '', extraExpenses: '', customsValueDirectKzt: '', photoUrl: '', photoUrls: [],
    ...overrides,
  }
}

// Черновик сметы на сервере: параметры и строки (сразу этап «Проверка строк»).
export async function seedDraft(api: APIRequestContext, id: number, items: Record<string, unknown>[], params: Record<string, unknown> = {}) {
  const input = { assessmentDate: today(), usdKzt: USD_RATE, deliveryMode: 'none', packageWeightKg: '0.3', mrp: '4325', feeMultiplier: '6', customsValueMode: 'invoice', origin: '', ...params, items }
  const response = await api.patch(`/api/goods-estimates/${id}/draft`, { data: { input } })
  expect(response.ok(), await response.text()).toBeTruthy()
}

// Курс НБРК — фиксированный: расчёт не зависит от сайта НБРК и даты.
export async function mockNbk(page: Page, rate = USD_RATE) {
  await page.route(/\/api\/rates\/nbk/, (route) => route.fulfill({ json: { rate: { currency: 'USD', rate, date: today(), source: 'НБРК' } } }))
}

// Реестр ТРОИС в этих тестах пуст, если тест не подставил свой ответ.
export async function mockTrademarksEmpty(page: Page) {
  await page.route(/\/api\/trademarks\/check/, async (route) => {
    const rows = (route.request().postDataJSON()?.rows ?? []) as unknown[]
    await route.fulfill({ json: { results: rows.map(() => []) } })
  })
}

export const sheetHost = (page: Page) => page.locator('.xs-host')
// Ячейка листа «Проверка строк»: x — номер колонки SHEET_COLUMNS, y — строка (с нуля).
export const COL = { photo: 0, name: 1, code: 2, duty: 3, places: 4, pairsPerPlace: 5, pairs: 6, grossKg: 7, packageWeightKg: 8, netKg: 9, included: 10 }
export const cell = (page: Page, x: number, y: number): Locator => sheetHost(page).locator(`tbody td[data-x="${x}"][data-y="${y}"]`)

// Ввод в ячейку листа: выделить её (щелчок по соседней «Товар» и стрелки — чтобы не попасть в кнопки ячейки) и печатать.
export async function typeInCell(page: Page, x: number, y: number, text: string) {
  await cell(page, COL.name, y).click({ position: { x: 4, y: 4 } })
  for (let step = COL.name; step < x; step++) await page.keyboard.press('ArrowRight')
  for (let step = COL.name; step > x; step--) await page.keyboard.press('ArrowLeft')
  await page.keyboard.type(text)
  await page.keyboard.press('Enter')
}

// Назначить колонке файла поле сметы: щелчок по заголовку листа «Как в Excel» и пункт списка.
export async function mapColumn(page: Page, header: string, field: string) {
  await page.locator('.xp-host thead td.xp-pick').filter({ hasText: new RegExp(`^${header}$`) }).click()
  const menu = page.getByRole('listbox', { name: `Что в колонке «${header}»` })
  if (field === 'Основное') await menu.getByRole('button', { name: 'Наименование' }).click()
  await menu.getByRole('option', { name: new RegExp(`^${field}`) }).click()
  await expect(menu).toBeHidden()
}

// Сумма как на экране: «12 345,67».
export function ruNumber(value: string) {
  const [integer, fraction] = value.split('.')
  return integer.replace(/\B(?=(\d{3})+(?!\d))/g, ' ') + (fraction ? `,${fraction}` : '')
}
