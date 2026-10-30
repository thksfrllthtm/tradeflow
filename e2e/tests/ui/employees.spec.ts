import type { APIRequestContext, Page } from '@playwright/test'
import { test, expect, as } from '../../support/fixtures'
import { E2E, uid } from '../../support/users'
import { createTemp, listEmployees, loginAs, removeTemp, tmpEmail, tmpPassword } from '../../support/employees-helpers'

// Страница «Сотрудники» под администратором. Работаем только со своими временными
// сотрудниками e2e-tmp-*@e2e.local: в списке есть и настоящие сотрудники.

test.use(as('admin'))

const created: number[] = []
let admin: APIRequestContext

const createdEmails = new Set<string>()

test.beforeEach(async ({ apiAs }) => { admin = await apiAs('admin') })
test.afterEach(async () => {
  // Сотрудники, созданные через форму, ищутся по email.
  for (const item of await listEmployees(admin)) if (createdEmails.has(item.email) && !created.includes(item.id)) created.push(item.id)
  createdEmails.clear()
  await removeTemp(admin, created)
})

const row = (page: Page, email: string) => page.getByRole('row').filter({ hasText: email })
const status = (page: Page) => page.getByRole('status')
const alert = (page: Page) => page.getByRole('alert')

test.describe('администратор', () => {
test.beforeEach(async ({ page }) => {
  await page.goto('/employees')
  await expect(page.getByRole('heading', { name: /Сотрудники/ })).toBeVisible()
})

test('список: свой аккаунт отмечен «(вы)», роль себе менять нельзя, удалить/заблокировать себя — нет кнопок', async ({ page }) => {
  const me = row(page, 'e2e-admin@e2e.local')
  await expect(me).toContainText('(вы)')
  await expect(me.getByRole('combobox')).toBeDisabled()
  await expect(me.getByRole('button', { name: 'Удалить' })).toHaveCount(0)
  await expect(me.getByRole('button', { name: 'Заблокировать' })).toHaveCount(0)
  await expect(row(page, 'e2e-invoicer@e2e.local')).toContainText('Инвойсер')
  await expect(row(page, 'e2e-translator@e2e.local')).toContainText('Переводчик')
  // Временных сотрудников e2e-tmp-* соседние тесты создают и удаляют параллельно — их не сверяем.
  const count = Number(await page.locator('.dir-count').innerText())
  const stable = (await listEmployees(admin)).filter((item) => !item.email.startsWith('e2e-tmp-'))
  expect(count).toBeGreaterThanOrEqual(stable.length)
  for (const item of stable.slice(0, 50)) await expect(row(page, item.email)).toHaveCount(1)
})

test('создание сотрудника через форму, затем он входит', async ({ page, baseURL }) => {
  const email = tmpEmail()
  const name = `${E2E} Форма ${uid()}`
  createdEmails.add(email)
  await page.getByRole('button', { name: '+ Добавить сотрудника' }).click()
  const form = page.locator('form.emp-card')
  await form.getByLabel('Имя').fill(name)
  await form.getByLabel('Email').fill(email)
  await form.getByLabel('Пароль', { exact: true }).fill(tmpPassword)
  await form.getByRole('combobox').selectOption({ label: 'Переводчик' })
  await form.getByRole('button', { name: 'Добавить' }).click()

  await expect(status(page)).toContainText(`Сотрудник ${name} добавлен`)
  await expect(form).toHaveCount(0)
  const added = row(page, email)
  await expect(added).toContainText(name)
  await expect(added.getByRole('combobox')).toHaveValue('translator')
  await expect(added).toContainText('Активен')

  const { context, status: code } = await loginAs(baseURL, email, tmpPassword)
  expect(code).toBe(200)
  await context.dispose()
})

test('форма: браузер не пускает короткий пароль и пустые поля; «Отмена» очищает', async ({ page }) => {
  let posted = false
  page.on('request', (request) => { if (request.method() === 'POST' && request.url().endsWith('/api/employees')) posted = true })
  await page.getByRole('button', { name: '+ Добавить сотрудника' }).click()
  const form = page.locator('form.emp-card')
  await form.getByRole('button', { name: 'Добавить' }).click()
  expect(await form.getByLabel('Имя').evaluate((input: HTMLInputElement) => input.validity.valueMissing)).toBe(true)

  await form.getByLabel('Имя').fill(`${E2E} коротко`)
  await form.getByLabel('Email').fill(tmpEmail())
  await form.getByLabel('Пароль', { exact: true }).fill('1234567')
  await form.getByRole('button', { name: 'Добавить' }).click()
  expect(await form.getByLabel('Пароль', { exact: true }).evaluate((input: HTMLInputElement) => input.validity.tooShort || !input.checkValidity())).toBe(true)
  await expect(form).toBeVisible()
  expect(posted).toBe(false)

  await form.getByRole('button', { name: 'Отмена' }).click()
  await expect(form).toHaveCount(0)
  await page.getByRole('button', { name: '+ Добавить сотрудника' }).click()
  await expect(page.locator('form.emp-card').getByLabel('Имя')).toHaveValue('')
})

test('ошибка сервера на форме: дубль email', async ({ page }) => {
  const existing = await createTemp(admin, created)
  await page.getByRole('button', { name: '+ Добавить сотрудника' }).click()
  const form = page.locator('form.emp-card')
  await form.getByLabel('Имя').fill(`${E2E} дубль`)
  await form.getByLabel('Email').fill(existing.email.toUpperCase())
  await form.getByLabel('Пароль', { exact: true }).fill(tmpPassword)
  await form.getByRole('button', { name: 'Добавить' }).click()
  await expect(alert(page)).toContainText('Сотрудник с таким email уже есть')
  await expect(form, 'форма остаётся с введёнными данными').toBeVisible()
  await expect(form.getByLabel('Имя')).toHaveValue(`${E2E} дубль`)
  await alert(page).getByRole('button', { name: '×' }).click()
  await expect(alert(page)).toHaveCount(0)
})

test('редактирование имени и email; «Отмена» не сохраняет', async ({ page }) => {
  const employee = await createTemp(admin, created)
  await page.reload()
  const target = row(page, employee.email)
  // В режиме правки email стоит в поле ввода, а не текстом — строку ищем по форме правки.
  const editing = page.getByRole('row').filter({ has: page.locator('form.emp-edit') })

  await target.getByRole('button', { name: 'Изменить' }).click()
  await expect(editing.getByLabel('Email')).toHaveValue(employee.email)
  await editing.getByLabel('Имя').fill(`${E2E} не сохранять`)
  await editing.getByRole('button', { name: 'Отмена' }).click()
  await expect(target).toContainText(employee.name)

  const email = tmpEmail()
  const name = `${E2E} Изменён ${uid()}`
  await target.getByRole('button', { name: 'Изменить' }).click()
  await editing.getByLabel('Имя').fill(name)
  await editing.getByLabel('Email').fill(email)
  await editing.getByRole('button', { name: 'Сохранить' }).click()
  await expect(status(page)).toContainText(`Данные сотрудника ${name} сохранены`)
  await expect(row(page, email)).toContainText(name)
  expect((await listEmployees(admin)).find((item) => item.id === employee.id)).toMatchObject({ name, email })
})

test('смена роли из списка', async ({ page }) => {
  const employee = await createTemp(admin, created)
  await page.reload()
  await row(page, employee.email).getByRole('combobox').selectOption('translator')
  await expect(status(page)).toContainText('роль «Переводчик»')
  expect((await listEmployees(admin)).find((item) => item.id === employee.id)?.role).toBe('translator')
})

test('смена пароля из списка: новый пароль входит', async ({ page, baseURL }) => {
  const employee = await createTemp(admin, created)
  await page.reload()
  const target = row(page, employee.email)
  await target.getByRole('button', { name: 'Сменить пароль' }).click()
  await target.getByLabel('Новый пароль').fill('e2e-ui-new-pass-1')
  await target.getByRole('button', { name: 'Сохранить' }).click()
  await expect(status(page)).toContainText('изменён')
  await expect(target.getByLabel('Новый пароль')).toHaveCount(0)
  expect((await loginAs(baseURL, employee.email, tmpPassword)).status).toBe(401)
  const { context, status: code } = await loginAs(baseURL, employee.email, 'e2e-ui-new-pass-1')
  expect(code).toBe(200)
  await context.dispose()
})

test('блокировка с подтверждением и разблокировка', async ({ page, baseURL }) => {
  const employee = await createTemp(admin, created)
  await page.reload()
  const target = row(page, employee.email)

  // Отказ в подтверждении — ничего не меняется.
  page.once('dialog', (dialog) => dialog.dismiss())
  await target.getByRole('button', { name: 'Заблокировать' }).click()
  await expect(target).toContainText('Активен')

  page.once('dialog', (dialog) => { expect(dialog.message()).toContain('Заблокировать'); void dialog.accept() })
  await target.getByRole('button', { name: 'Заблокировать' }).click()
  await expect(target).toContainText('Заблокирован')
  await expect(status(page)).toContainText('заблокирован')
  expect((await loginAs(baseURL, employee.email, tmpPassword)).status).toBe(401)

  await target.getByRole('button', { name: 'Разблокировать' }).click()
  await expect(target).toContainText('Активен')
  const { context, status: code } = await loginAs(baseURL, employee.email, tmpPassword)
  expect(code).toBe(200)
  await context.dispose()
})

test('удаление: отмена оставляет, подтверждение удаляет', async ({ page }) => {
  const employee = await createTemp(admin, created)
  await page.reload()
  const target = row(page, employee.email)

  page.once('dialog', (dialog) => dialog.dismiss())
  await target.getByRole('button', { name: 'Удалить' }).click()
  await expect(target).toBeVisible()

  page.once('dialog', (dialog) => { expect(dialog.message()).toContain('Отменить удаление нельзя'); void dialog.accept() })
  await target.getByRole('button', { name: 'Удалить' }).click()
  await expect(status(page)).toContainText('удалён')
  await expect(target).toHaveCount(0)
  created.splice(created.indexOf(employee.id), 1)
})

test('сметы сотрудника: панель, переход в смету с пометкой владельца, удаление переносит смету', async ({ page, baseURL }) => {
  const employee = await createTemp(admin, created)
  const { context } = await loginAs(baseURL, employee.email, tmpPassword)
  const name = `${E2E} смета сотрудника ${uid()}`
  const response = await context.post('/api/goods-estimates', { data: { name, client: `${E2E} клиент`, assessmentDate: '2026-10-30' } })
  expect(response.status(), await response.text()).toBe(201)
  const estimateId = (await response.json()).estimate.id
  await context.dispose()

  await page.reload()
  const target = row(page, employee.email)
  await expect(target.getByRole('button', { name: '1' })).toBeEnabled()
  await target.getByRole('button', { name: '1' }).click()

  const panel = page.getByRole('dialog', { name: `Сметы: ${employee.name}` })
  await expect(panel).toBeVisible()
  await expect(panel).toContainText(name)
  await page.keyboard.press('Escape')
  await expect(panel).toHaveCount(0)

  await target.getByRole('button', { name: '1' }).click()
  // У пустой сметы первый шаг — только загрузка файла, без шапки; шапка с пометкой — со второго.
  await panel.getByRole('link', { name: new RegExp(name) }).click()
  await expect(page).toHaveURL(new RegExp(`/estimates/goods/${estimateId}$`))
  await page.getByRole('button', { name: 'заполните строки вручную' }).click()
  await expect(page.getByText(`Смета сотрудника: ${employee.name}`)).toBeVisible()
  await expect(page.getByRole('heading', { name })).toBeVisible()
  await page.getByRole('link', { name: 'к сотрудникам' }).click()
  await expect(page).toHaveURL(/\/employees$/)

  // Удаление: подтверждение предупреждает о переносе, после — смета в «Сметах» администратора.
  page.once('dialog', (dialog) => { expect(dialog.message()).toContain('перейдут к вам'); void dialog.accept() })
  await row(page, employee.email).getByRole('button', { name: 'Удалить' }).click()
  await expect(status(page)).toContainText('теперь в ваших «Сметах»')
  created.splice(created.indexOf(employee.id), 1)
  const mine = (await (await admin.get('/api/goods-estimates')).json()).estimates as Array<{ id: number }>
  expect(mine.some((item) => item.id === estimateId)).toBe(true)
  await admin.delete(`/api/goods-estimates/${estimateId}`)
})

test('кнопка смет выключена, если смет нет', async ({ page }) => {
  const employee = await createTemp(admin, created)
  await page.reload()
  await expect(row(page, employee.email).getByRole('button', { name: '0' })).toBeDisabled()
})

})

for (const who of ['invoicer', 'translator'] as const) {
  test.describe(`${who}: страница закрыта`, () => {
    test.use(as(who))
    test('прямой адрес /employees уводит на главную, в меню профиля нет «Сотрудники»', async ({ page }) => {
      await page.goto('/employees')
      await expect(page).toHaveURL(/\/$/)
      await page.locator('summary.app-header-avatar').click()
      await expect(page.getByRole('link', { name: 'Сотрудники' })).toHaveCount(0)
      await expect(page.getByRole('button', { name: 'Выйти' })).toBeVisible()
    })
  })
}
