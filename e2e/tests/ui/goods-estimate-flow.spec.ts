import { readFileSync } from 'node:fs'
import { test, expect, as } from '../../support/fixtures'
import { authFile, E2E, uid } from '../../support/users'
import { supplierXlsx } from '../../support/xlsx-ui'
import { CODE_PLASTIC, CODE_TOYS, COL, cell, createEstimate, deleteEstimate, item, mapColumn, mockNbk, mockTrademarksEmpty, ruNumber, seedDraft } from '../../support/goods-ui'

// Полный путь инвойсера по смете товаров: файл → проверка строк → результат → Excel → автосохранение.
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

test('новая смета: Excel с фото → колонки → проверка → расчёт → Excel → перезагрузка → удаление', async ({ page }) => {
  test.setTimeout(120_000)
  const name = `${E2E}-${uid()}`
  page.on('dialog', (dialog) => void dialog.accept())

  // Начальный экран и создание сметы кнопкой.
  await page.goto('/estimates/goods')
  await expect(page.getByRole('heading', { name: 'Начните с новой сметы' })).toBeVisible()
  const createdResponse = page.waitForResponse((response) => response.url().endsWith('/api/goods-estimates') && response.request().method() === 'POST')
  await page.getByRole('button', { name: '+ Создать смету' }).click()
  const estimateId = (await (await createdResponse).json()).estimate.id as number
  created.push(estimateId)
  await expect(page).toHaveURL(new RegExp(`/estimates/goods/${estimateId}$`))

  // Этап 1: загрузка Excel поставщика (3 строки, фото в первой).
  const file = await supplierXlsx([
    { name: 'Контейнер пластиковый', code: CODE_PLASTIC, qty: 100, gross: 55, net: 50, price: 1.5, photo: true },
    { name: 'Игрушка-самокат', code: CODE_TOYS, qty: 20, gross: 44, net: 40, price: 12 },
    { name: 'Лишняя строка', code: '', qty: 1, gross: 1, net: 1, price: 1 },
  ])
  const parsed = page.waitForResponse((response) => response.url().includes('/api/goods/parse-xlsx'))
  await page.locator('input[type=file]').first().setInputFiles({ name: 'invoice.xlsx', mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', buffer: file })
  const parseBody = await (await parsed).json()
  expect(parseBody.sheets.map((sheet: { name: string }) => sheet.name)).toEqual(['Инвойс', 'Прочее'])
  expect(parseBody.images.length, 'фото из файла прочитано').toBe(1)
  await expect(page.getByText(/К импорту\s*3\s*из 3 строк/)).toBeVisible()
  await expect(page.getByText('Выберите колонку «Наименование»')).toBeVisible()

  // Название и клиент — в шапке сметы (она появляется после загрузки файла).
  await page.locator('.estimate-bar-editor summary').click()
  await page.getByLabel('Название').fill(name)
  await page.getByLabel('Клиент').fill(`${E2E} ООО Клиент`)
  await page.locator('.estimate-bar-fields').getByRole('button', { name: 'Сохранить' }).click()
  await expect(page.getByRole('heading', { level: 1, name })).toBeVisible()

  // Источник: файл и выбор листа.
  await page.locator('.im-source').getByRole('button', { name: 'Изменить' }).click()
  const sheets = page.getByRole('radiogroup', { name: 'Лист файла' })
  await expect(sheets.getByRole('radio', { name: 'Инвойс' })).toHaveAttribute('aria-checked', 'true')
  await expect(sheets.getByRole('radio', { name: 'Прочее' })).toBeVisible()
  await page.keyboard.press('Escape')

  // Колонки: щелчок по заголовку и поле сметы.
  await mapColumn(page, 'Наименование', 'Наименование')
  await mapColumn(page, 'Код ТН ВЭД', 'Код ТН ВЭД')
  await mapColumn(page, 'Количество', 'Кол-во, шт / пар')
  await mapColumn(page, 'Брутто', 'Брутто, кг')
  await mapColumn(page, 'Нетто', 'Нетто, кг')
  await mapColumn(page, 'Цена', 'Цена, USD')
  await expect(page.getByText('Выберите колонку «Наименование»')).toBeHidden()
  await expect(page.locator('.xp-host thead td.xp-mapped')).toHaveCount(6)
  await expect(page.locator('.xp-host thead td').filter({ hasText: 'Наименование ← Наименование' })).toBeVisible()

  // Третью строку не импортируем (флажок в первой колонке).
  await page.locator('.xp-host tbody td[data-x="0"][data-y="2"]').click()
  await expect(page.getByText(/К импорту\s*2\s*из 3 строк/)).toBeVisible()

  await page.getByRole('button', { name: /Применить и перейти к проверке/ }).click()

  // Этап 2: коды сверены с номенклатурой, ставки в колонке «Пошлина», фото у первой строки.
  await expect(cell(page, COL.name, 0)).toContainText('Контейнер пластиковый')
  await expect(cell(page, COL.name, 1)).toContainText('Игрушка-самокат')
  await expect(cell(page, COL.name, 2)).toHaveCount(0)
  await expect(cell(page, COL.duty, 0)).toContainText('6,5%')
  await expect(cell(page, COL.duty, 1)).toContainText('10%')
  await expect(cell(page, COL.photo, 0).locator('img')).toBeVisible()

  // Параметры расчёта: курс (НБРК подставлен), доставка суммой, сбор, страна.
  await expect(page.locator('.rv-params-item').filter({ hasText: 'Курс USD' })).toContainText('500')
  await page.getByRole('button', { name: 'Изменить параметры' }).click()
  const form = page.getByRole('group', { name: 'Параметры расчёта' })
  await form.getByRole('radio', { name: 'Сумма' }).click()
  await form.getByLabel('Доставка за партию').fill('100')
  await form.getByLabel('Доставка за партию').press('Enter')
  await form.getByLabel('Множитель сбора (0 — без сбора)').fill('6')
  await form.getByLabel('Множитель сбора (0 — без сбора)').press('Enter')
  await form.getByLabel('Страна происхождения').fill('Китай')
  await form.getByRole('option', { name: /Китай/ }).click()
  await page.getByRole('button', { name: 'Изменить параметры' }).click()
  await expect(page.locator('.rv-params-item').filter({ hasText: 'Доставка' })).toContainText('100')
  await expect(page.locator('.rv-params-item').filter({ hasText: 'Страна' })).toContainText('Китай')

  // Этап 3: расчёт на сервере; итог на экране — из ответа сервера.
  const calculated = page.waitForResponse('**/api/goods/calculate')
  await page.getByRole('button', { name: /^Рассчитать/ }).click()
  const calcResponse = await calculated
  const requestBody = calcResponse.request().postDataJSON()
  expect(requestBody.usdKzt).toBe('500')
  expect(requestBody.deliveryMode).toBe('total')
  expect(requestBody.deliveryTotalUsd).toBe('100')
  expect(requestBody.items.map((row: { code: string }) => row.code)).toEqual([CODE_PLASTIC, CODE_TOYS])
  const result = await calcResponse.json()
  const total = String(result.totals.customsPaymentsKzt)
  expect(Number(total)).toBeGreaterThan(0)
  const summary = page.getByRole('region', { name: 'Итоги расчёта' })
  await expect(summary).toContainText(ruNumber(total))
  await expect(summary).toContainText(ruNumber(String(result.totals.vatKzt)))
  await expect(summary).toContainText(ruNumber(String(result.totals.dutyKzt)))
  await expect(page.getByRole('region', { name: 'Строки расчёта' })).toContainText('Контейнер пластиковый')

  // Выгрузка в Excel.
  const downloadPromise = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Экспорт в Excel' }).click()
  const download = await downloadPromise
  expect(download.suggestedFilename()).toMatch(new RegExp(`^${name}.*\\.xlsx$`))
  const bytes = readFileSync(await download.path())
  expect(bytes.length).toBeGreaterThan(2000)
  expect(bytes.subarray(0, 2).toString()).toBe('PK')

  // Автосохранение: дождаться отметки «Сохранено», перезагрузить — результат и итог на месте.
  await expect(page.locator('.rs-save-state')).toContainText('Сохранено', { timeout: 20_000 })
  await page.reload()
  await expect(page.getByRole('region', { name: 'Итоги расчёта' })).toContainText(ruNumber(total))
  await expect(page.getByRole('heading', { level: 1, name })).toBeVisible()

  // Смета на главной с отметкой «Рассчитано».
  await page.goto('/')
  const homeRow = page.locator('.home-row').filter({ hasText: name })
  await expect(homeRow).toBeVisible()
  await expect(homeRow).toContainText('Рассчитано')
  await homeRow.click()
  await expect(page).toHaveURL(new RegExp(`/estimates/goods/${estimateId}$`))

  // Удаление из списка смет (подтверждение принимается).
  await page.locator('.es-rail-button').click()
  await page.getByRole('button', { name: `Удалить смету ${name}` }).click()
  await expect(page.getByRole('link', { name: new RegExp(name) })).toHaveCount(0)
  await expect(page).not.toHaveURL(new RegExp(`/estimates/goods/${estimateId}$`))
})

test('правка ячейки автосохраняется на сервер и видна после перезагрузки', async ({ page, apiAs }) => {
  const api = await apiAs('invoicer')
  const estimate = await createEstimate(api)
  created.push(estimate.id)
  await seedDraft(api, estimate.id, [item(0, { code: CODE_PLASTIC })])

  await page.goto(`/estimates/goods/${estimate.id}`)
  await expect(cell(page, COL.name, 0)).toContainText(`${E2E} товар 1`)

  const saved = page.waitForResponse((response) => response.url().includes(`/api/goods-estimates/${estimate.id}/draft`) && response.ok())
  await cell(page, COL.name, 0).dblclick()
  await page.keyboard.press('Control+A')
  await page.keyboard.type(`${E2E} переименован`)
  await page.keyboard.press('Enter')
  await saved

  const draft = (await (await api.get(`/api/goods-estimates/${estimate.id}`)).json()).estimate
  expect(draft.input.items[0].name).toBe(`${E2E} переименован`)

  await page.reload()
  await expect(cell(page, COL.name, 0)).toContainText(`${E2E} переименован`)
})

test('название, клиент и дата сметы меняются в шапке и сохраняются', async ({ page, apiAs }) => {
  const api = await apiAs('invoicer')
  const estimate = await createEstimate(api)
  created.push(estimate.id)
  const name = `${E2E}-${uid()} новое`
  await seedDraft(api, estimate.id, [item(0)])

  await page.goto(`/estimates/goods/${estimate.id}`)
  await page.locator('.estimate-bar-editor summary').click()
  await page.getByLabel('Название').fill(name)
  await page.getByLabel('Клиент').fill(`${E2E} Клиент 2`)
  await page.getByLabel('Дата').fill('2026-01-15')
  await page.locator('.estimate-bar-fields').getByRole('button', { name: 'Сохранить' }).click()
  await expect(page.getByRole('heading', { level: 1, name })).toBeVisible()

  const saved = (await (await api.get(`/api/goods-estimates/${estimate.id}`)).json()).estimate
  expect(saved.name).toBe(name)
  expect(saved.client).toBe(`${E2E} Клиент 2`)
  expect(saved.assessmentDate).toBe('2026-01-15')
})

test('пустая смета: строки можно заполнить вручную без файла', async ({ page, apiAs }) => {
  const api = await apiAs('invoicer')
  const estimate = await createEstimate(api)
  created.push(estimate.id)

  const loaded = page.waitForResponse((response) => response.url().endsWith(`/api/goods-estimates/${estimate.id}`) && response.request().method() === 'GET')
  await page.goto(`/estimates/goods/${estimate.id}`)
  await loaded
  await expect(page.getByText(/Перетащите Excel/)).toBeVisible()
  await page.getByRole('button', { name: 'заполните строки вручную' }).click()
  await expect(cell(page, COL.name, 0)).toBeVisible()
  await expect(page.getByRole('button', { name: /^Рассчитать/ })).toBeEnabled()
  await expect(page.getByRole('tab', { name: /Все\s*1/ })).toBeVisible()
})

// Раньше строка, добавленная до ответа сервера, пропадала: ответ заменял строки. Теперь до
// загрузки сметы правок нет — только «Загружаем смету…».
test('медленная загрузка сметы: до ответа сервера правки недоступны, после — строка не пропадает', async ({ page, apiAs }) => {
  const api = await apiAs('invoicer')
  const estimate = await createEstimate(api)
  created.push(estimate.id)
  // Медленный ответ сервера, как после простоя Render.
  await page.route(new RegExp(`/api/goods-estimates/${estimate.id}$`), async (route) => {
    if (route.request().method() !== 'GET') return route.fallback()
    await new Promise((resolve) => setTimeout(resolve, 3000))
    await route.fallback()
  })
  const loaded = page.waitForResponse((response) => response.url().endsWith(`/api/goods-estimates/${estimate.id}`) && response.request().method() === 'GET')
  await page.goto(`/estimates/goods/${estimate.id}`)
  await expect(page.getByRole('status').filter({ hasText: 'Загружаем смету…' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'заполните строки вручную' })).toHaveCount(0)
  await loaded
  await page.getByRole('button', { name: 'заполните строки вручную' }).click()
  await expect(cell(page, COL.name, 0)).toBeVisible()
  // Второй ответ сервера (страница читает смету и для шапки) строку не убирает.
  await page.waitForLoadState('networkidle')
  await expect(cell(page, COL.name, 0), 'добавленная строка на месте').toBeVisible()
})

test('битый файл вместо Excel — понятная ошибка, смета не ломается', async ({ page, apiAs }) => {
  const api = await apiAs('invoicer')
  const estimate = await createEstimate(api)
  created.push(estimate.id)

  await page.goto(`/estimates/goods/${estimate.id}`)
  await page.locator('input[type=file]').first().setInputFiles({ name: 'broken.xlsx', mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', buffer: Buffer.from('это не Excel') })
  await expect(page.getByRole('alert')).toContainText(/.+/)
  await expect(page.getByText(/Перетащите Excel/)).toBeVisible()
})

test('чужая смета: второй инвойсер по прямой ссылке не видит данных сметы', async ({ browser, apiAs }) => {
  const owner = await apiAs('invoicer')
  const estimate = await createEstimate(owner, `${E2E}-${uid()} секрет`)
  created.push(estimate.id)
  await seedDraft(owner, estimate.id, [item(0, { name: `${E2E} секретный товар`, code: CODE_PLASTIC })])

  const other = await apiAs('invoicer2')
  expect((await other.get(`/api/goods-estimates/${estimate.id}`)).status()).toBe(404)

  const context = await browser.newContext({ storageState: authFile('invoicer2') })
  const page = await context.newPage()
  const calls: string[] = []
  page.on('response', (response) => { if (response.url().includes(`/api/goods-estimates/${estimate.id}`)) calls.push(`${response.request().method()} ${response.status()}`) })
  await page.goto(`/estimates/goods/${estimate.id}`)
  // Чужая смета не в списке: приложение уводит на начальный экран.
  await expect(page).toHaveURL(/\/estimates\/goods$/)
  await expect(page.getByText('секретный товар')).toHaveCount(0)
  await expect(page.getByText('секрет')).toHaveCount(0)
  expect(calls.every((call) => !call.endsWith(' 200'))).toBeTruthy()
  await context.close()
})
