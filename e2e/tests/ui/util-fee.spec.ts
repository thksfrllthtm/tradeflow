import type { Page } from '@playwright/test'
import { test, expect, as } from '../../support/fixtures'

// Калькулятор утильсбора (/util-fee): один код и таблица из Excel. Суммы считает сервер;
// здесь проверяется, что страница показывает те же цифры, что API (см. tests/api/util-fee.spec.ts).

test.use(as('invoicer'))

// «5 332,29 KZT» → «5332,29KZT»: пробелы разрядов не важны.
const compact = (text: string | null) => (text ?? '').replace(/\s/g, '')
const total = (page: Page) => page.getByRole('complementary', { name: 'Расчёт утильсбора' }).locator('.uf-calc-total strong')

test.beforeEach(async ({ page }) => {
  // Режим и таблица хранятся в localStorage — каждый тест начинает с чистого «Один товар».
  await page.addInitScript(() => { if (!sessionStorage.getItem('e2e-init')) { localStorage.removeItem('utilFee.mode'); localStorage.removeItem('utilFee.table'); sessionStorage.setItem('e2e-init', '1') } })
  await page.goto('/util-fee')
  await expect(page.getByRole('heading', { name: 'Утильсбор', level: 1 })).toBeVisible()
})

test('перечень целиком: 19 разделов, раскрытие раздела, выбор кода из перечня', async ({ page }) => {
  await expect(page.locator('.uf-count')).toContainText('19 разделов')
  await page.getByRole('button', { name: 'Раскрыть раздел 7' }).click()
  const item = page.locator('tr.uf-item', { hasText: '3924' })
  await expect(item).toBeVisible()
  await page.getByRole('button', { name: 'Свернуть раздел 7' }).click()
  await expect(item).toBeHidden()

  await page.getByRole('button', { name: 'Раскрыть все' }).click()
  await expect(page.getByRole('button', { name: 'Свернуть все' })).toBeVisible()
  await page.locator('tr.uf-item', { hasText: '3924' }).first().click()
  await expect(page.getByLabel('Код ТН ВЭД')).toHaveValue('3924')
  await expect(page.getByRole('heading', { name: /Код 3924 — утильсбор платится за товар/ })).toBeVisible()
})

test('поиск по слову фильтрует перечень', async ({ page }) => {
  await page.getByLabel('Код ТН ВЭД').fill('шины')
  await expect(page.locator('tbody .uf-section-row')).toHaveCount(1)
  await expect(page.locator('.uf-section-title')).toContainText('шины')
  await page.getByLabel('Код ТН ВЭД').fill('несуществующееслово')
  await expect(page.getByText('Ничего не найдено. Код не из перечня')).toBeVisible()
})

test('товар и упаковка: суммы совпадают с API (5 332,29 KZT)', async ({ page, apiAs }) => {
  await page.getByLabel('Код ТН ВЭД').fill('8516500000')
  await expect(page.getByRole('heading', { name: 'Код 8516 50 000 0 — утильсбор платится за товар и за упаковку' })).toBeVisible()
  await expect(page.getByText('Раздел 16, пункт 16.13 · k = 2,00')).toBeVisible()
  await expect(total(page)).toHaveText('—')

  await page.getByLabel('Масса нетто, кг').fill('500')
  await expect.poll(async () => compact(await total(page).textContent())).toBe('4325,00KZT')

  await page.getByLabel('Масса упаковки, кг').fill('120')
  await expect.poll(async () => compact(await total(page).textContent())).toBe('5238,44KZT')

  await page.getByRole('button', { name: '+ Ещё материал' }).click()
  await page.getByLabel('Масса упаковки, кг').nth(1).fill('15,5')
  await expect.poll(async () => compact(await total(page).textContent())).toBe('5332,29KZT')

  // Та же сумма у сервера.
  const api = await (await apiAs('invoicer')).post('/api/util-fee', { data: { code: '8516500000', mrp: '4325', massKg: '500', packaging: [{ material: 'paper', massKg: '120' }, { material: 'polymer', massKg: '15,5' }] } })
  expect((await api.json()).total).toBe('5332.29')

  // Удалить строку упаковки — итог пересчитан.
  await page.getByRole('button', { name: 'Удалить строку' }).nth(1).click()
  await expect.poll(async () => compact(await total(page).textContent())).toBe('5238,44KZT')

  // Другой материал первой строки: стекло k 1,14 → 0,12 × 4325 × 1,14 = 591,66.
  await page.getByLabel('Материал упаковки').selectOption('glass')
  await expect.poll(async () => compact(await total(page).textContent())).toBe('4916,66KZT')
})

test('смена МРП пересчитывает сумму', async ({ page }) => {
  await page.getByLabel('Код ТН ВЭД').fill('8471300000')
  await page.getByLabel('Масса нетто, кг').fill('1000')
  await expect.poll(async () => compact(await total(page).textContent())).toBe('8650,00KZT') // 1 т × 4325 × 2
  await page.getByLabel('МРП').fill('4000')
  await expect.poll(async () => compact(await total(page).textContent())).toBe('8000,00KZT')
})

test('новый код сбрасывает введённые массы', async ({ page }) => {
  await page.getByLabel('Код ТН ВЭД').fill('8471300000')
  await page.getByLabel('Масса нетто, кг').fill('1000')
  await page.getByLabel('Код ТН ВЭД').fill('8516500000')
  await expect(page.getByLabel('Масса нетто, кг')).toHaveValue('')
})

test('транспорт: категория и ввоз из РБ/РФ', async ({ page }) => {
  await page.getByLabel('Код ТН ВЭД').fill('8703231940')
  await expect(page.getByRole('heading', { name: /как за транспорт или технику/ })).toBeVisible()
  const category = page.getByRole('radio', { name: /Двигатель 1 001–2 000 см³/ })
  await category.click()
  await expect(category).toHaveAttribute('aria-checked', 'true')
  await expect.poll(async () => compact(await total(page).textContent())).toBe('756875,00KZT')
  await page.getByLabel('Ввоз из Беларуси или России').check()
  await expect.poll(async () => compact(await total(page).textContent())).toBe('29410000,00KZT')
  // Повторный щелчок снимает выбор.
  await category.click()
  await expect(category).toHaveAttribute('aria-checked', 'false')
  await expect(total(page)).toHaveText('—')
})

test('код не из перечня и неполный код', async ({ page }) => {
  await page.getByLabel('Код ТН ВЭД').fill('0101210000')
  await expect(page.getByRole('heading', { name: 'Код 0101 21 000 0 не входит в перечень' })).toBeVisible()
  await expect(page.getByText('Утильсбор за этот товар и его упаковку не платится.')).toBeVisible()

  await page.getByLabel('Код ТН ВЭД').fill('4011')
  await expect(page.getByText(/зависит от полного кода/).first()).toBeVisible()
  await page.locator('.uf-picks button').first().click()
  await expect(page.getByLabel('Код ТН ВЭД')).not.toHaveValue('4011')
  await expect(page.getByRole('heading', { name: /утильсбор платится/ })).toBeVisible()

  await page.getByLabel('Код ТН ВЭД').fill('84713000001')
  await expect(page.getByText('В коде ТН ВЭД не больше 10 цифр.')).toBeVisible()
})

test('ошибка сервера показывается баннером', async ({ page }) => {
  await page.getByLabel('Код ТН ВЭД').fill('8471300000')
  await page.getByLabel('Масса нетто, кг').fill('-5')
  await expect(page.getByRole('alert')).toBeVisible()
})

test.describe('таблица из Excel', () => {
  const cell = (page: Page, row: number, column: number) => page.locator(`[data-cell="${row}-${column}"]`)

  test.beforeEach(async ({ page }) => {
    await page.getByRole('tab', { name: 'Таблица из Excel' }).click()
    await expect(page.getByRole('tab', { name: 'Таблица из Excel' })).toHaveAttribute('aria-selected', 'true')
  })

  test('ввод строк вручную: суммы по строкам и итог как у API', async ({ page }) => {
    const rows = [['4202929800', '686', ''], ['6307909800', '552', '450'], ['3924100000', '100', ''], ['0101210000', '1800,5', '']]
    for (const [index, row] of rows.entries()) {
      for (const [column, value] of row.entries()) if (value) await cell(page, index, column).fill(value)
    }
    const foot = page.locator('tfoot')
    await expect(foot).toContainText('Итого утильсбор по 4 строкам')
    await expect.poll(async () => compact(await foot.locator('td').nth(1).textContent())).toBe('2457,43KZT')
    const bodyRows = page.locator('.uf-rows-table tbody tr')
    await expect(bodyRows.nth(0)).toContainText('1 044,37 KZT')
    await expect(bodyRows.nth(2)).toContainText('636,64 KZT')
    await expect(bodyRows.nth(3)).toContainText('не платится — кода нет в перечне')
    // Расчётное нетто — серой подсказкой: 686 × 0,8.
    await expect(cell(page, 0, 2)).toHaveAttribute('placeholder', '548,8')
  })

  test('вставка колонок из Excel (Ctrl+V) заполняет строки вниз и вправо', async ({ page }) => {
    await cell(page, 0, 0).focus()
    await cell(page, 0, 0).evaluate((input) => {
      const data = new DataTransfer()
      data.setData('text/plain', '3924100000\t100\n4202929800\t686\n')
      input.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }))
    })
    await expect(cell(page, 0, 0)).toHaveValue('3924100000')
    await expect(cell(page, 0, 1)).toHaveValue('100')
    await expect(cell(page, 1, 0)).toHaveValue('4202929800')
    await expect(cell(page, 1, 1)).toHaveValue('686')
    await expect(cell(page, 2, 0)).toHaveValue('') // пустая строка в конце
    await expect.poll(async () => compact(await page.locator('tfoot td').nth(1).textContent())).toBe('1681,01KZT') // 636,64 + 1044,37
  })

  test('сбор за товар от брутто, % упаковки и материал меняют суммы', async ({ page }) => {
    await cell(page, 0, 0).fill('3924100000')
    await cell(page, 0, 1).fill('100')
    await expect.poll(async () => compact(await page.locator('tfoot td').nth(1).textContent())).toBe('636,64KZT')
    await page.getByLabel('Сбор за товар от').selectOption('gross')
    await expect.poll(async () => compact(await page.locator('tfoot td').nth(1).textContent())).toBe('757,74KZT')
    await page.getByLabel('Сбор за товар от').selectOption('net')
    await page.getByLabel('Нет нетто — вычесть из брутто').fill('10')
    // нетто 90: 0,09 × 4325 × 1,40 = 544,95; упаковка 10 кг × 1,76 = 76,12
    await expect.poll(async () => compact(await page.locator('tfoot td').nth(1).textContent())).toBe('621,07KZT')
    await page.getByLabel('Упаковка из').selectOption('metal')
    // упаковка 0,01 × 4325 × 0,20 = 8,65
    await expect.poll(async () => compact(await page.locator('tfoot td').nth(1).textContent())).toBe('553,60KZT')
  })

  test('ошибка строки — в примечании; неверный % — баннер', async ({ page }) => {
    await cell(page, 0, 0).fill('3924100000')
    await cell(page, 0, 1).fill('10')
    await cell(page, 0, 2).fill('20')
    await expect(page.locator('.c-note.bad')).toHaveText('Нетто больше брутто')
    await page.getByLabel('Нет нетто — вычесть из брутто').fill('100')
    await expect(page.getByRole('alert')).toContainText('Процент упаковки')
  })

  test('таблица и режим сохраняются после перезагрузки; «Очистить таблицу» спрашивает подтверждение', async ({ page }) => {
    await cell(page, 0, 0).fill('3924100000')
    await cell(page, 0, 1).fill('100')
    await expect(page.locator('tfoot')).toBeVisible()
    await page.waitForTimeout(600) // сохранение в localStorage — с задержкой 400 мс
    await page.reload()
    await expect(page.getByRole('tab', { name: 'Таблица из Excel' })).toHaveAttribute('aria-selected', 'true')
    await expect(cell(page, 0, 0)).toHaveValue('3924100000')

    page.once('dialog', (dialog) => dialog.dismiss())
    await page.getByRole('button', { name: 'Очистить таблицу' }).click()
    await expect(cell(page, 0, 0)).toHaveValue('3924100000')

    page.once('dialog', (dialog) => dialog.accept())
    await page.getByRole('button', { name: 'Очистить таблицу' }).click()
    await expect(cell(page, 0, 0)).toHaveValue('')
    await expect(page.locator('tfoot')).toHaveCount(0)
  })
})

test.describe('переводчику утильсбор доступен', () => {
  test.use(as('translator'))
  test('страница открывается и считает', async ({ page }) => {
    await page.getByLabel('Код ТН ВЭД').fill('8471300000')
    await page.getByLabel('Масса нетто, кг').fill('1000')
    await expect.poll(async () => compact(await total(page).textContent())).toBe('8650,00KZT')
  })
})
