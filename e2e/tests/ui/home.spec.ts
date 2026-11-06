import type { Page, Route } from '@playwright/test'
import { test, expect, as } from '../../support/fixtures'
import { codePrefix, createDecision, decisionName, DecisionTracker } from '../../support/decisions-helpers'
import { E2E, uid } from '../../support/users'

// Главная: быстрые действия по роли, последние сметы, подсказка администратору о решениях на проверке.
// Чат-справочник — home-reference-chat.spec.ts.

const json = (route: Route, body: unknown, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
const chat = (page: Page) => page.getByRole('region', { name: 'Подбор кода ТН ВЭД' })

test.describe('главная по ролям', () => {
  test.describe('инвойсер', () => {
    test.use(as('invoicer'))

    test('приветствие, быстрые действия и чат', async ({ page }) => {
      await page.goto('/')
      await expect(page.getByRole('heading', { level: 1 })).toContainText(', E2E')
      const actions = page.getByRole('complementary', { name: 'Действия и последние сметы' })
      await expect(actions.getByRole('link', { name: /Новая смета/ })).toBeVisible()
      await expect(actions.getByRole('link', { name: /Номенклатура ТН ВЭД/ })).toBeVisible()
      await expect(actions.getByRole('link', { name: /Утильсбор/ })).toBeVisible()
      await expect(page.getByRole('heading', { name: /Последние сметы/ })).toBeVisible()
      await expect(chat(page).getByRole('heading', { name: 'Подбор кода ТН ВЭД' })).toBeVisible()
      await expect(page.getByRole('link', { name: /решени.* по кодам/ }), 'подсказка о проверке — только администратору').toHaveCount(0)
    })

    test('новая смета появляется в «Последних сметах», ссылка ведёт на неё', async ({ page, apiAs }) => {
      const api = await apiAs('invoicer')
      const name = `${E2E}-home-${uid()}`
      const created = await api.post('/api/goods-estimates', { data: { name, client: 'Клиент E2E', assessmentDate: '' } })
      expect(created.status()).toBe(201)
      const id = (await created.json()).estimate.id as number
      try {
        await page.goto('/')
        const link = page.getByRole('link', { name: new RegExp(name) })
        await expect(link).toBeVisible()
        await expect(link).toContainText('Клиент E2E')
        await expect(link).toContainText('сегодня')
        await link.click()
        await expect(page).toHaveURL(new RegExp(`/estimates/goods/${id}$`))
      } finally {
        await api.delete(`/api/goods-estimates/${id}`)
      }
    })

    test('«Новая смета» и «Все сметы» ведут в сметы, быстрые действия — в разделы', async ({ page }) => {
      await page.goto('/')
      await page.getByRole('link', { name: /Новая смета/ }).click()
      await expect(page).toHaveURL(/\/estimates\/goods$/)
      await page.goto('/')
      await page.getByRole('link', { name: 'Все сметы' }).click()
      await expect(page).toHaveURL(/\/estimates\/goods$/)
      await page.goto('/')
      await page.getByRole('link', { name: /Номенклатура ТН ВЭД/ }).click()
      await expect(page).toHaveURL(/\/directory\/nomenclature$/)
      await page.goto('/')
      await page.getByRole('link', { name: /Утильсбор/ }).click()
      await expect(page).toHaveURL(/\/util-fee$/)
    })

    test('ошибка загрузки смет: сообщение и «Повторить»', async ({ page }) => {
      let fail = true
      await page.route('**/api/goods-estimates', (route) => fail ? json(route, { error: { message: 'База недоступна' } }, 500) : route.fallback())
      await page.goto('/')
      const alert = page.getByRole('alert').filter({ hasText: 'Сметы не загрузились.' })
      await expect(alert).toContainText('База недоступна')
      fail = false
      await alert.getByRole('button', { name: 'Повторить' }).click()
      await expect(alert).toBeHidden()
      await expect(page.getByRole('heading', { name: /Последние сметы/ })).toBeVisible()
    })

    test('пустой список смет', async ({ page }) => {
      await page.route('**/api/goods-estimates', (route) => json(route, { estimates: [] }))
      await page.goto('/')
      await expect(page.getByText('Смет пока нет. Нажмите «Новая смета» — она появится здесь.')).toBeVisible()
    })
  })

  test.describe('переводчик', () => {
    test.use(as('translator'))

    test('без смет: только номенклатура, утильсбор и чат', async ({ page }) => {
      await page.goto('/')
      await expect(page.getByRole('link', { name: /Номенклатура ТН ВЭД/ })).toBeVisible()
      await expect(page.getByRole('link', { name: /Утильсбор/ })).toBeVisible()
      await expect(page.getByRole('link', { name: /Новая смета/ })).toHaveCount(0)
      await expect(page.getByRole('heading', { name: /Последние сметы/ })).toHaveCount(0)
      await expect(chat(page)).toBeVisible()
    })
  })

  test.describe('администратор', () => {
    test.use(as('admin'))

    test('решения «Не проверено» — подсказка со ссылкой на фильтр «Ждут проверки»', async ({ page, apiAs }) => {
      const tracker = new DecisionTracker(() => apiAs('admin'))
      const prefix = codePrefix()
      const { body } = await createDecision(await apiAs('invoicer'), { invoiceName: decisionName('home'), code: `${prefix}1` })
      tracker.track(body.id)
      try {
        await page.goto('/')
        const notice = page.getByRole('link', { name: /по кодам.*вашей проверки/ })
        await expect(notice).toBeVisible()
        await notice.click()
        await expect(page).toHaveURL(/\/decisions\?status=pending$/)
        await expect(page.getByRole('radio', { name: /Ждут проверки/ })).toHaveAttribute('aria-checked', 'true')
      } finally {
        await tracker.cleanup()
      }
    })

    test('без решений на проверке подсказки нет', async ({ page }) => {
      await page.route('**/api/code-decisions/stats', (route) => json(route, { pending: 0, verified: 1, rejected: 0, agentChecked: 0, agentCorrect: 0 }))
      await page.goto('/')
      await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
      await expect(page.getByRole('link', { name: /по кодам.*вашей проверки/ })).toHaveCount(0)
    })
  })
})
