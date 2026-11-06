import type { Page, Route } from '@playwright/test'
import { test, expect, as } from '../../support/fixtures'
import { realCode } from '../../support/decisions-helpers'

// Чат-справочник ТН ВЭД на главной. Все ответы ИИ — моки: настоящие запросы к агенту платные.

const now = new Date().toISOString()

const nomenclature = {
  code: realCode, name: 'машины вычислительные 🠺 портативные массой не более 10 кг', tariffText: '0%',
  duty: { type: 'percentage', percent: '0', amount: '', currency: '', unit: '' }, vatPercent: '16',
  detailsUrl: 'https://www.tws.by/tws/tnved/view/8471300000', kz: null, antiDumping: [],
}

const answer = {
  description: 'Ноутбук — портативная вычислительная машина массой до 10 кг.',
  candidates: [
    { code: realCode, verdict: 'fits', quote: 'Портативные вычислительные машины массой не более 10 кг', quoteSource: 'page', reason: 'Масса 1,5 кг, есть процессор, клавиатура и дисплей.', check: 'Масса без упаковки', pageOpened: true, nomenclature, declarations: { url: 'https://www.ifcg.ru/kb/tnved/8471300000/', query: 'ноутбук', count: 42 } },
    { code: '8471410000', verdict: 'no', quote: '', quoteSource: '', reason: 'Был бы, если бы машина не была портативной.', check: '', pageOpened: false, nomenclature: null },
  ],
  name: 'Ноутбук', nameFull: 'Ноутбук (портативная вычислительная машина), масса 1,5 кг', notes: 'Проверьте массу по упаковочному листу.',
  droppedCodes: [], quotesRemoved: 0, confidence: 'high', confidenceReason: 'Код подтверждён текстом позиции и пояснениями.',
  sources: [{ title: 'Пояснения к ТН ВЭД', url: 'https://www.tws.by/tws/tnved/view/8471300000' }],
}

function session(id: number, title: string, extra: Record<string, unknown> = {}) {
  return {
    id, mode: 'reference', title, estimateId: null, itemId: '', status: 'done', error: '',
    item: { name: title, supplierName: '', code: '', quantity: '', places: '', grossKg: '', netKg: '', pricePerUnit: '', unit: '' },
    photos: [], result: answer, steps: 2, runs: 1, costUsd: '0', tokens: 0, model: 'mock', createdAt: now, updatedAt: now,
    messages: [
      { id: 1, kind: 'input', content: { item: { name: title }, photos: 0, ai: true }, createdAt: now },
      { id: 2, kind: 'tool', content: { tool: 'search_nomenclature', label: 'Поиск в номенклатуре', detail: 'ноутбук' }, createdAt: now },
      { id: 3, kind: 'tool', content: { tool: 'open_details', label: 'Пояснения к коду', detail: '8471 30 000 0' }, createdAt: now },
      { id: 4, kind: 'result', content: answer, createdAt: now },
    ],
    ...extra,
  }
}

const json = (route: Route, body: unknown, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })

async function mockStatus(page: Page, status: Record<string, unknown> = {}) {
  await page.route('**/api/tnved-agent/status', (route) => json(route, { configured: true, model: 'mock', vision: true, dailyLimit: 100, usedToday: 3, ...status }))
}

// История вопросов: список меняется по ходу теста.
async function mockHistory(page: Page, items: unknown[]) {
  const state = { items }
  await page.route('**/api/tnved-agent/reference', (route) => route.request().method() === 'GET' ? json(route, { items: state.items }) : route.fallback())
  return state
}

const chat = (page: Page) => page.getByRole('region', { name: 'Подбор кода ТН ВЭД' })
const input = (page: Page) => page.getByLabel('Вопрос о товаре')

test.describe('чат-справочник ТН ВЭД', () => {
  test.use(as('invoicer'))

  test('лимит на сегодня и подсветка у порога', async ({ page }) => {
    await mockStatus(page, { usedToday: 3 })
    await mockHistory(page, [])
    await page.goto('/')
    await expect(chat(page).locator('.rc-limit')).toHaveText('сегодня 3 из 100')
    await expect(chat(page).locator('.rc-limit.near')).toHaveCount(0)

    await page.unroute('**/api/tnved-agent/status')
    await mockStatus(page, { usedToday: 85 })
    await page.reload()
    await expect(chat(page).locator('.rc-limit.near')).toHaveText('сегодня 85 из 100')
  })

  test('ИИ не настроен — поле вопроса закрыто', async ({ page }) => {
    await mockStatus(page, { configured: false })
    await mockHistory(page, [])
    await page.goto('/')
    await expect(chat(page).getByText('ИИ не настроен на сервере.')).toBeVisible()
    await expect(input(page)).toBeDisabled()
    await expect(chat(page).getByRole('button', { name: 'Отправить' })).toBeDisabled()
  })

  test('вопрос → ответ: коды, ставки, цитата, отвергнутый код, наименование, уверенность', async ({ page }) => {
    await mockStatus(page)
    const history = await mockHistory(page, [])
    let sent: unknown = null
    await page.route('**/api/tnved-agent/reference', (route) => {
      if (route.request().method() !== 'POST') return route.fallback()
      sent = route.request().postDataJSON()
      history.items = [{ id: 501, title: 'ноутбук 15 дюймов', status: 'done', bestCode: realCode, hasPhoto: false, createdAt: now, updatedAt: now, kind: 'chat' }]
      return json(route, session(501, 'ноутбук 15 дюймов'), 201)
    })
    await page.goto('/')
    await expect(chat(page).getByRole('heading', { name: 'Какой код у товара?' })).toBeVisible()
    await expect(chat(page).getByRole('button', { name: 'Отправить' })).toBeDisabled()
    await input(page).fill('ноутбук 15 дюймов')
    await input(page).press('Enter')

    const thread = chat(page).locator('.rc-thread')
    await expect(thread).toContainText('Ноутбук — портативная вычислительная машина')
    expect(sent).toEqual({ text: 'ноутбук 15 дюймов', photos: [] })
    await expect(input(page)).toHaveValue('')
    const card = thread.locator('article.rc-cand')
    await expect(card).toHaveCount(1)
    await expect(card.getByRole('button', { name: 'Код 8471 30 000 0, скопировать' })).toBeVisible()
    await expect(card).toContainText('подходит')
    await expect(card).toContainText('Пошлина 0%')
    await expect(card).toContainText('НДС 16%')
    await expect(card.locator('blockquote')).toContainText('Портативные вычислительные машины')
    await expect(card.locator('cite')).toHaveText('пояснения к коду (TWS.BY)')
    await expect(card).toContainText('Уточнить: Масса без упаковки')
    await expect(card.getByRole('link', { name: 'Страница кода ↗' })).toHaveAttribute('href', nomenclature.detailsUrl)
    await expect(card.getByRole('link', { name: 'Декларации · 42 ↗' })).toBeVisible()
    await expect(card.getByRole('button', { name: 'Документы для ввоза' })).toBeEnabled()
    await expect(thread.locator('.rc-rejected')).toContainText('Когда был бы другой код')
    await expect(thread.locator('.rc-rejected')).toContainText('8471 41 000 0')
    await expect(thread).toContainText('Уверенность: высокая')
    await expect(thread).toContainText('Код подтверждён текстом позиции')
    await expect(thread.locator('.rc-names')).toContainText('Краткое')
    await expect(thread.locator('.rc-names')).toContainText('Ноутбук (портативная вычислительная машина), масса 1,5 кг')
    await expect(thread).toContainText('Проверьте массу по упаковочному листу.')

    const steps = thread.getByRole('button', { name: /Ход поиска · 2 шага/ })
    await expect(steps).toHaveAttribute('aria-expanded', 'false')
    await steps.click()
    await expect(thread.getByText('Поиск в номенклатуре')).toBeVisible()

    // На широком экране история видна всегда (кнопка «История» — только на узком).
    await expect(chat(page).locator('.rc-history-title')).toHaveText('История вопросов1')
    await expect(chat(page).getByRole('complementary', { name: 'Прошлые вопросы' })).toContainText('8471 30 000 0')
    await expect(chat(page).getByRole('button', { name: '+ Новый вопрос' })).toBeEnabled()
  })

  test('копирование кода и наименования', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await mockStatus(page)
    await mockHistory(page, [])
    await page.route('**/api/tnved-agent/reference', (route) => route.request().method() === 'POST' ? json(route, session(502, 'ноутбук'), 201) : route.fallback())
    await page.goto('/')
    await input(page).fill('ноутбук')
    await input(page).press('Enter')
    const card = chat(page).locator('article.rc-cand')
    await card.getByRole('button', { name: 'Код 8471 30 000 0, скопировать' }).click()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(realCode)
    const short = chat(page).locator('.rc-name-row').filter({ hasText: 'Краткое' })
    await short.getByRole('button', { name: 'Копировать' }).click()
    await expect(short.getByRole('button', { name: 'Скопировано' })).toBeVisible()
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('Ноутбук')
  })

  test('уточнение с теми же кодами — «Код без изменений» и «Ответ тот же»', async ({ page }) => {
    await mockStatus(page)
    await mockHistory(page, [])
    await page.route('**/api/tnved-agent/reference', (route) => route.request().method() === 'POST' ? json(route, session(503, 'ноутбук'), 201) : route.fallback())
    let clarification: unknown = null
    await page.route('**/api/tnved-agent/sessions/503/messages', (route) => {
      clarification = route.request().postDataJSON()
      return json(route, session(503, 'ноутбук', {
        messages: [
          { id: 5, kind: 'user', content: { text: 'а если вес 3 кг?' }, createdAt: now },
          { id: 6, kind: 'result', content: answer, createdAt: now },
        ],
      }))
    })
    await page.goto('/')
    await input(page).fill('ноутбук')
    await input(page).press('Enter')
    await expect(chat(page).locator('article.rc-cand')).toHaveCount(1)
    await input(page).fill('а если вес 3 кг?')
    await input(page).press('Enter')
    expect.soft(clarification).toEqual({ text: 'а если вес 3 кг?' })
    const thread = chat(page).locator('.rc-thread')
    await expect(thread.locator('.rc-user').last()).toHaveText('а если вес 3 кг?')
    await expect(thread.locator('.rc-same-code')).toContainText('Код без изменений')
    await expect(thread.getByText('Ответ тот же.')).toBeVisible()
    await expect(thread.locator('article.rc-cand'), 'карточки кода не повторяются').toHaveCount(1)
  })

  test('«Перевести на китайский» показывает перевод наименования', async ({ page }) => {
    await mockStatus(page)
    await mockHistory(page, [])
    await page.route('**/api/tnved-agent/reference', (route) => route.request().method() === 'POST' ? json(route, session(504, 'ноутбук'), 201) : route.fallback())
    await page.route('**/api/tnved-agent/sessions/504/name', (route) => json(route, { name: 'Ноутбук / 笔记本电脑' }))
    await page.goto('/')
    await input(page).fill('ноутбук')
    await input(page).press('Enter')
    const short = chat(page).locator('.rc-name-row').filter({ hasText: 'Краткое' })
    await short.getByRole('button', { name: 'Перевести на китайский' }).click()
    await expect(short).toContainText('Ноутбук / 笔记本电脑')
    await expect(short.getByRole('button', { name: 'Перевести на китайский' })).toHaveCount(0)
  })

  test('ИИ недоступен (503) — ошибка в поле вопроса, текст не теряется', async ({ page, aiBlocked }) => {
    await mockStatus(page)
    await mockHistory(page, [])
    await page.goto('/')
    await input(page).fill('кружка керамическая')
    await chat(page).getByRole('button', { name: 'Отправить' }).click()
    await expect(chat(page).getByRole('alert')).toHaveText('ИИ отключён в E2E')
    await expect(input(page)).toHaveValue('кружка керамическая')
    expect(aiBlocked.some((url) => url.endsWith('/api/tnved-agent/reference'))).toBe(true)
  })

  test('команды: меню по «/», неизвестная команда, /троис и /документы без аргументов', async ({ page, aiBlocked }) => {
    await mockStatus(page)
    await mockHistory(page, [])
    await page.goto('/')
    await input(page).fill('/')
    const menu = chat(page).getByRole('listbox', { name: 'Команды' })
    await expect(menu.getByRole('option')).toHaveCount(5)
    await input(page).fill('/тр')
    await expect(menu.getByRole('option')).toHaveCount(1)
    await expect(menu.getByRole('option')).toContainText('Проверка бренда по ТРОИС')
    await input(page).press('Enter')
    await expect(input(page), 'выбор в меню дописывает команду').toHaveValue('/троис ')
    await input(page).press('Enter')
    await expect(chat(page).getByRole('alert')).toHaveText('Напишите бренд или добавьте фото: /троис SONV')

    await input(page).fill('/абв что-то')
    await input(page).press('Enter')
    await expect(chat(page).getByRole('alert')).toHaveText('Нет такой команды — наберите «/», чтобы увидеть список')

    // Пробел в конце: без него Enter выбирает команду в меню, а не отправляет.
    await input(page).fill('/документы ')
    await input(page).press('Enter')
    await expect(chat(page).getByRole('alert')).toContainText('Укажите код')
    await input(page).fill('/документы круг 6804 22')
    await input(page).press('Enter')
    await expect(chat(page).getByRole('alert')).toContainText('Код — 10 цифр')
    await input(page).fill('/документы 6804225000')
    await input(page).press('Enter')
    await expect(chat(page).getByRole('alert')).toContainText('Опишите товар или добавьте фото')

    await input(page).fill('/сравнить ')
    await input(page).press('Enter')
    await expect(chat(page).getByRole('alert')).toContainText('Укажите коды')

    await input(page).fill('/')
    await input(page).press('Escape')
    await expect(input(page)).toHaveValue('')
    expect(aiBlocked, 'ни один запрос к ИИ не ушёл').toEqual([])
  })

  test('«/» вне поля переводит фокус в вопрос и открывает меню', async ({ page }) => {
    await mockStatus(page)
    await mockHistory(page, [])
    await page.goto('/')
    await page.getByRole('heading', { level: 1 }).click()
    await page.keyboard.press('/')
    await expect(input(page)).toBeFocused()
    await expect(chat(page).getByRole('listbox', { name: 'Команды' })).toBeVisible()
  })

  test('/троис — проверка бренда в фоне: ответ «нет в реестре»', async ({ page }) => {
    await mockStatus(page)
    await mockHistory(page, [])
    let sent: unknown = null
    await page.route('**/api/tnved-agent/reference/trademarks', (route) => {
      sent = route.request().postDataJSON()
      return json(route, session(505, 'SONV', {
        trademarkCheck: true, result: null,
        messages: [
          { id: 1, kind: 'input', content: { item: { name: 'SONV' }, photos: 0, ai: false }, createdAt: now },
          { id: 2, kind: 'trademarks', content: { brands: [], pictures: [], hits: [], compared: 0 }, createdAt: now },
        ],
      }), 201)
    })
    await page.goto('/')
    await input(page).fill('/троис SONV')
    await input(page).press('Enter')
    expect(sent).toEqual({ text: 'SONV', photos: [] })
    const thread = chat(page).locator('.rc-thread')
    await expect(thread).toContainText('Проверка по ТРОИС · SONV')
    await expect(thread).toContainText('В реестре ТРОИС по написанию не найдено')
    await expect(input(page)).toHaveAttribute('placeholder', /для другого бренда/)
  })

  test('история: вкладки по виду, открытие вопроса, удаление с подтверждением', async ({ page }) => {
    await mockStatus(page)
    const history = await mockHistory(page, [
      { id: 601, title: 'перчатки нитриловые', status: 'done', bestCode: '4015190000', hasPhoto: true, createdAt: now, updatedAt: now, kind: 'chat' },
      { id: 602, title: 'SONV', status: 'done', bestCode: '', hasPhoto: false, createdAt: now, updatedAt: now, kind: 'trademarks', trademarkHits: 2 },
      { id: 603, title: 'круг наждачный', status: 'done', bestCode: '6804225000', hasPhoto: false, createdAt: now, updatedAt: now, kind: 'documents' },
      { id: 604, title: 'неудачный вопрос', status: 'error', bestCode: '', hasPhoto: false, createdAt: now, updatedAt: now, kind: 'chat' },
    ])
    await page.route('**/api/tnved-agent/sessions/601*', (route) => json(route, session(601, 'перчатки нитриловые')))
    let deleted = 0
    await page.route('**/api/tnved-agent/reference/604', (route) => {
      if (route.request().method() !== 'DELETE') return route.fallback()
      deleted = 604
      history.items = history.items.filter((item) => (item as { id: number }).id !== 604)
      return route.fulfill({ status: 204 })
    })
    await page.goto('/')
    const aside = chat(page).getByRole('complementary', { name: 'Прошлые вопросы' })
    const tabs = aside.getByRole('group', { name: 'Вид вопросов' })
    const item = (title: string) => aside.locator('.rc-history-open').filter({ hasText: title })
    await expect(tabs.getByRole('button', { name: /Общий/ })).toHaveAttribute('aria-pressed', 'true')
    await expect(aside.locator('.rc-history-open')).toHaveCount(4)
    await expect(item('SONV')).toContainText('ТРОИС: найдено 2')
    await expect(item('неудачный вопрос')).toContainText('не завершён')
    await expect(item('перчатки нитриловые')).toContainText('4015 19 000 0')

    await tabs.getByRole('button', { name: /ТРОИС/ }).click()
    await expect(aside.locator('.rc-history-open')).toHaveCount(1)
    await tabs.getByRole('button', { name: /Документы/ }).click()
    await expect(aside.locator('.rc-history-open')).toHaveText(/круг наждачный/)
    await tabs.getByRole('button', { name: /Чат/ }).click()
    await expect(aside.locator('.rc-history-open')).toHaveCount(2)

    page.once('dialog', (dialog) => dialog.dismiss())
    await aside.getByRole('button', { name: 'Удалить вопрос «неудачный вопрос»' }).click({ force: true })
    expect(deleted, 'отмена confirm — без удаления').toBe(0)
    page.once('dialog', (dialog) => { expect(dialog.message()).toBe('Удалить этот вопрос из истории?'); void dialog.accept() })
    await aside.getByRole('button', { name: 'Удалить вопрос «неудачный вопрос»' }).click({ force: true })
    await expect(item('неудачный вопрос')).toHaveCount(0)
    expect(deleted).toBe(604)

    await item('перчатки нитриловые').click()
    await expect(chat(page).locator('article.rc-cand')).toHaveCount(1)
    await expect(item('перчатки нитриловые')).toHaveAttribute('aria-current', 'true')

    await chat(page).getByRole('button', { name: '+ Новый вопрос' }).click()
    await expect(chat(page).getByRole('heading', { name: 'Какой код у товара?' })).toBeVisible()
  })

  test('открытый вопрос восстанавливается после обновления страницы', async ({ page }) => {
    await mockStatus(page)
    await mockHistory(page, [])
    await page.route('**/api/tnved-agent/reference', (route) => route.request().method() === 'POST' ? json(route, session(506, 'ноутбук'), 201) : route.fallback())
    await page.route('**/api/tnved-agent/sessions/506*', (route) => json(route, session(506, 'ноутбук')))
    await page.goto('/')
    await input(page).fill('ноутбук')
    await input(page).press('Enter')
    await expect(chat(page).locator('article.rc-cand')).toHaveCount(1)
    await page.reload()
    await expect(chat(page).locator('article.rc-cand')).toHaveCount(1)
  })

  test('фото не JPG/PNG — ошибка, вложение не добавляется', async ({ page }) => {
    await mockStatus(page)
    await mockHistory(page, [])
    await page.goto('/')
    await chat(page).locator('.rc-attach input[type=file]').setInputFiles({ name: 'doc.txt', mimeType: 'text/plain', buffer: Buffer.from('not an image') })
    await expect(chat(page).getByRole('alert')).toHaveText('Фото — JPG или PNG')
    await expect(chat(page).locator('.rc-attachment')).toHaveCount(0)
  })

  test('фото PNG прикрепляется и убирается (без отправки)', async ({ page }) => {
    await mockStatus(page)
    await mockHistory(page, [])
    await page.goto('/')
    const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64')
    await chat(page).locator('.rc-attach input[type=file]').setInputFiles({ name: 'item.png', mimeType: 'image/png', buffer: png })
    await expect(chat(page).locator('.rc-attachment')).toHaveCount(1)
    await expect(chat(page).getByRole('button', { name: 'Отправить' })).toBeEnabled()
    await chat(page).getByRole('button', { name: 'Убрать фото' }).click()
    await expect(chat(page).locator('.rc-attachment')).toHaveCount(0)
  })
})

test.describe('чат-справочник на узком экране', () => {
  test.use({ ...as('invoicer'), viewport: { width: 800, height: 900 } })

  test('история прячется за кнопкой «История · N»', async ({ page }) => {
    await mockStatus(page)
    await mockHistory(page, [{ id: 701, title: 'кружка', status: 'done', bestCode: '6912002300', hasPhoto: false, createdAt: now, updatedAt: now, kind: 'chat' }])
    await page.goto('/')
    const aside = chat(page).getByRole('complementary', { name: 'Прошлые вопросы' })
    await expect(aside).toBeHidden()
    const toggle = chat(page).getByRole('button', { name: 'История · 1' })
    await expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await toggle.click()
    await expect(toggle).toHaveAttribute('aria-expanded', 'true')
    await expect(aside).toBeVisible()
    await expect(aside).toContainText('кружка')
  })
})
