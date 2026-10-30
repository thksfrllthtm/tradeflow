import type { APIRequestContext } from '@playwright/test'
import { test, expect } from '../../support/fixtures'
import { E2E, uid } from '../../support/users'
import { createTemp, listEmployees, loginAs, removeTemp, tmpEmail, tmpPassword } from '../../support/employees-helpers'

// API «Сотрудники» (/api/employees): создание, проверки, изменение, блокировка, удаление.
// Каждый тест заводит своих временных сотрудников e2e-tmp-*@e2e.local и удаляет их в afterEach.

const created: number[] = []
let admin: APIRequestContext

test.beforeEach(async ({ apiAs }) => { admin = await apiAs('admin') })
test.afterEach(async () => { await removeTemp(admin, created) })

const post = (data: object) => admin.post('/api/employees', { data })

test.describe('создание', () => {
  for (const role of ['admin', 'invoicer', 'translator']) {
    test(`роль ${role}: создаётся, входит, /me возвращает роль`, async ({ baseURL }) => {
      const employee = await createTemp(admin, created, role)
      expect(employee).toMatchObject({ role, disabled: false, estimateCount: 0 })
      expect(employee.id).toBeGreaterThan(0)
      expect((await listEmployees(admin)).some((item) => item.id === employee.id)).toBe(true)

      const { context, status } = await loginAs(baseURL, employee.email, employee.password)
      expect(status).toBe(200)
      const me = await (await context.get('/api/auth/me')).json()
      expect(me.user).toMatchObject({ id: employee.id, email: employee.email, role })
      await context.dispose()
    })
  }

  test('пароль в ответе не возвращается, имя и email обрезаются по краям', async () => {
    const email = tmpEmail()
    const response = await post({ name: `  ${E2E} пробелы  `, email: `  ${email}  `, password: tmpPassword, role: 'invoicer' })
    expect(response.status(), await response.text()).toBe(201)
    const item = (await response.json()).item
    created.push(item.id)
    expect(item.name).toBe(`${E2E} пробелы`)
    expect(item.email).toBe(email)
    expect(JSON.stringify(item)).not.toContain(tmpPassword)
    expect(item).not.toHaveProperty('passwordHash')
  })

  const invalid: Array<[string, object, string]> = [
    ['пустое имя', { name: '   ' }, 'name'],
    ['имя длиннее 120 символов', { name: 'Я'.repeat(121) }, 'name'],
    ['email без @', { email: 'not-an-email' }, 'email'],
    ['email с именем «Имя <a@b>»', { email: 'Имя <x@e2e.local>' }, 'email'],
    ['пароль короче 8 символов', { password: '1234567' }, 'password'],
    ['неизвестная роль', { role: 'superuser' }, 'role'],
    ['пустая роль', { role: '' }, 'role'],
  ]
  for (const [title, override, field] of invalid) {
    test(`отказ 400: ${title}`, async () => {
      const response = await post({ name: `${E2E} ${uid()}`, email: tmpEmail(), password: tmpPassword, role: 'invoicer', ...override })
      expect(response.status(), await response.text()).toBe(400)
      const body = await response.json()
      expect(body.error.field).toBe(field)
      expect(body.error.message).toBeTruthy()
    })
  }

  test('имя ровно 120 символов допустимо', async () => {
    const response = await post({ name: `${E2E}${'я'.repeat(117)}`, email: tmpEmail(), password: tmpPassword, role: 'invoicer' })
    expect(response.status(), await response.text()).toBe(201)
    created.push((await response.json()).item.id)
  })

  test('битый JSON → 400', async () => {
    const response = await admin.post('/api/employees', { data: '{', headers: { 'Content-Type': 'application/json' } })
    expect(response.status()).toBe(400)
  })

  test('дубль email → 409, в том числе другим регистром', async () => {
    const employee = await createTemp(admin, created)
    for (const email of [employee.email, employee.email.toUpperCase()]) {
      const response = await post({ name: `${E2E} дубль`, email, password: tmpPassword, role: 'invoicer' })
      expect(response.status(), `${email}: ${await response.text()}`).toBe(409)
      expect((await response.json()).error.field).toBe('email')
    }
    expect((await listEmployees(admin)).filter((item) => item.email.toLowerCase() === employee.email)).toHaveLength(1)
  })
})

test.describe('изменение', () => {
  test('имя, email и роль меняются', async ({ baseURL }) => {
    const employee = await createTemp(admin, created)
    const email = tmpEmail()
    const response = await admin.patch(`/api/employees/${employee.id}`, { data: { name: `${E2E} новое имя`, email, role: 'translator' } })
    expect(response.status(), await response.text()).toBe(200)
    expect((await response.json()).item).toMatchObject({ id: employee.id, name: `${E2E} новое имя`, email, role: 'translator' })

    expect((await loginAs(baseURL, employee.email, employee.password)).status, 'старый email больше не входит').toBe(401)
    const { context, status } = await loginAs(baseURL, email, employee.password)
    expect(status).toBe(200)
    expect((await (await context.get('/api/auth/me')).json()).user.role).toBe('translator')
    await context.dispose()
  })

  test('пустой PATCH ничего не меняет', async () => {
    const employee = await createTemp(admin, created)
    const response = await admin.patch(`/api/employees/${employee.id}`, { data: {} })
    expect(response.status()).toBe(200)
    expect((await response.json()).item).toMatchObject({ name: employee.name, email: employee.email, role: employee.role })
  })

  test('проверки при изменении: пустое имя, плохой email, роль, короткий пароль', async () => {
    const employee = await createTemp(admin, created)
    for (const [data, field] of [[{ name: '' }, 'name'], [{ email: 'bad' }, 'email'], [{ role: 'boss' }, 'role'], [{ password: 'short' }, 'password']] as const) {
      const response = await admin.patch(`/api/employees/${employee.id}`, { data })
      expect(response.status(), JSON.stringify(data)).toBe(400)
      expect((await response.json()).error.field).toBe(field)
    }
  })

  test('email другого сотрудника → 409; свой же email другим регистром — не ошибка', async () => {
    const first = await createTemp(admin, created)
    const second = await createTemp(admin, created)
    const taken = await admin.patch(`/api/employees/${second.id}`, { data: { email: first.email.toUpperCase() } })
    expect(taken.status()).toBe(409)
    const own = await admin.patch(`/api/employees/${second.id}`, { data: { email: second.email.toUpperCase() } })
    expect(own.status(), await own.text()).toBe(200)
  })

  test('смена пароля: старый не входит, новый входит, прежние сессии завершены', async ({ baseURL }) => {
    const employee = await createTemp(admin, created)
    const before = await loginAs(baseURL, employee.email, employee.password)
    expect(before.status).toBe(200)

    const newPassword = 'e2e-new-pass-456'
    expect((await admin.patch(`/api/employees/${employee.id}`, { data: { password: newPassword } })).status()).toBe(200)

    expect((await before.context.get('/api/auth/me')).status(), 'старая сессия закрыта').toBe(401)
    expect((await loginAs(baseURL, employee.email, employee.password)).status).toBe(401)
    const after = await loginAs(baseURL, employee.email, newPassword)
    expect(after.status).toBe(200)
    await Promise.all([before.context.dispose(), after.context.dispose()])
  })

  test('блокировка: сессия завершена, вход закрыт; разблокировка возвращает вход', async ({ baseURL }) => {
    const employee = await createTemp(admin, created)
    const session = await loginAs(baseURL, employee.email, employee.password)

    const blocked = await admin.patch(`/api/employees/${employee.id}`, { data: { disabled: true } })
    expect((await blocked.json()).item.disabled).toBe(true)
    expect((await session.context.get('/api/auth/me')).status()).toBe(401)
    expect((await loginAs(baseURL, employee.email, employee.password)).status).toBe(401)
    expect((await listEmployees(admin)).find((item) => item.id === employee.id)?.disabled).toBe(true)

    const unblocked = await admin.patch(`/api/employees/${employee.id}`, { data: { disabled: false } })
    expect((await unblocked.json()).item.disabled).toBe(false)
    const again = await loginAs(baseURL, employee.email, employee.password)
    expect(again.status).toBe(200)
    await Promise.all([session.context.dispose(), again.context.dispose()])
  })

  test('администратор не может снять роль с себя и заблокировать себя', async () => {
    const me = (await (await admin.get('/api/auth/me')).json()).user
    for (const data of [{ role: 'invoicer' }, { disabled: true }]) {
      const response = await admin.patch(`/api/employees/${me.id}`, { data })
      expect(response.status(), JSON.stringify(data)).toBe(400)
    }
    expect((await (await admin.get('/api/auth/me')).json()).user.role, 'роль не изменилась').toBe('admin')
  })

  test('другой администратор понижается, если админов больше одного', async () => {
    const other = await createTemp(admin, created, 'admin')
    const response = await admin.patch(`/api/employees/${other.id}`, { data: { role: 'invoicer' } })
    expect(response.status(), await response.text()).toBe(200)
    expect((await response.json()).item.role).toBe('invoicer')
  })

  test('несуществующий сотрудник → 404, неверный id → 404', async () => {
    expect((await admin.patch('/api/employees/999999999', { data: { name: 'x' } })).status()).toBe(404)
    expect((await admin.patch('/api/employees/abc', { data: { name: 'x' } })).status()).toBe(404)
    expect((await admin.patch('/api/employees/0', { data: { name: 'x' } })).status()).toBe(404)
  })
})

test.describe('удаление', () => {
  test('сметы удалённого сотрудника переходят администратору; estimateCount считается', async ({ baseURL }) => {
    const employee = await createTemp(admin, created)
    const { context } = await loginAs(baseURL, employee.email, employee.password)
    const name = `${E2E} смета ${uid()}`
    const estimateResponse = await context.post('/api/goods-estimates', { data: { name, client: `${E2E} клиент`, assessmentDate: '2026-10-30' } })
    expect(estimateResponse.status(), await estimateResponse.text()).toBe(201)
    const estimateId = (await estimateResponse.json()).estimate.id

    expect((await listEmployees(admin)).find((item) => item.id === employee.id)?.estimateCount).toBe(1)

    const removed = await admin.delete(`/api/employees/${employee.id}`)
    expect(removed.status(), await removed.text()).toBe(200)
    expect((await removed.json()).movedEstimates).toBe(1)
    created.splice(created.indexOf(employee.id), 1)

    expect((await context.get('/api/auth/me')).status(), 'сессия удалённого закрыта').toBe(401)
    expect((await loginAs(baseURL, employee.email, employee.password)).status).toBe(401)
    expect((await listEmployees(admin)).some((item) => item.id === employee.id)).toBe(false)

    const mine = (await (await admin.get('/api/goods-estimates')).json()).estimates as Array<{ id: number; name: string }>
    expect(mine.find((item) => item.id === estimateId)?.name).toBe(name)
    expect((await admin.delete(`/api/goods-estimates/${estimateId}`)).ok()).toBe(true)
    await context.dispose()
  })

  test('сотрудник без смет: movedEstimates = 0; повторное удаление → 404', async () => {
    const employee = await createTemp(admin, created, 'translator')
    const removed = await admin.delete(`/api/employees/${employee.id}`)
    expect((await removed.json()).movedEstimates).toBe(0)
    created.splice(created.indexOf(employee.id), 1)
    expect((await admin.delete(`/api/employees/${employee.id}`)).status()).toBe(404)
  })

  test('удалить себя нельзя', async () => {
    const me = (await (await admin.get('/api/auth/me')).json()).user
    const response = await admin.delete(`/api/employees/${me.id}`)
    expect(response.status()).toBe(400)
    expect((await response.json()).error.message).toContain('себя')
    expect((await admin.get('/api/auth/me')).status()).toBe(200)
  })

  test('другого администратора удалить можно, если админов больше одного', async () => {
    const other = await createTemp(admin, created, 'admin')
    expect((await admin.delete(`/api/employees/${other.id}`)).status()).toBe(200)
    created.splice(created.indexOf(other.id), 1)
  })

  test('несуществующий сотрудник → 404', async () => {
    expect((await admin.delete('/api/employees/999999999')).status()).toBe(404)
    expect((await admin.delete('/api/employees/-1')).status()).toBe(404)
  })
})

test('временный администратор управляет сотрудниками, после понижения — 403', async ({ baseURL }) => {
  const other = await createTemp(admin, created, 'admin')
  const { context } = await loginAs(baseURL, other.email, other.password)
  expect((await context.get('/api/employees')).status()).toBe(200)
  await admin.patch(`/api/employees/${other.id}`, { data: { role: 'invoicer' } })
  // Смена роли не завершает сессию, но права проверяются на каждом запросе.
  expect((await context.get('/api/employees')).status()).toBe(403)
  await context.dispose()
})
