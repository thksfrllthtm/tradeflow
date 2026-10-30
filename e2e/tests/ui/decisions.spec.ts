import type { Page } from '@playwright/test'
import { test, expect, as } from '../../support/fixtures'
import { codePrefix, createDecision, decisionName, DecisionTracker } from '../../support/decisions-helpers'

// Страница «База решений»: поиск (только по коду — текстовый поиск дергает платные эмбеддинги),
// фильтры статусов, карточка записи, ручное добавление и проверка администратором.

const spaced = (code: string) => `${code.slice(0, 4)} ${code.slice(4, 6)} ${code.slice(6, 9)} ${code.slice(9)}`

let tracker: DecisionTracker
let prefix: string
test.beforeEach(({ apiAs }) => { tracker = new DecisionTracker(() => apiAs('admin')); prefix = codePrefix() })
test.afterEach(async () => { await tracker.cleanup(); await tracker.cleanupByCode(prefix) })

async function openDecisions(page: Page, query: string) {
  await page.goto('/decisions')
  await expect(page.getByRole('heading', { name: 'База решений' })).toBeVisible()
  await page.getByLabel('Поиск по базе решений').fill(query)
}

const row = (page: Page, name: string) => page.getByRole('row').filter({ hasText: name })
// Щелчок по ячейке даты: в середине строки кнопки «копировать», они строку не открывают.
const openRow = (page: Page, name: string) => row(page, name).locator('.dc-date-col').click()

test.describe('инвойсер', () => {
  test.use(as('invoicer'))

  test('видит запись, ищет по коду, фильтрует по статусу; кнопок администратора нет', async ({ page, apiAs }) => {
    const name = decisionName('ui-inv')
    const { body } = await createDecision(await apiAs('invoicer'), { invoiceName: name, code: `${prefix}1`, unitPriceUsd: '4.5' })
    tracker.track(body.id)

    await openDecisions(page, prefix)
    await expect(page.getByRole('button', { name: 'Импорт базы из Excel' })).toHaveCount(0)
    await expect(page.getByRole('button', { name: '+ Добавить вручную' })).toHaveCount(0)
    const line = row(page, name)
    await expect(line).toHaveCount(1)
    await expect(line).toContainText(spaced(`${prefix}1`))
    await expect(line).toContainText('Не проверено')
    await expect(line).toContainText('вручную')
    await expect(line).toContainText('4,5')
    await expect(line.locator('.dc-warn'), 'кода нет в номенклатуре — значок ⚠').toBeVisible()

    await page.getByRole('radio', { name: /Ждут проверки/ }).click()
    await expect(page.getByRole('radio', { name: /Ждут проверки/ })).toHaveAttribute('aria-checked', 'true')
    await expect(row(page, name)).toHaveCount(1)
    await page.getByRole('radio', { name: /^Проверено/ }).click()
    await expect(page.getByText('Ничего не найдено')).toBeVisible()
    await page.getByRole('radio', { name: /Отклонено/ }).click()
    await expect(page.getByText('Ничего не найдено')).toBeVisible()
    await page.getByRole('radio', { name: /^Все/ }).click()
    await expect(row(page, name)).toHaveCount(1)
  })

  test('карточка записи только для чтения, закрывается по Esc и крестиком', async ({ page, apiAs }) => {
    const name = decisionName('ui-card')
    const { body } = await createDecision(await apiAs('invoicer'), { invoiceName: name, code: `${prefix}1`, reason: 'Причина выбора E2E', quote: 'Цитата E2E', note: 'Заметка E2E' })
    tracker.track(body.id)

    await openDecisions(page, prefix)
    await openRow(page, name)
    const card = page.getByRole('dialog', { name })
    await expect(card).toBeVisible()
    await expect(card).toContainText('Не проверено')
    await expect(card).toContainText('Причина выбора E2E')
    await expect(card).toContainText('Цитата E2E')
    await expect(card).toContainText('Заметка E2E')
    await expect(card).toContainText('Кода нет в текущей номенклатуре')
    await expect(card.getByRole('button', { name: `Код ${spaced(`${prefix}1`)}, скопировать` })).toBeVisible()
    await expect(card.getByLabel('Код ТН ВЭД'), 'поле правки кода — только у администратора').toHaveCount(0)
    for (const label of ['Отметить «Проверено»', 'Отклонить', 'Удалить', 'Сохранить без проверки']) {
      await expect(card.getByRole('button', { name: label })).toHaveCount(0)
    }
    await page.keyboard.press('Escape')
    await expect(card).toBeHidden()

    await openRow(page, name)
    await card.getByRole('button', { name: 'Закрыть' }).click()
    await expect(card).toBeHidden()
  })

  test('карточка открывается по ссылке ?id=, фильтр — по ?status=', async ({ page, apiAs }) => {
    const name = decisionName('ui-link')
    const { body } = await createDecision(await apiAs('invoicer'), { invoiceName: name, code: `${prefix}1` })
    tracker.track(body.id)
    await page.goto(`/decisions?id=${body.id}`)
    await expect(page.getByRole('dialog', { name })).toBeVisible()
    await page.goto('/decisions?status=rejected')
    await expect(page.getByRole('radio', { name: /Отклонено/ })).toHaveAttribute('aria-checked', 'true')
    await page.goto('/decisions?status=bogus')
    await expect(page.getByRole('radio', { name: /^Все/ })).toHaveAttribute('aria-checked', 'true')
  })

  test('код копируется без пробелов', async ({ page, apiAs, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    const name = decisionName('ui-copy')
    const { body } = await createDecision(await apiAs('invoicer'), { invoiceName: name, code: `${prefix}1` })
    tracker.track(body.id)
    await openDecisions(page, prefix)
    await row(page, name).getByRole('button', { name: /^Код .*скопировать$/ }).click()
    await expect(page.getByRole('dialog'), 'щелчок по коду не открывает карточку').toHaveCount(0)
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(`${prefix}1`)
  })

  test('поиск без результатов', async ({ page }) => {
    await openDecisions(page, `${prefix}9`)
    await expect(page.getByText('Ничего не найдено')).toBeVisible()
  })
})

test.describe('администратор', () => {
  test.use(as('admin'))

  test('проверка с исправлением кода: статус «Проверено», отметка «ИИ ошибся»', async ({ page, apiAs }) => {
    const name = decisionName('ui-verify')
    const { body } = await createDecision(await apiAs('invoicer'), { invoiceName: name, code: `${prefix}1`, agentCode: `${prefix}1` })
    tracker.track(body.id)

    await openDecisions(page, prefix)
    await openRow(page, name)
    const card = page.getByRole('dialog', { name })
    await expect(card).toContainText('ИИ предлагал')
    const code = card.getByLabel('Код ТН ВЭД')
    await code.fill('123')
    await expect(card.getByText('Код — 10 цифр')).toBeVisible()
    await expect(card.getByRole('button', { name: 'Отметить «Проверено»' })).toBeDisabled()
    await card.getByLabel('Цена за 1 шт, USD').fill('abc')
    await expect(card.getByText('Число, например 0.5')).toBeVisible()
    await card.getByLabel('Цена за 1 шт, USD').fill('1,5')
    await code.fill(`${prefix}2`)
    await expect(card).toContainText('при проверке будет отмечено, что ИИ ошибся')
    await card.getByLabel('Наименование для декларации').fill('Итог E2E / 中文')
    await card.getByRole('button', { name: 'Отметить «Проверено»' }).click()

    await expect(card.locator('.dc-head-meta .dc-status')).toHaveText('Проверено')
    await expect(card).toContainText('администратор исправил')
    await expect(card.getByRole('button', { name: 'Сохранить', exact: true })).toBeVisible()
    await page.keyboard.press('Escape')
    // В списке показывается итоговое наименование, если оно есть.
    const line = row(page, 'Итог E2E / 中文')
    await expect(line).toContainText('Проверено')
    await expect(line).toContainText(spaced(`${prefix}2`))
    await expect(line.locator('.dc-ai.bad')).toBeVisible()

    const saved = await (await (await apiAs('admin')).get(`/api/code-decisions/${body.id}`)).json()
    expect(saved).toMatchObject({ status: 'verified', code: `${prefix}2`, finalName: 'Итог E2E / 中文', unitPriceUsd: '1.5', agentCorrect: false })
  })

  test('«Сохранить без проверки» и «Отклонить»', async ({ page, apiAs }) => {
    const name = decisionName('ui-reject')
    const { body } = await createDecision(await apiAs('invoicer'), { invoiceName: name, code: `${prefix}1` })
    tracker.track(body.id)

    await openDecisions(page, prefix)
    await openRow(page, name)
    const card = page.getByRole('dialog', { name })
    await card.getByLabel('Заметка').fill('Проверить позже E2E')
    await card.getByRole('button', { name: 'Сохранить без проверки' }).click()
    await expect.poll(async () => (await (await (await apiAs('admin')).get(`/api/code-decisions/${body.id}`)).json()).note).toBe('Проверить позже E2E')
    await expect(card.locator('.dc-head-meta .dc-status')).toHaveText('Не проверено')

    await card.getByRole('button', { name: 'Отклонить' }).click()
    await expect(card.locator('.dc-head-meta .dc-status')).toHaveText('Отклонено')
    await expect(card.getByRole('button', { name: 'Отклонить' })).toHaveCount(0)
    await page.keyboard.press('Escape')
    await page.getByRole('radio', { name: /Отклонено/ }).click()
    await expect(row(page, name)).toContainText('Отклонено')
  })

  test('удаление: отмена в confirm ничего не удаляет, подтверждение — удаляет', async ({ page, apiAs }) => {
    const name = decisionName('ui-delete')
    const { body } = await createDecision(await apiAs('invoicer'), { invoiceName: name, code: `${prefix}1` })
    tracker.track(body.id)

    await openDecisions(page, prefix)
    await openRow(page, name)
    const card = page.getByRole('dialog', { name })
    page.once('dialog', (dialog) => dialog.dismiss())
    await card.getByRole('button', { name: 'Удалить' }).click()
    await expect(card).toBeVisible()
    expect((await (await apiAs('admin')).get(`/api/code-decisions/${body.id}`)).status()).toBe(200)

    page.once('dialog', (dialog) => { expect(dialog.message()).toContain('Удалить запись'); void dialog.accept() })
    await card.getByRole('button', { name: 'Удалить' }).click()
    await expect(card).toBeHidden()
    await expect(page.getByText('Ничего не найдено')).toBeVisible()
    expect((await (await apiAs('admin')).get(`/api/code-decisions/${body.id}`)).status()).toBe(404)
  })

  test('ручное добавление: проверка кода, запись сразу «Проверено», повтор не добавляется', async ({ page }) => {
    const name = decisionName('ui-manual')
    await page.goto('/decisions')
    await page.getByRole('button', { name: '+ Добавить вручную' }).click()
    const form = page.getByRole('dialog', { name: 'Добавить вручную' })
    const submit = form.getByRole('button', { name: 'Добавить' })
    await expect(submit).toBeDisabled()
    await form.getByLabel(/Наименование \(как в инвойсе\)/).fill(name)
    await form.getByLabel(/Код ТН ВЭД/).fill('1234 56')
    await expect(form.getByText('Код — 10 цифр, сейчас 6.')).toBeVisible()
    await expect(submit).toBeDisabled()
    await form.getByLabel(/Код ТН ВЭД/).fill(spaced(`${prefix}1`))
    await form.getByLabel(/Итоговое наименование/).fill('Итог / 中文')
    await expect(submit).toBeEnabled()
    await submit.click()
    await expect(form).toBeHidden()
    await expect(page.getByRole('status')).toHaveText('Решение добавлено в базу')

    await page.getByLabel('Поиск по базе решений').fill(prefix)
    const line = row(page, 'Итог / 中文')
    await expect(line).toContainText('Проверено')
    await expect(line).toContainText('импорт')

    await page.getByRole('button', { name: '+ Добавить вручную' }).click()
    await form.getByLabel(/Наименование \(как в инвойсе\)/).fill(name)
    await form.getByLabel(/Итоговое наименование/).fill('Итог / 中文')
    await form.getByLabel(/Код ТН ВЭД/).fill(`${prefix}1`)
    await form.getByRole('button', { name: 'Добавить' }).click()
    await expect(form.getByRole('alert')).toHaveText('Такой товар с этим кодом уже есть в базе')
    await form.getByRole('button', { name: 'Отмена' }).click()
    await expect(form).toBeHidden()
  })

  test('ручное добавление закрывается по Esc без записи', async ({ page }) => {
    await page.goto('/decisions')
    await page.getByRole('button', { name: '+ Добавить вручную' }).click()
    const form = page.getByRole('dialog', { name: 'Добавить вручную' })
    await form.getByLabel(/Код ТН ВЭД/).fill(`${prefix}1`)
    await page.keyboard.press('Escape')
    await expect(form).toBeHidden()
    await page.getByLabel('Поиск по базе решений').fill(prefix)
    await expect(page.getByText('Ничего не найдено')).toBeVisible()
  })

  test('окно импорта открывается и закрывается', async ({ page }) => {
    await page.goto('/decisions')
    await page.getByRole('button', { name: 'Импорт базы из Excel' }).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()
  })
})

test.describe('переводчик', () => {
  test.use(as('translator'))

  test('страница базы решений закрыта: переход на главную, пункта меню нет', async ({ page }) => {
    await page.goto('/decisions')
    await expect(page).toHaveURL(/\/$/)
    await page.getByRole('navigation', { name: 'Разделы' }).getByText('Справочники').click()
    await expect(page.getByRole('link', { name: 'База решений' })).toHaveCount(0)
  })
})
