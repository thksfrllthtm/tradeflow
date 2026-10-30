import { readFileSync } from 'node:fs'
import ExcelJS from 'exceljs'
import type { APIRequestContext, Page, Route } from '@playwright/test'
import { test, expect, as } from '../../support/fixtures'
import { E2E, uid } from '../../support/users'

// Керамика (/ceramics, /ceramics/base). Подбор по файлу: известные марки — поиском в базе
// (без ИИ), неизвестные — /api/ceramics/analyze, здесь он всегда подменён (мок или 503 из фикстуры).

const XLSX = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'

async function xlsx(rows: string[][]) {
  const book = new ExcelJS.Workbook()
  const sheet = book.addWorksheet('Лист1')
  rows.forEach((row) => sheet.addRow(row))
  return Buffer.from(await book.xlsx.writeBuffer())
}

// Марки этого теста; после теста администратор удаляет их из базы.
const used: string[] = []
const mark = (kind = 'CER') => { const value = `${E2E}-${kind}-${uid()}`.toUpperCase(); used.push(value); return value }

async function cleanup(admin: APIRequestContext) {
  for (const value of used.splice(0)) {
    const list = await (await admin.get(`/api/ceramics/products?q=${encodeURIComponent(value)}`)).json()
    for (const item of list.items) if (item.mark === value) await admin.delete(`/api/ceramics/products/${item.id}`)
  }
}

test.afterEach(async ({ apiAs }) => cleanup(await apiAs('admin')))

// Ответ ИИ-подбора в формате Resolution (frontend/src/features/ceramics/types.ts).
function aiResolution(key: string, mark: string) {
  return {
    key, mark, markKey: key, status: 'AUTO', productType: 'Пиала керамическая', typeStatus: 'MATCHED',
    typeCandidates: [{ productType: 'Пиала керамическая', confidence: 0.9, reason: 'по фото и названию' }],
    size: { value: '5', unit: 'inch' }, sizeStatus: 'MATCHED', sizeOptions: [],
    price: { status: 'MATCHED', price: '3.3', options: [{ price: '3.3', count: 4 }], total: 4, availableSizes: [], hasUnsized: false, message: '', name: 'Пиала керамическая 5 дюймов' },
    displayName: 'Пиала керамическая 5 дюймов', chineseTerms: [{ text: '陶瓷碗', meaning: 'керамическая пиала' }],
    evidence: [{ field: 'type', source: 'ai', text: 'ИИ: пиала' }], warnings: [], aiUsed: true,
  }
}

async function mockAnalyze(page: Page) {
  const calls: unknown[] = []
  await page.route('**/api/ceramics/analyze', async (route: Route) => {
    const body = route.request().postDataJSON() as { items: { key: string; mark: string }[] }
    calls.push(body)
    await route.fulfill({ json: { items: body.items.map((item) => aiResolution(item.key, item.mark)) } })
  })
  return calls
}

test.describe('подбор по файлу', () => {
  test.use(as('invoicer'))

  async function prepare(page: Page, apiAs: (who: 'invoicer') => Promise<APIRequestContext>) {
    const known = mark()
    const unknown = mark('UNK')
    const created = await (await apiAs('invoicer')).post('/api/ceramics/products', { data: { mark: known, name: 'Тарелка керамическая 8 дюймов/ 陶瓷盘8寸', price: '7.7' } })
    expect(created.status()).toBe(201)
    const file = await xlsx([['Марка', 'Китайское наименование', 'Количество'], [known, '陶瓷盘8寸', '10'], [unknown, '陶瓷碗', '5'], [known, '陶瓷盘8寸', '3']])
    await page.goto('/ceramics')
    await expect(page.getByRole('heading', { name: 'Керамика: наименование и цена' })).toBeVisible()
    await page.locator('input[type=file]').first().setInputFiles({ name: `${E2E} клиент.xlsx`, mimeType: XLSX, buffer: file })
    await expect(page.getByText('К подбору')).toContainText('3 из 3')
    return { known, unknown }
  }

  test('известная марка — из базы, неизвестная — ИИ (мок); фильтры, детали, CSV', async ({ page, apiAs }) => {
    const calls = await mockAnalyze(page)
    const { known, unknown } = await prepare(page, apiAs)
    await page.getByRole('button', { name: /^Подобрать/ }).click()

    const results = page.getByRole('region', { name: 'Результат подбора' })
    const knownRow = results.locator('tr.cer-row', { hasText: known })
    const unknownRow = results.locator('tr.cer-row', { hasText: unknown })
    await expect(knownRow).toContainText('По марке')
    await expect(knownRow).toContainText('×2') // марка в двух строках файла — подбирается один раз
    await expect(knownRow).toContainText('7.7')
    await expect(unknownRow).toContainText('Авто')
    await expect(unknownRow).toContainText('Пиала керамическая 5 дюймов/ 陶瓷碗')
    await expect(unknownRow).toContainText('3.3')

    // В ИИ ушла только неизвестная марка.
    expect(calls).toHaveLength(1)
    expect((calls[0] as { items: { mark: string }[] }).items.map((item) => item.mark)).toEqual([unknown])

    await results.getByRole('tab', { name: /По марке/ }).click()
    await expect(results.locator('tr.cer-row')).toHaveCount(1)
    await results.getByRole('tab', { name: /Авто/ }).click()
    await expect(results.locator('tr.cer-row')).toHaveCount(1)
    await results.getByRole('tab', { name: /Все/ }).click()
    await expect(results.locator('tr.cer-row')).toHaveCount(2)

    // Подробнее → «Готово» отмечает строку проверенной.
    await unknownRow.getByRole('button', { name: 'Подробнее' }).click()
    await page.getByRole('button', { name: /Готово/ }).click()
    await expect(unknownRow).toContainText('проверено')

    const [download] = await Promise.all([page.waitForEvent('download'), results.getByRole('button', { name: 'Скачать CSV' }).click()])
    expect(download.suggestedFilename()).toBe(`${E2E} клиент — подбор.csv`)
    const csv = readFileSync(await download.path(), 'utf8')
    expect(csv.startsWith('﻿Строка Excel;Марка')).toBe(true)
    expect(csv).toContain(`2;${known};陶瓷盘8寸;Тарелка керамическая 8 дюймов/ 陶瓷盘8寸;7.7`)
    expect(csv).toContain(`4;${known}`)
    expect(csv).toContain(`3;${unknown}`)
    expect(csv).toContain('✔ Проверено')
  })

  test('ИИ недоступен: строка «Ошибка», «Повторить» после восстановления', async ({ page, apiAs, aiBlocked }) => {
    const { unknown } = await prepare(page, apiAs)
    await page.getByRole('button', { name: /^Подобрать/ }).click()
    const unknownRow = page.locator('tr.cer-row', { hasText: unknown })
    await expect(unknownRow).toContainText('Ошибка')
    expect(aiBlocked.some((url) => url.includes('/api/ceramics/analyze'))).toBe(true)

    await mockAnalyze(page)
    await unknownRow.getByRole('button', { name: 'Повторить' }).click()
    await expect(unknownRow).toContainText('Авто')
  })

  test('результат подбора не теряется при переходе в базу и обратно', async ({ page, apiAs }) => {
    await mockAnalyze(page)
    const { known } = await prepare(page, apiAs)
    await page.getByRole('button', { name: /^Подобрать/ }).click()
    await expect(page.locator('tr.cer-row', { hasText: known })).toContainText('По марке')
    await page.getByRole('link', { name: /База марок/ }).click()
    await expect(page).toHaveURL(/\/ceramics\/base$/)
    await page.getByRole('link', { name: 'Подбор по файлу' }).click()
    await expect(page).toHaveURL(/\/ceramics$/)
    await expect(page.locator('tr.cer-row', { hasText: known })).toBeVisible()
  })

  test('ИИ не настроен — предупреждение', async ({ page }) => {
    await page.route('**/api/ceramics/status', (route) => route.fulfill({ json: { ai: { configured: false, vision: false } } }))
    await page.goto('/ceramics')
    await expect(page.getByText('ИИ не настроен на сервере')).toBeVisible()
  })

  test('файл без марок и китайских названий — сообщение', async ({ page }) => {
    await page.goto('/ceramics')
    const file = await xlsx([['Колонка А', 'Колонка Б'], ['', '1'], ['', '2']])
    await page.locator('input[type=file]').first().setInputFiles({ name: 'пусто.xlsx', mimeType: XLSX, buffer: file })
    await expect(page.getByRole('button', { name: /^Подобрать/ })).toBeDisabled()
  })
})

test.describe('база марок: администратор', () => {
  test.use(as('admin'))

  test('добавить, найти, исправить и удалить марку', async ({ page }) => {
    const code = mark()
    await page.goto('/ceramics/base')
    await page.getByRole('button', { name: '+ Добавить марку' }).click()
    await page.getByLabel('Наименование *').fill('Кружка керамическая 300 мл/ 陶瓷杯')
    await page.getByLabel('Марка *').fill(code)
    await page.getByLabel('Цена *').fill('2,5')
    await page.getByRole('button', { name: 'Сохранить' }).click()
    await expect(page.getByRole('status')).toContainText(`Марка ${code} добавлена`)

    await page.getByLabel('Поиск по базе').fill(code)
    const row = page.locator('tbody tr', { hasText: code })
    await expect(row).toHaveCount(1)
    await expect(row).toContainText('2.5')
    await expect(row).toContainText('300')

    await row.click()
    await expect(page.getByText('Распознано из наименования')).toBeVisible()
    await page.getByLabel('Цена *').fill('3')
    await page.getByRole('button', { name: 'Сохранить' }).click()
    await expect(page.getByRole('status')).toContainText(`Марка ${code} сохранена`)
    await expect(row).toContainText('3')

    page.once('dialog', (dialog) => dialog.dismiss())
    await row.getByRole('button', { name: 'Удалить' }).click()
    await expect(row).toHaveCount(1)
    page.once('dialog', (dialog) => { expect(dialog.message()).toContain(code); void dialog.accept() })
    await row.getByRole('button', { name: 'Удалить' }).click()
    await expect(page.getByRole('status')).toContainText(`Марка ${code} удалена`)
    await expect(page.getByText('Ничего не найдено.')).toBeVisible()
  })

  test('повтор марки — ошибка в форме', async ({ page, apiAs }) => {
    const code = mark()
    await (await apiAs('admin')).post('/api/ceramics/products', { data: { mark: code, name: 'Чашка керамическая', price: '1' } })
    await page.goto('/ceramics/base')
    await page.getByRole('button', { name: '+ Добавить марку' }).click()
    await page.getByLabel('Наименование *').fill('Чашка керамическая')
    await page.getByLabel('Марка *').fill(code)
    await page.getByLabel('Цена *').fill('1')
    await page.getByRole('button', { name: 'Сохранить' }).click()
    await expect(page.locator('form').getByRole('alert')).toContainText('уже есть в базе')
    await page.getByRole('button', { name: 'Отмена' }).click()
    await expect(page.locator('form')).toHaveCount(0)
  })

  test('загрузка базы из Excel: проверка строк, загружаются только верные', async ({ page }) => {
    const [good, badPrice] = [mark(), mark()]
    const file = await xlsx([['Наименование', 'Марка', 'Цена'], ['Ваза керамическая 10 дюймов/ 陶瓷花瓶', good, '15'], ['Ваза керамическая', badPrice, 'abc'], ['Ваза без марки', '', '5']])
    await page.goto('/ceramics/base')
    await page.getByRole('button', { name: 'Загрузить из Excel' }).click()
    await page.locator('.cer-import input[type=file]').setInputFiles({ name: 'прайс.xlsx', mimeType: XLSX, buffer: file })
    const card = page.locator('.cer-import')
    await expect(card).toContainText('Готово к загрузке: 1, с ошибками (не загрузятся): 2')
    await expect(card).toContainText('цена «abc» не распознана')
    await expect(card).toContainText('нет марки')
    await card.getByRole('button', { name: 'Загрузить 1' }).click()
    await expect(page.getByRole('status')).toContainText('Загружено: новых 1, обновлено 0, пропущено существующих 0')
    await page.getByLabel('Поиск по базе').fill(good)
    await expect(page.locator('tbody tr', { hasText: good })).toHaveCount(1)
  })
})

test.describe('база марок: инвойсер', () => {
  test.use(as('invoicer'))

  test('добавляет марку, но не загружает Excel и не удаляет', async ({ page }) => {
    const code = mark()
    await page.goto('/ceramics/base')
    await expect(page.getByRole('button', { name: '+ Добавить марку' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Загрузить из Excel' })).toHaveCount(0)
    await page.getByRole('button', { name: '+ Добавить марку' }).click()
    await page.getByLabel('Наименование *').fill('Блюдце керамическое')
    await page.getByLabel('Марка *').fill(code)
    await page.getByLabel('Цена *').fill('1')
    await page.getByRole('button', { name: 'Сохранить' }).click()
    await expect(page.getByRole('status')).toContainText(`Марка ${code} добавлена`)
    await page.getByLabel('Поиск по базе').fill(code)
    const row = page.locator('tbody tr', { hasText: code })
    await expect(row).toHaveCount(1)
    await expect(row.getByRole('button', { name: 'Удалить' })).toHaveCount(0)
  })
})
