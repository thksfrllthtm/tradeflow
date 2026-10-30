import type { APIRequestContext, Page } from '@playwright/test'
import { test, expect, as } from '../../support/fixtures'
import { E2E } from '../../support/users'
import { CODE_PLASTIC, CODE_TOYS, COL, cell, createEstimate, deleteEstimate, item, mockNbk, mockTrademarksEmpty, seedDraft, sheetHost, typeInCell } from '../../support/goods-ui'

// Лист «Проверка строк» (Jspreadsheet CE): ввод, Delete, отмена/повтор, контекстное меню, вставка из буфера,
// подсветка неверного кода и проблема ТРОИС у наименования.
test.use(as('invoicer'))

const created: number[] = []
test.afterEach(async ({ apiAs }) => {
  const api = await apiAs('invoicer')
  for (const id of created.splice(0)) await deleteEstimate(api, id)
})

const rowsOf = (page: Page) => sheetHost(page).locator('tbody tr[data-y]')

// Смета с черновиком на сервере, открытая на этапе «Проверка строк».
async function openSeeded(page: Page, api: APIRequestContext, items: Record<string, unknown>[]) {
  const estimate = await createEstimate(api)
  created.push(estimate.id)
  await seedDraft(api, estimate.id, items)
  await page.goto(`/estimates/goods/${estimate.id}`)
  await expect(cell(page, COL.name, 0)).toBeVisible()
  return estimate.id
}

async function serverItems(api: APIRequestContext, id: number) {
  return (await (await api.get(`/api/goods-estimates/${id}`)).json()).estimate.input.items as Array<Record<string, string>>
}

test.beforeEach(async ({ page }) => { await mockNbk(page) })

test('ввод кода ТН ВЭД в ячейку подтягивает ставку пошлины из номенклатуры', async ({ page, apiAs }) => {
  await mockTrademarksEmpty(page)
  const api = await apiAs('invoicer')
  const id = await openSeeded(page, api, [item(0)])
  await expect(cell(page, COL.code, 0)).toContainText('Указать код')

  await typeInCell(page, COL.code, 0, CODE_TOYS)
  await expect(cell(page, COL.code, 0)).toContainText('9503 00 700 0')
  await expect(cell(page, COL.duty, 0)).toContainText('10%')

  await expect.poll(async () => (await serverItems(api, id))[0].code, { timeout: 15_000 }).toBe(CODE_TOYS)
})

for (const [label, typed] of [['с пробелами', '3926 90 970 9'], ['из Excel как число («.0»)', '3926909709.0']]) {
  test(`код ${label} нормализуется в 10 цифр`, async ({ page, apiAs }) => {
    await mockTrademarksEmpty(page)
    const api = await apiAs('invoicer')
    const id = await openSeeded(page, api, [item(0)])
    await typeInCell(page, COL.code, 0, typed)
    await expect(cell(page, COL.code, 0)).toContainText('3926 90 970 9')
    await expect(cell(page, COL.duty, 0)).toContainText('6,5%')
    await expect.poll(async () => (await serverItems(api, id))[0].code, { timeout: 15_000 }).toBe(CODE_PLASTIC)
  })
}

test('неверный код (нет в номенклатуре) подсвечивается, пошлины нет', async ({ page, apiAs }) => {
  await mockTrademarksEmpty(page)
  const api = await apiAs('invoicer')
  await openSeeded(page, api, [item(0)])
  await typeInCell(page, COL.code, 0, '0000000001')
  await expect(cell(page, COL.duty, 0)).toHaveText('—')
  await expect(cell(page, COL.code, 0)).toHaveClass(/xs-(err|warn)/)
  await expect(cell(page, COL.code, 0)).toHaveAttribute('title', /номенклатур/)
  await expect(page.getByRole('tab', { name: /Требуют внимания\s*1/ })).toBeEnabled()
})

test('строка без кода — «Нет кода» в фильтре проблем; F8 ведёт к ячейке кода', async ({ page, apiAs }) => {
  await mockTrademarksEmpty(page)
  const api = await apiAs('invoicer')
  await openSeeded(page, api, [item(0, { code: CODE_PLASTIC }), item(1)])
  const chip = page.locator('.rv-pchip').filter({ hasText: 'Нет кода' })
  await expect(chip).toContainText('1')
  await chip.click()
  await expect(chip).toHaveAttribute('aria-pressed', 'true')
  await page.keyboard.press('F8')
  await expect(cell(page, COL.code, 1)).toHaveClass(/highlight|selected/)
})

test('Delete очищает ячейку, Ctrl+Z возвращает, Ctrl+Y повторяет', async ({ page, apiAs }) => {
  await mockTrademarksEmpty(page)
  const api = await apiAs('invoicer')
  await openSeeded(page, api, [item(0, { code: CODE_PLASTIC, pairs: '77' })])
  await expect(cell(page, COL.pairs, 0)).toHaveText('77')

  await cell(page, COL.name, 0).click({ position: { x: 4, y: 4 } })
  for (let x = COL.name; x < COL.pairs; x++) await page.keyboard.press('ArrowRight')
  await page.keyboard.press('Delete')
  await expect(cell(page, COL.pairs, 0)).not.toHaveText('77')

  await page.keyboard.press('Control+z')
  await expect(cell(page, COL.pairs, 0)).toHaveText('77')

  await page.keyboard.press('Control+y')
  await expect(cell(page, COL.pairs, 0)).not.toHaveText('77')
})

test('правка наименования двойным щелчком и отмена Ctrl+Z', async ({ page, apiAs }) => {
  await mockTrademarksEmpty(page)
  const api = await apiAs('invoicer')
  await openSeeded(page, api, [item(0, { code: CODE_PLASTIC })])
  await typeInCell(page, COL.name, 0, `${E2E} новое имя`)
  await expect(cell(page, COL.name, 0)).toContainText(`${E2E} новое имя`)
  await page.keyboard.press('Control+z')
  await expect(cell(page, COL.name, 0)).toContainText(`${E2E} товар 1`)
})

test('контекстное меню: вставить строку ниже и удалить строку', async ({ page, apiAs }) => {
  await mockTrademarksEmpty(page)
  const api = await apiAs('invoicer')
  const id = await openSeeded(page, api, [item(0, { code: CODE_PLASTIC })])
  await expect(rowsOf(page)).toHaveCount(1)

  await cell(page, COL.name, 0).click({ button: 'right', position: { x: 4, y: 4 } })
  const menu = page.locator('.jss_contextmenu')
  await menu.getByText('Вставить строку ниже').click()
  await expect(rowsOf(page)).toHaveCount(2)
  await expect.poll(async () => (await serverItems(api, id)).length, { timeout: 15_000 }).toBe(2)

  await cell(page, COL.name, 1).click({ button: 'right', position: { x: 4, y: 4 } })
  await menu.getByText('Удалить строку').click()
  await expect(rowsOf(page)).toHaveCount(1)
  // Удалённую строку можно вернуть.
  await expect(page.getByRole('status').filter({ hasText: 'Вернуть' })).toBeVisible()
  await page.getByRole('button', { name: 'Вернуть' }).click()
  await expect(rowsOf(page)).toHaveCount(2)
})

test('вставка из буфера (как из Excel): длинная вставка добавляет строки', async ({ page, apiAs }) => {
  await mockTrademarksEmpty(page)
  const api = await apiAs('invoicer')
  const id = await openSeeded(page, api, [item(0)])
  await cell(page, COL.name, 0).click({ position: { x: 4, y: 4 } })

  // Headless-браузер не даёт системный буфер: то же событие paste, что шлёт браузер при Ctrl+V.
  const text = [`${E2E} вставка А\t${CODE_TOYS}`, `${E2E} вставка Б\t${CODE_PLASTIC}`, `${E2E} вставка В\t`].join('\r\n')
  await page.evaluate((value) => {
    const data = new DataTransfer()
    data.setData('text', value)
    document.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }))
  }, text)

  await expect(rowsOf(page)).toHaveCount(3)
  await expect(cell(page, COL.name, 0)).toContainText(`${E2E} вставка А`)
  await expect(cell(page, COL.name, 2)).toContainText(`${E2E} вставка В`)
  await expect(cell(page, COL.duty, 0)).toContainText('10%')
  await expect(cell(page, COL.duty, 1)).toContainText('6,5%')
  await expect.poll(async () => (await serverItems(api, id)).map((row) => row.code), { timeout: 15_000 }).toEqual([CODE_TOYS, CODE_PLASTIC, ''])
})

test('флажок «В расчёте» исключает строку из итогов', async ({ page, apiAs }) => {
  await mockTrademarksEmpty(page)
  const api = await apiAs('invoicer')
  await openSeeded(page, api, [item(0, { code: CODE_PLASTIC }), item(1, { code: CODE_TOYS })])
  await expect(sheetHost(page).locator('.xs-foot')).toContainText('2 из 2')
  await cell(page, COL.included, 1).click()
  await expect(sheetHost(page).locator('.xs-foot')).toContainText('1 из 2')
})

test('поиск по строкам фильтрует лист', async ({ page, apiAs }) => {
  await mockTrademarksEmpty(page)
  const api = await apiAs('invoicer')
  await openSeeded(page, api, [item(0, { name: `${E2E} яблоко`, code: CODE_PLASTIC }), item(1, { name: `${E2E} груша`, code: CODE_TOYS })])
  await page.getByLabel('Найти товар или код').fill('груша')
  await expect(sheetHost(page).locator('tbody tr[data-y]:visible')).toHaveCount(1)
  await expect(sheetHost(page).locator('tbody tr[data-y]:visible')).toContainText('груша')
})

test('ТРОИС: знак из реестра в наименовании — проблема «ТРОИС» у строки', async ({ page, apiAs }) => {
  // Реестр в локальной БД может быть пуст — ответ сервера подставлен.
  await page.route(/\/api\/trademarks\/check/, async (route) => {
    const rows = (route.request().postDataJSON()?.rows ?? []) as Array<{ texts: string[] }>
    await route.fulfill({ json: { results: rows.map((row) => row.texts.some((value) => /SONY/i.test(value)) ? [{
      regNo: '9999', name: 'SONY', fromLogo: false, hasLogo: false, holder: 'Sony Group Corporation', goods: 'электроника', kind: 'brand', found: 'SONY', sameGoods: true,
    }] : []) } })
  })
  const api = await apiAs('invoicer')
  await openSeeded(page, api, [item(0, { name: `${E2E} наушники SONY`, code: CODE_PLASTIC }), item(1, { code: CODE_TOYS })])
  const chip = page.locator('.rv-pchip').filter({ hasText: 'ТРОИС' })
  await expect(chip).toContainText('1')
  await expect(cell(page, COL.name, 0)).toHaveClass(/xs-warn/)
  await expect(cell(page, COL.name, 0)).toHaveAttribute('title', /№ 9999/)
  await expect(cell(page, COL.name, 1)).not.toHaveClass(/xs-warn/)
})
