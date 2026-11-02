import { test, expect, as } from '../../support/fixtures'
import type { Page } from '@playwright/test'
import { E2E, uid } from '../../support/users'
import { archiveProducts, CODE_LAPTOP, productColumns, readXlsx, xlsx } from '../../support/directories-helpers'

// Справочники в браузере: номенклатура ТН ВЭД с панелями (проверка кодов, антидемпинг, СУР,
// перечень РК), страница ТРОИС и справочник товаров. Общие справочники настоящими файлами не загружаются.

const search = (page: Page) => page.getByRole('searchbox', { name: 'Поиск по номенклатуре' })

test.describe('номенклатура — инвойсер', () => {
  test.use(as('invoicer'))

  test('поиск по коду: строка со ставками и ссылкой «Подробнее»', async ({ page }) => {
    await page.goto('/directory/nomenclature')
    await expect(page.getByRole('heading', { name: 'Номенклатура ТН ВЭД ЕАЭС' })).toBeVisible()
    await expect(page.getByText(/Всего: [\d\s ]+/)).toBeVisible()
    await search(page).fill('8471 30')
    await expect(page.getByText(/Найдено: \d+/)).toBeVisible()
    const row = page.getByRole('row').filter({ hasText: '8471 30 000 0' })
    await expect(row).toHaveCount(1)
    await expect(row.locator('td.nom-rate')).toHaveText('0%')
    await expect(row.locator('td.nom-vat')).toHaveText('16%')
    await expect(row).toContainText('машины вычислительные портативные')
    const link = row.getByRole('link', { name: `Подробнее о коде ${CODE_LAPTOP}` })
    await expect(link).toHaveAttribute('href', /^https:\/\/www\.tws\.by\//)
    await expect(link).toHaveAttribute('target', '_blank')
    await expect(page.getByRole('row', { name: /Товарная позиция 8471/ })).toBeVisible()
  })

  test('поиск по словам и пустой результат', async ({ page }) => {
    await page.goto('/directory/nomenclature')
    await search(page).fill('перчатки резиновые')
    await expect(page.locator('td.nom-name').first()).toContainText(/перчат/i)
    await search(page).fill('zzzнесуществующеесловоqqq')
    await expect(page.getByText('Ничего не найдено. Проверьте код или попробуйте другие слова.')).toBeVisible()
    await expect(page.getByText('Найдено: 0')).toBeVisible()
  })

  test('«Показать ещё» догружает следующую страницу', async ({ page }) => {
    await page.goto('/directory/nomenclature')
    const codes = page.locator('td.c-code')
    await expect(codes).toHaveCount(100)
    await expect(page.getByText(/Показано 100 из/)).toBeVisible()
    await page.getByRole('button', { name: /^Показать ещё \d+$/ }).click()
    await expect(codes).toHaveCount(200)
    await expect(page.getByText(/Показано 200 из/)).toBeVisible()
  })

  test('проверка вставленной колонки кодов', async ({ page }) => {
    await page.goto('/directory/nomenclature')
    await page.getByRole('button', { name: 'Проверить коды' }).click()
    const input = page.locator('textarea.code-check-input')
    await expect(input).toBeFocused()
    const zeroCode = (await (await page.request.get('/api/tnved-nomenclature?q=0&limit=1')).json()).items[0].code as string
    await input.fill(['Код ТН ВЭД', CODE_LAPTOP, '8471 30 000 0', zeroCode.slice(1), '0000000000', '12345'].join('\n'))
    await input.press('Control+Enter')
    await expect(page.getByText('Строк с кодами: 5, разных кодов: 3')).toBeVisible()
    await expect(page.getByText('Нет в ТН ВЭД ЕАЭС: 1')).toBeVisible()
    await expect(page.getByText('Есть в ТН ВЭД: 2')).toBeVisible()
    await expect(page.getByText('в Excel потерян ведущий ноль')).toBeVisible()
    await expect(page.getByText('строк: 2')).toBeVisible()
    await expect(page.getByText('Не похоже на код из 10 цифр: 12345')).toBeVisible()
    await page.getByRole('button', { name: 'Очистить' }).click()
    await expect(input).toHaveValue('')
    await expect(page.getByRole('button', { name: 'Проверить', exact: true })).toBeDisabled()
  })

  test('проверка кодов открывается с главной по ?check=1', async ({ page }) => {
    await page.goto('/directory/nomenclature?check=1')
    await expect(page.getByText('Проверка кодов по номенклатуре ТН ВЭД ЕАЭС')).toBeVisible()
  })

  test('антидемпинг: меры, раскрытие ставок, переход к коду', async ({ page }) => {
    await page.goto('/directory/nomenclature')
    await page.getByRole('button', { name: /^Антидемпинг ЕАЭС/ }).click()
    const panel = page.getByRole('region', { name: 'Антидемпинговые меры ЕАЭС' })
    await expect(panel.getByRole('heading', { name: /Антидемпинговые меры ЕАЭС/ })).toBeVisible()
    await expect(page.getByRole('button', { name: /^Антидемпинг ЕАЭС · \d+$/ })).toBeVisible()
    await expect(panel.getByRole('button', { name: 'Обновить из реестра ЕЭК' }), 'обновление — только администратору').toHaveCount(0)
    const rows = panel.locator('tbody tr')
    test.skip(await rows.count() === 0, 'антидемпинговых мер нет')
    await rows.first().getByRole('button', { name: 'Ставки по производителям' }).click()
    await expect(rows.first().locator('pre.dump-rate-text')).toBeVisible()
    const codeButton = rows.first().locator('td.dump-codes button').first()
    const code = (await codeButton.textContent())!.trim()
    await codeButton.click()
    await expect(search(page)).toHaveValue(code)
  })

  test('СУР: просмотр без кнопок загрузки', async ({ page }) => {
    await page.goto('/directory/nomenclature')
    await page.getByRole('button', { name: /^СУР/ }).click()
    await expect(page.getByText('СУР — стоимостные индикаторы риска')).toBeVisible()
    await expect(page.getByText('Выбрать .xlsx')).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Удалить' })).toHaveCount(0)
    await page.getByRole('button', { name: /^Антидемпинг ЕАЭС/ }).click()
    await expect(page.getByText('СУР — стоимостные индикаторы риска'), 'панели переключаются').toHaveCount(0)
  })

  test('ТРОИС: отдельная страница, просмотр без кнопки загрузки', async ({ page }) => {
    await page.goto('/directory/trademarks')
    await expect(page.getByRole('heading', { name: /^ТРОИС/ })).toBeVisible()
    await expect(page.getByText('Выбрать файл')).toHaveCount(0)
    const registry = page.getByRole('searchbox', { name: 'Поиск по реестру' })
    test.skip(await registry.count() === 0, 'реестр ТРОИС не загружен')
    const regNo = (await page.locator('.tm-table tbody tr td:nth-child(2)').first().textContent())!.trim()
    await registry.fill(regNo)
    await expect(page.locator('.tm-table tbody tr').first()).toContainText(regNo)
    await expect(page.getByRole('textbox', { name: /^Надпись логотипа/ }), 'подписывает логотипы только администратор').toHaveCount(0)
  })

  test('кнопки загрузки номенклатуры и перечня РК — не для инвойсера', async ({ page }) => {
    await page.goto('/directory/nomenclature')
    await expect(page.getByRole('button', { name: 'Проверить коды' })).toBeVisible()
    await expect(page.getByText(/(Обновить из|Загрузить) файл(а)? TWS/)).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Перечень РК' })).toHaveCount(0)
  })
})

test.describe('номенклатура — администратор', () => {
  test.use(as('admin'))

  test('видит загрузку TWS, перечень РК, обновление антидемпинга, управление СУР и загрузку ТРОИС', async ({ page }) => {
    await page.goto('/directory/nomenclature')
    await expect(page.getByText('Обновить из файла TWS')).toBeVisible()
    await page.getByRole('button', { name: 'Перечень РК' }).click()
    await expect(page.getByText('Перечень РК — ставки ниже ЕТТ')).toBeVisible()
    await expect(page.getByText('Excel из PDF')).toBeVisible()
    await page.getByRole('button', { name: /^Антидемпинг ЕАЭС/ }).click()
    await expect(page.getByRole('button', { name: 'Обновить из реестра ЕЭК' })).toBeVisible()
    await page.getByRole('button', { name: /^СУР/ }).click()
    await expect(page.getByText('Выбрать .xlsx')).toBeVisible()
    await page.goto('/directory/trademarks')
    await expect(page.getByText('Выбрать файл')).toBeVisible()
  })

  test('негодный файл TWS: ошибка на экране, номенклатура прежняя', async ({ page }) => {
    await page.goto('/directory/nomenclature')
    const total = await page.getByText(/Всего: [\d\s ]+/).textContent()
    await page.locator('label.nom-upload input[type=file]').setInputFiles({ name: 'tws.xlsx', mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', buffer: Buffer.from('не excel') })
    await expect(page.getByRole('alert')).toBeVisible()
    await expect(page.getByText(total!)).toBeVisible()
  })

  test('негодный Excel перечня РК: ошибка на экране', async ({ page }) => {
    await page.goto('/directory/nomenclature')
    await page.getByRole('button', { name: 'Перечень РК' }).click()
    await page.locator('.kz-step', { hasText: 'Загрузить Excel' }).locator('input[type=file]').setInputFiles({ name: 'kz.xlsx', mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', buffer: await xlsx([['Нет', 'колонок'], ['1', '2']]) })
    await expect(page.locator('.kz-panel').getByRole('alert')).toBeVisible()
  })
})

test.describe('справочник товаров — администратор', () => {
  test.use(as('admin'))
  const prefix = `${E2E} UI товар ${uid()}`
  test.afterAll(async ({ apiAs }) => { await archiveProducts(await apiAs('admin'), prefix) })

  const row = (page: Page, name: string) => page.getByRole('row').filter({ has: page.locator('.pd-name', { hasText: new RegExp(`^${name.replace(/[()]/g, '\\$&')}$`) }) })

  async function create(page: Page, name: string) {
    await page.getByRole('button', { name: '+ Добавить товар' }).click()
    const drawer = page.getByRole('dialog')
    await expect(drawer.getByRole('heading', { name: 'Новый товар' })).toBeVisible()
    await drawer.getByRole('textbox', { name: /^Наименование/ }).fill(name)
    await drawer.getByRole('combobox', { name: 'Код ТН ВЭД' }).fill('8471 30')
    await drawer.getByRole('option', { name: /8471 30 000 0/ }).click()
    await expect(drawer.locator('.tnved-picker-card')).toContainText('ЕТТ 0%')
    await drawer.getByRole('textbox', { name: 'ИТС' }).fill('1,25')
    await drawer.getByRole('textbox', { name: 'Русский' }).fill('ноутбук, лэптоп')
    await drawer.getByRole('radio', { name: 'Проверено' }).click()
    await expect(drawer.getByText('Есть несохранённые изменения')).toBeVisible()
    await drawer.getByRole('button', { name: 'Сохранить' }).click()
    await expect(drawer).toBeHidden()
  }

  test('создание карточки через панель', async ({ page }) => {
    const name = `${prefix} создание`
    await page.goto('/directory/products')
    await create(page, name)
    const created = row(page, name)
    await expect(created).toBeVisible()
    await expect(created).toContainText('8471 30 000 0')
    await expect(created).toContainText('ЕТТ 0% · НДС 16%')
    await expect(created.locator('td.c-num')).toHaveText('1.25')
    await expect(created.locator('.directory-status')).toHaveText('Проверено')
    await expect(created).toContainText('синоним: ноутбук')
  })

  test('без кода — подсказка; дубль — ошибка сервера', async ({ page }) => {
    const name = `${prefix} дубль`
    await page.goto('/directory/products')
    await create(page, name)
    await page.getByRole('button', { name: '+ Добавить товар' }).click()
    const drawer = page.getByRole('dialog')
    await drawer.getByRole('textbox', { name: /^Наименование/ }).fill(name)
    await drawer.getByRole('button', { name: 'Сохранить' }).click()
    await expect(drawer.getByRole('alert')).toHaveText('Выберите код ТН ВЭД')
    // Клавиатура: дождаться списка, ↓ и Enter выбирают код.
    const picker = drawer.getByRole('combobox', { name: 'Код ТН ВЭД' })
    await picker.fill(CODE_LAPTOP)
    await expect(drawer.getByRole('option', { name: /8471 30 000 0/ })).toBeVisible()
    await picker.press('ArrowDown')
    await picker.press('Enter')
    await expect(drawer.locator('.tnved-picker-card')).toContainText('8471 30 000 0')
    await drawer.getByRole('button', { name: 'Сохранить' }).click()
    await expect(drawer.getByRole('alert')).toContainText('Такая карточка товара уже существует')
    page.once('dialog', (dialog) => dialog.accept())
    await drawer.getByRole('button', { name: 'Отмена' }).click()
    await expect(drawer).toBeHidden()
  })

  test('редактирование, несохранённые изменения и Esc', async ({ page }) => {
    const name = `${prefix} правка`
    await page.goto('/directory/products')
    await create(page, name)
    await row(page, name).locator('.pd-name').click()
    const drawer = page.getByRole('dialog')
    await expect(drawer.getByRole('heading', { name })).toBeVisible()
    await expect(drawer.getByText('Без изменений')).toBeVisible()
    await expect(drawer.getByRole('button', { name: 'Сохранить' })).toBeDisabled()
    await drawer.getByRole('textbox', { name: 'Единица' }).fill('компл.')
    // Esc с изменениями спрашивает подтверждение; отказ оставляет панель открытой.
    page.once('dialog', (dialog) => dialog.dismiss())
    await page.keyboard.press('Escape')
    await expect(drawer).toBeVisible()
    await drawer.getByRole('radio', { name: 'Требует проверки' }).click()
    await drawer.getByRole('button', { name: 'Сохранить' }).click()
    await expect(drawer).toBeHidden()
    await expect(row(page, name).locator('td.c-unit')).toHaveText('компл.')
    await expect(row(page, name).locator('.directory-status')).toHaveText('Проверить')
  })

  test('копия, архив и восстановление', async ({ page }) => {
    const name = `${prefix} архив`
    await page.goto('/directory/products')
    await create(page, name)
    await page.getByRole('button', { name: `Копировать «${name}»` }).click()
    await expect(row(page, `${name} (копия)`)).toBeVisible()

    page.once('dialog', (dialog) => { expect(dialog.message()).toContain(name); return dialog.accept() })
    await page.getByRole('button', { name: `В архив «${name}»` }).click()
    await expect(row(page, name)).toHaveCount(0)
    await page.getByLabel('Показать архив').check()
    await expect(row(page, name).locator('.directory-status')).toHaveText('В архиве')
    await page.getByRole('button', { name: `Вернуть из архива «${name}»` }).click()
    await expect(row(page, name).locator('.directory-status')).toHaveText('Проверено')
  })

  test('поиск и фильтр по статусу', async ({ page }) => {
    const name = `${prefix} фильтр`
    await page.goto('/directory/products')
    await create(page, name)
    await page.getByRole('searchbox', { name: 'Поиск' }).fill('лэптоп')
    await expect(row(page, name)).toBeVisible()
    await expect(page.getByText(/^Найдено: \d+$/)).toBeVisible()
    await page.getByRole('radiogroup', { name: 'Статус проверки' }).getByRole('radio', { name: 'Требуют проверки' }).click()
    await expect(row(page, name)).toHaveCount(0)
    await page.getByRole('radiogroup', { name: 'Статус проверки' }).getByRole('radio', { name: 'Проверены' }).click()
    await expect(row(page, name)).toBeVisible()
    await page.getByRole('searchbox', { name: 'Поиск' }).fill('zzzнет-такого')
    await expect(page.getByText('Ничего не найдено. Измените поиск или фильтр.')).toBeVisible()
  })

  test('выгрузка в Excel скачивает файл справочника', async ({ page }) => {
    const name = `${prefix} выгрузка`
    await page.goto('/directory/products')
    await create(page, name)
    const download = page.waitForEvent('download')
    await page.getByRole('button', { name: 'Выгрузить' }).click()
    const file = await download
    expect(file.suggestedFilename()).toMatch(/^Справочник товаров \d{4}-\d{2}-\d{2}\.xlsx$/)
    const rows = await readXlsx(Buffer.from(await (await import('node:fs/promises')).readFile(await file.path())))
    expect(rows[0]).toEqual(productColumns)
    expect(rows.some((cells) => cells[0] === name)).toBe(true)
  })

  test('загрузка из Excel: успех и ошибки в строках', async ({ page }) => {
    const name = `${prefix} импорт`
    await page.goto('/directory/products')
    await page.getByRole('button', { name: 'Загрузить', exact: true }).click()
    const input = page.locator('.dir-transfer-panel input[type=file]')
    await input.setInputFiles({ name: 'products.xlsx', mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', buffer: await xlsx([['Наименование', 'Код ТН ВЭД'], [name, CODE_LAPTOP]]) })
    await expect(page.getByRole('status')).toHaveText('Загружено: новых 1, обновлено 0.')
    await expect(row(page, name)).toBeVisible()

    await input.setInputFiles({ name: 'bad.xlsx', mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', buffer: await xlsx([['Наименование', 'Код ТН ВЭД'], [`${prefix} плохой`, '0000000000']]) })
    await expect(page.locator('.dir-transfer-panel').getByRole('alert')).toHaveText('Файл не загружен: исправьте строки и загрузите снова')
    await expect(page.locator('.dir-transfer-errors')).toContainText('Строка 2')
  })
})

test.describe('справочник товаров — инвойсер', () => {
  test.use(as('invoicer'))

  test('только просмотр: без кнопок изменения, карточка открывается для чтения', async ({ page, apiAs }) => {
    const admin = await apiAs('admin')
    const name = `${E2E} UI чтение ${uid()}`
    await admin.post('/api/product-directory', { data: { name, code: CODE_LAPTOP } })
    try {
      await page.goto('/directory/products')
      await expect(page.getByRole('heading', { name: 'Справочник товаров' })).toBeVisible()
      await expect(page.getByRole('button', { name: '+ Добавить товар' })).toHaveCount(0)
      await expect(page.getByRole('button', { name: 'Выгрузить' })).toHaveCount(0)
      await page.getByRole('searchbox', { name: 'Поиск' }).fill(name)
      const item = page.getByRole('row').filter({ hasText: name })
      await expect(item.getByRole('button', { name: /^В архив/ })).toHaveCount(0)
      await item.locator('.pd-name').click()
      const drawer = page.getByRole('dialog')
      await expect(drawer.getByText('Только просмотр: менять справочник может администратор')).toBeVisible()
      await expect(drawer.getByRole('textbox', { name: /^Наименование/ })).toBeDisabled()
      await expect(drawer.getByRole('button', { name: 'Сохранить' })).toHaveCount(0)
      await drawer.getByRole('button', { name: 'Закрыть', exact: true }).last().click()
      await expect(drawer).toBeHidden()
    } finally {
      await archiveProducts(admin, name)
    }
  })
})

test.describe('справочники — переводчик', () => {
  test.use(as('translator'))

  test('номенклатура доступна, справочник товаров — нет', async ({ page }) => {
    await page.goto('/directory/nomenclature')
    await expect(page.getByRole('heading', { name: 'Номенклатура ТН ВЭД ЕАЭС' })).toBeVisible()
    await page.getByRole('navigation', { name: 'Разделы' }).getByText('Справочники').click()
    await expect(page.getByRole('link', { name: 'Номенклатура' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Справочник товаров' })).toHaveCount(0)
    await page.goto('/directory/products')
    await expect(page).toHaveURL(/\/$/)
    await expect(page.getByRole('heading', { name: 'Справочник товаров' })).toHaveCount(0)
  })
})
