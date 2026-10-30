import type { Page } from '@playwright/test'
import { test, expect } from '../../support/fixtures'
import { createTempUser, deleteUser, type TempUser } from '../../support/auth-helpers'
import { E2E, uid } from '../../support/users'

// Вход, выход и регистрация в браузере. Без сохранённой сессии: каждый тест входит сам
// временным сотрудником, чтобы выход не ломал общие учётки.

const emailInput = (page: Page) => page.getByLabel('Рабочий email')
const passwordInput = (page: Page) => page.locator('input[autocomplete="current-password"]')

async function loginUI(page: Page, email: string, password: string) {
  await emailInput(page).fill(email)
  await passwordInput(page).fill(password)
  await page.getByRole('button', { name: 'Войти' }).click()
}

let user: TempUser
test.beforeEach(async ({ apiAs }) => { user = await createTempUser(await apiAs('admin')) })
test.afterEach(async ({ apiAs }) => { await deleteUser(await apiAs('admin'), user.id) })

test('без сессии любой адрес ведёт на форму входа', async ({ page }) => {
  for (const path of ['/', '/estimates/goods', '/employees', '/no-such-page']) {
    await page.goto(path)
    await expect(page).toHaveURL(/\/login$/)
    await expect(page.getByRole('heading', { name: 'Вход' })).toBeVisible()
  }
})

test('форма входа: поля обязательны, пароль скрыт и показывается кнопкой', async ({ page }) => {
  await page.goto('/login')
  await expect(emailInput(page)).toHaveAttribute('type', 'email')
  await expect(emailInput(page)).toHaveAttribute('required', '')
  await expect(passwordInput(page)).toHaveAttribute('type', 'password')

  await page.getByRole('button', { name: 'Показать пароль' }).click()
  await expect(passwordInput(page)).toHaveAttribute('type', 'text')
  await page.getByRole('button', { name: 'Скрыть пароль' }).click()
  await expect(passwordInput(page)).toHaveAttribute('type', 'password')

  // Пустая форма не уходит на сервер.
  let sent = false
  page.on('request', (request) => { if (request.url().includes('/api/auth/login')) sent = true })
  await page.getByRole('button', { name: 'Войти' }).click()
  await expect(page).toHaveURL(/\/login$/)
  expect(sent).toBe(false)
})

test('неверный пароль — понятная ошибка, остаёмся на входе', async ({ page }) => {
  await page.goto('/login')
  await loginUI(page, user.email, 'wrong-password-123')
  await expect(page.getByRole('alert')).toHaveText('Неверный email или пароль')
  await expect(page).toHaveURL(/\/login$/)
  await expect(passwordInput(page)).toHaveValue('wrong-password-123')
})

test('сеть недоступна — сообщение о сети', async ({ page }) => {
  await page.route('**/api/auth/login', (route) => route.abort('internetdisconnected'))
  await page.goto('/login')
  await loginUI(page, user.email, user.password)
  await expect(page.getByRole('alert')).toHaveText('Сетевая ошибка. Проверьте подключение.')
})

test('пока идёт вход, кнопка заблокирована', async ({ page }) => {
  let release!: () => void
  const gate = new Promise<void>((resolve) => { release = resolve })
  await page.route('**/api/auth/login', async (route) => { await gate; await route.fallback() })
  await page.goto('/login')
  await loginUI(page, user.email, user.password)
  await expect(page.getByRole('button', { name: 'Входим…' })).toBeDisabled()
  release()
  await expect(page).toHaveURL(/\/$/)
})

test('вход → главная с именем, выход → форма входа, сессия на сервере закрыта', async ({ page }) => {
  await page.goto('/login')
  await loginUI(page, user.email, user.password)
  await expect(page).toHaveURL(/\/$/)
  await expect(page.getByRole('heading', { level: 1 })).toContainText(E2E)

  await page.getByLabel(`Профиль: ${user.name}`).click()
  await page.getByRole('button', { name: 'Выйти' }).click()
  await expect(page.getByRole('heading', { name: 'Вход' })).toBeVisible()

  const me = await page.request.get('/api/auth/me')
  expect(me.status()).toBe(401)
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Вход' })).toBeVisible()
  await expect(page.getByRole('status'), 'после своего выхода нет «Сессия завершена»').toHaveCount(0)
})

test('после входа возвращает на страницу, которую открывали', async ({ page }) => {
  await page.goto('/directory/nomenclature')
  await expect(page).toHaveURL(/\/login$/)
  await loginUI(page, user.email, user.password)
  await expect(page).toHaveURL(/\/directory\/nomenclature$/)
  await expect(page.getByRole('heading', { name: 'Номенклатура ТН ВЭД ЕАЭС' })).toBeVisible()
})

test('открытая страница после входа на /login уводит на главную', async ({ page }) => {
  await page.goto('/login')
  await loginUI(page, user.email, user.password)
  await expect(page).toHaveURL(/\/$/)
  await page.goto('/login')
  await expect(page).toHaveURL(/\/$/)
  await page.goto('/register')
  await expect(page).toHaveURL(/\/$/)
})

test('сессия пропала → при перезагрузке «Сессия завершена. Войдите снова»', async ({ page, context }) => {
  await page.goto('/login')
  await loginUI(page, user.email, user.password)
  await expect(page).toHaveURL(/\/$/)
  await context.clearCookies()
  await page.reload()
  await expect(page.getByRole('status')).toHaveText('Сессия завершена. Войдите снова')
  // Сообщение пропадает после входа и при переходе к регистрации.
  await page.getByRole('button', { name: 'Нет аккаунта? Зарегистрироваться' }).click()
  await page.getByRole('button', { name: 'Уже есть аккаунт? Войти' }).click()
  await expect(page.getByRole('status')).toHaveCount(0)
})

test('сессия пропала посреди работы → первый же запрос возвращает на вход с уведомлением', async ({ page, context }) => {
  await page.goto('/login')
  await loginUI(page, user.email, user.password)
  await expect(page).toHaveURL(/\/$/)
  // Главная догружает данные; если стереть cookie раньше, на вход вернёт её же запрос, а не клик.
  await page.waitForLoadState('networkidle')
  await context.clearCookies()
  await page.getByText('Справочники', { exact: true }).click()
  await page.getByRole('link', { name: 'Номенклатура', exact: true }).click()
  await expect(page.getByRole('status')).toHaveText('Сессия завершена. Войдите снова')
  await expect(page.getByRole('heading', { name: 'Вход' })).toBeVisible()
})

test('курс НБРК на странице входа: USD и EUR, другие валюты скрыты', async ({ page }) => {
  await page.route('**/api/rates/today', (route) => route.fulfill({ json: { rates: [
    { currency: 'USD', rate: '480.5', date: '2026-10-30', source: 'НБРК' },
    { currency: 'EUR', rate: '520.12', date: '2026-10-30', source: 'НБРК' },
    { currency: 'CNY', rate: '67.3', date: '2026-10-30', source: 'НБРК' },
  ] } }))
  await page.goto('/login')
  const rates = page.getByRole('region', { name: 'Курс Национального банка РК' })
  await expect(rates).toContainText('Курс НБРК на 30.10.2026')
  await expect(rates).toContainText('1 USD')
  await expect(rates).toContainText('480,5 KZT')
  await expect(rates).toContainText('520,12 KZT')
  await expect(rates).not.toContainText('CNY')
})

test('курса нет — блока нет, вход работает', async ({ page }) => {
  await page.route('**/api/rates/today', (route) => route.fulfill({ status: 502, json: {} }))
  await page.goto('/login')
  await expect(page.getByRole('heading', { name: 'Вход' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Курс Национального банка РК' })).toHaveCount(0)
})

test.describe('регистрация', () => {
  const created: string[] = []
  test.afterAll(async ({ apiAs }) => {
    const admin = await apiAs('admin')
    const list = (await (await admin.get('/api/employees')).json()).items as Array<{ id: number; email: string }>
    for (const item of list.filter((employee) => created.includes(employee.email))) await deleteUser(admin, item.id)
  })

  test('переход с входа и обратно', async ({ page }) => {
    await page.goto('/login')
    await page.getByRole('button', { name: 'Нет аккаунта? Зарегистрироваться' }).click()
    await expect(page).toHaveURL(/\/register$/)
    await expect(page.getByRole('heading', { name: 'Создать аккаунт' })).toBeVisible()
    await expect(page.getByLabel('Имя')).toHaveAttribute('maxlength', '120')
    await expect(page.locator('input[autocomplete="new-password"]')).toHaveAttribute('minlength', '8')
    await page.getByRole('button', { name: 'Уже есть аккаунт? Войти' }).click()
    await expect(page).toHaveURL(/\/login$/)
  })

  test('новый сотрудник регистрируется и попадает на главную как инвойсер', async ({ page }) => {
    const email = `e2e-reg-${uid()}@e2e.local`
    await page.goto('/register')
    await page.getByLabel('Имя').fill(`${E2E} Регистрант`)
    await page.getByLabel('Email').fill(email)
    await page.locator('input[autocomplete="new-password"]').fill('register-pass-1')
    const response = page.waitForResponse('**/api/auth/register')
    await page.getByRole('button', { name: 'Создать аккаунт' }).click()
    test.skip((await response).status() === 403, 'регистрация закрыта (ALLOW_REGISTRATION=false)')
    created.push(email)
    await expect(page).toHaveURL(/\/$/)
    await page.getByLabel(`Профиль: ${E2E} Регистрант`).click()
    await expect(page.locator('.app-header-who')).toContainText('Инвойсер')
    await expect(page.getByRole('link', { name: 'Сотрудники' })).toHaveCount(0)
  })

  test('занятый email — ошибка у поля', async ({ page }) => {
    await page.goto('/register')
    await page.getByLabel('Имя').fill(`${E2E} Дубль`)
    await page.getByLabel('Email').fill(user.email)
    await page.locator('input[autocomplete="new-password"]').fill('register-pass-1')
    await page.getByRole('button', { name: 'Создать аккаунт' }).click()
    const alert = page.getByRole('alert')
    await expect(alert).toBeVisible()
    const text = await alert.textContent()
    test.skip(text?.includes('Регистрация закрыта') ?? false, 'регистрация закрыта')
    await expect(alert).toHaveText('Аккаунт с таким email уже существует')
    await expect(page).toHaveURL(/\/register$/)
  })
})
