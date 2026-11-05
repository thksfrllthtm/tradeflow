import type { Page } from '@playwright/test'
import { test, expect, as } from '../../support/fixtures'
import { users } from '../../support/users'

// Навигация по ролям: меню шапки, закрытые адреса, история браузера и чистая консоль на
// каждой доступной странице.

type PageInfo = { path: string; heading: string | RegExp; menu?: string }

const pages: Record<string, PageInfo> = {
  home: { path: '/', heading: /^Добр/ },
  goods: { path: '/estimates/goods', heading: /.+/ },
  products: { path: '/directory/products', heading: 'Справочник товаров', menu: 'Справочник товаров' },
  nomenclature: { path: '/directory/nomenclature', heading: 'Номенклатура ТН ВЭД ЕАЭС', menu: 'Номенклатура' },
  trademarks: { path: '/directory/trademarks', heading: /^ТРОИС/, menu: 'ТРОИС' },
  decisions: { path: '/decisions', heading: 'База решений', menu: 'База решений' },
  ceramics: { path: '/ceramics', heading: 'Керамика: наименование и цена', menu: 'Керамика' },
  ceramicsBase: { path: '/ceramics/base', heading: 'Керамика: наименование и цена' },
  utilFee: { path: '/util-fee', heading: 'Утильсбор', menu: 'Утильсбор' },
  translator: { path: '/translator', heading: /^Переводчик/ },
  employees: { path: '/employees', heading: /^Сотрудники/ },
}

// Группы меню шапки: «Коды ТН ВЭД», «Инструменты», «Администрирование» (только администратору).
// «Переводчик» у переводчика — отдельной ссылкой в верхней строке, у администратора — в «Инструментах».
const codes = ['Номенклатура', 'База решений', 'Справочник товаров', 'ТРОИС']
const access: Record<'admin' | 'invoicer' | 'translator', { allowed: string[]; menus: Record<string, string[]>; estimates: boolean; translatorOnTop: boolean }> = {
  admin: {
    allowed: Object.keys(pages),
    menus: { 'Коды ТН ВЭД': codes, 'Инструменты': ['Утильсбор', 'Сверка с экспортной декларацией', 'Керамика', 'Переводчик'], 'Администрирование': ['Сотрудники', 'Журнал агента', 'База документов'] },
    estimates: true, translatorOnTop: false,
  },
  invoicer: {
    allowed: ['home', 'goods', 'products', 'nomenclature', 'trademarks', 'decisions', 'ceramics', 'ceramicsBase', 'utilFee'],
    menus: { 'Коды ТН ВЭД': codes, 'Инструменты': ['Утильсбор', 'Сверка с экспортной декларацией', 'Керамика'] },
    estimates: true, translatorOnTop: false,
  },
  translator: {
    allowed: ['home', 'nomenclature', 'trademarks', 'utilFee', 'translator'],
    menus: { 'Коды ТН ВЭД': ['Номенклатура', 'ТРОИС'], 'Инструменты': ['Утильсбор', 'Сверка с экспортной декларацией'] },
    estimates: false, translatorOnTop: true,
  },
}

const nav = (page: Page) => page.getByRole('navigation', { name: 'Разделы' })
const menus = (page: Page) => nav(page).locator('details.app-menu')
const menuOf = (page: Page, label: string) => menus(page).filter({ has: page.locator('summary', { hasText: label }) })
const groupOf = (rules: { menus: Record<string, string[]> }, item: string) => Object.keys(rules.menus).find((label) => rules.menus[label].includes(item)) ?? ''
// У администратора из bootstrap-admin имени может не быть — тогда в профиле его email.
const profile = (page: Page) => page.locator('summary.app-header-avatar')

// Ошибки JS и консоли; сетевые ответы 4xx/5xx браузер тоже пишет в консоль — их не считаем,
// за ними следят API-тесты, а 503 даёт заблокированный в E2E ИИ.
function watchErrors(page: Page) {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(`pageerror: ${error.message}`))
  page.on('console', (message) => {
    if (message.type() === 'error' && !message.text().startsWith('Failed to load resource')) errors.push(`console: ${message.text()}`)
  })
  return errors
}

for (const who of ['admin', 'invoicer', 'translator'] as const) {
  test.describe(`роль ${who}`, () => {
    test.use(as(who))
    const rules = access[who]

    test('меню шапки показывает только доступные разделы', async ({ page }) => {
      await page.goto('/')
      await expect(nav(page).getByRole('link', { name: 'Главная', exact: true })).toBeVisible()
      await expect(nav(page).getByRole('link', { name: 'Сметы', exact: true })).toHaveCount(rules.estimates ? 1 : 0)
      await expect(nav(page).getByRole('link', { name: 'Переводчик', exact: true })).toHaveCount(rules.translatorOnTop ? 1 : 0)

      await expect(menus(page).locator('summary')).toHaveText(Object.keys(rules.menus))
      for (const [label, items] of Object.entries(rules.menus)) {
        await menuOf(page, label).locator('summary').click()
        await expect(menuOf(page, label)).toHaveAttribute('open', '')
        await expect(menuOf(page, label).getByRole('link')).toHaveText(items)
        await page.keyboard.press('Escape')
      }

      // В профиле — только кто вошёл и «Выйти»; разделы администратора — в меню «Администрирование».
      await profile(page).click()
      await expect(page.locator('details.app-header-user').getByRole('link')).toHaveCount(0)
      await expect(page.getByRole('button', { name: 'Выйти' })).toBeVisible()
    })

    test('профиль: имя, email и роль', async ({ page }) => {
      await page.goto('/')
      await profile(page).click()
      const card = page.locator('.app-header-who')
      // Инвойсер — учётка процесса e2e-invoicer-wN (см. workerInvoicer).
      await expect(card).toContainText(who === 'invoicer' ? /e2e-invoicer(-w\d+)?@e2e\.local/ : users[who].email)
      await expect(card).toContainText({ admin: 'Администратор', invoicer: 'Инвойсер', translator: 'Переводчик' }[who])
    })

    for (const key of Object.keys(pages)) {
      const info = pages[key]
      if (rules.allowed.includes(key)) {
        test(`${info.path} открывается без ошибок`, async ({ page }) => {
          const errors = watchErrors(page)
          await page.goto(info.path)
          await expect(page).toHaveURL(new RegExp(`${info.path.replace(/\//g, '\\/')}$`))
          await expect(page.getByRole('heading', { level: 1 }).first()).toHaveText(info.heading)
          await page.waitForLoadState('networkidle')
          expect(errors).toEqual([])
        })
      } else {
        test(`${info.path} закрыт → главная`, async ({ page }) => {
          await page.goto(info.path)
          await expect(page).toHaveURL(/\/$/)
          await expect(page.getByRole('heading', { level: 1 })).toHaveText(/^Добр/)
        })
      }
    }

    test('пункты меню ведут на свои страницы и подсвечивают раздел', async ({ page }) => {
      await page.goto('/')
      await expect(nav(page).getByRole('link', { name: 'Главная', exact: true })).toHaveAttribute('aria-current', 'page')
      for (const key of rules.allowed) {
        const info = pages[key]
        if (!info.menu) continue
        const menu = menuOf(page, groupOf(rules, info.menu))
        await menu.locator('summary').click()
        await menu.getByRole('link', { name: info.menu, exact: true }).click()
        await expect(page).toHaveURL(new RegExp(`${info.path}$`))
        await expect(menu, 'меню закрывается после выбора').not.toHaveAttribute('open', '')
        await expect(menu, 'группа подсвечена и показывает страницу').toHaveClass(/active/)
        await expect(menu.locator('summary')).toContainText(info.menu)
        await expect(page.getByRole('heading', { level: 1 }).first()).toHaveText(info.heading)
      }
    })
  })
}

test.describe('общее', () => {
  test.use(as('invoicer'))

  test('неизвестный адрес → главная', async ({ page }) => {
    await page.goto('/no/such/page')
    await expect(page).toHaveURL(/\/$/)
    await page.goto('/estimates/goods/abc')
    await expect(page).toHaveURL(/\/$/)
  })

  test('лишний слеш в конце адреса не мешает', async ({ page }) => {
    await page.goto('/util-fee/')
    await expect(page.getByRole('heading', { name: 'Утильсбор' })).toBeVisible()
  })

  test('назад/вперёд браузера переключают страницы', async ({ page }) => {
    await page.goto('/')
    await nav(page).getByRole('link', { name: 'Сметы', exact: true }).click()
    await expect(page).toHaveURL(/\/estimates\/goods$/)
    await menuOf(page, 'Инструменты').locator('summary').click()
    await menuOf(page, 'Инструменты').getByRole('link', { name: 'Утильсбор' }).click()
    await expect(page.getByRole('heading', { name: 'Утильсбор' })).toBeVisible()

    await page.goBack()
    await expect(page).toHaveURL(/\/estimates\/goods$/)
    await page.goBack()
    await expect(page).toHaveURL(/\/$/)
    await page.goForward()
    await expect(page).toHaveURL(/\/estimates\/goods$/)
    await page.goForward()
    await expect(page.getByRole('heading', { name: 'Утильсбор' })).toBeVisible()
  })

  test('логотип ведёт на главную', async ({ page }) => {
    await page.goto('/util-fee')
    await page.getByRole('link', { name: 'TradeFlow — на главную' }).click()
    await expect(page).toHaveURL(/\/$/)
  })

  test('меню закрывается по Esc (фокус на кнопку) и щелчком вне', async ({ page }) => {
    await page.goto('/')
    const menu = menuOf(page, 'Инструменты')
    await menu.locator('summary').click()
    await expect(menu).toHaveAttribute('open', '')
    await page.keyboard.press('Escape')
    await expect(menu).not.toHaveAttribute('open', '')
    await expect(menu.locator('summary')).toBeFocused()

    await menu.locator('summary').click()
    await expect(menu).toHaveAttribute('open', '')
    await page.getByRole('heading', { level: 1 }).click()
    await expect(menu).not.toHaveAttribute('open', '')

    await profile(page).click()
    const user = page.locator('details.app-header-user')
    await expect(user).toHaveAttribute('open', '')
    await page.keyboard.press('Escape')
    await expect(user).not.toHaveAttribute('open', '')
  })

  test('прямая ссылка обновляется без потери страницы', async ({ page }) => {
    await page.goto('/directory/nomenclature')
    await page.reload()
    await expect(page).toHaveURL(/\/directory\/nomenclature$/)
    await expect(page.getByRole('heading', { name: 'Номенклатура ТН ВЭД ЕАЭС' })).toBeVisible()
  })
})
