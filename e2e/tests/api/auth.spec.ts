import { request } from '@playwright/test'
import { test, expect } from '../../support/fixtures'
import { createTempUser, deleteUser, type TempUser } from '../../support/auth-helpers'
import { E2E, uid } from '../../support/users'

// Вход, сессии и регистрация через API. Общие учётки из .auth не выходят и не меняются:
// всё, что ломает сессию, делается на временных сотрудниках.

test.describe('вход и сессия', () => {
  let user: TempUser
  test.beforeAll(async ({ apiAs }) => { user = await createTempUser(await apiAs('admin')) })
  test.afterAll(async ({ apiAs }) => { await deleteUser(await apiAs('admin'), user.id) })

  test('верный пароль → 200, пользователь и HttpOnly-cookie', async ({ apiAs }) => {
    const api = await apiAs('anon')
    const response = await api.post('/api/auth/login', { data: { email: user.email, password: user.password } })
    expect(response.status()).toBe(200)
    const body = await response.json()
    expect(body.user).toMatchObject({ id: user.id, email: user.email, name: user.name, role: 'invoicer' })
    expect(body.user.passwordHash, 'хеш пароля не отдаётся').toBeUndefined()

    const cookie = response.headersArray().find((header) => header.name.toLowerCase() === 'set-cookie')?.value ?? ''
    expect(cookie).toMatch(/HttpOnly/i)
    expect(cookie).toMatch(/SameSite=Lax/i)
    expect(cookie).toMatch(/Path=\//i)
    const token = cookie.split(';')[0].split('=')[1]
    expect(token.length, 'токен длинный и случайный').toBeGreaterThanOrEqual(40)
    expect(JSON.stringify(body), 'токен не уходит в тело ответа').not.toContain(token)

    const me = await api.get('/api/auth/me')
    expect(me.status()).toBe(200)
    expect((await me.json()).user.email).toBe(user.email)
  })

  test('email без учёта регистра и пробелов', async ({ apiAs }) => {
    const response = await (await apiAs('anon')).post('/api/auth/login', { data: { email: `  ${user.email.toUpperCase()} `, password: user.password } })
    expect(response.status()).toBe(200)
  })

  test('каждый вход — новая сессия, старая продолжает работать', async ({ apiAs }) => {
    const first = await apiAs('anon')
    const second = await apiAs('anon')
    await first.post('/api/auth/login', { data: { email: user.email, password: user.password } })
    await second.post('/api/auth/login', { data: { email: user.email, password: user.password } })
    const [a, b] = [(await first.storageState()).cookies[0].value, (await second.storageState()).cookies[0].value]
    expect(a).not.toBe(b)
    expect((await first.get('/api/auth/me')).status()).toBe(200)
    expect((await second.get('/api/auth/me')).status()).toBe(200)
  })

  test('неверный пароль → 401 без подсказки, что email существует', async ({ apiAs }) => {
    const api = await apiAs('anon')
    const wrong = await api.post('/api/auth/login', { data: { email: user.email, password: 'wrong-password' } })
    const unknown = await api.post('/api/auth/login', { data: { email: `nobody-${uid()}@e2e.local`, password: 'wrong-password' } })
    expect(wrong.status()).toBe(401)
    expect(unknown.status()).toBe(401)
    expect((await wrong.json()).error).toEqual((await unknown.json()).error)
    expect((await wrong.json()).error.code).toBe('invalid_credentials')
    expect(wrong.headers()['set-cookie']).toBeUndefined()
  })

  test('выход удаляет сессию на сервере, старый токен больше не работает', async ({ apiAs, baseURL }) => {
    const api = await apiAs('anon')
    await api.post('/api/auth/login', { data: { email: user.email, password: user.password } })
    const saved = await api.storageState()

    const logout = await api.post('/api/auth/logout')
    expect(logout.status()).toBe(204)
    expect(logout.headers()['set-cookie']).toMatch(/Max-Age=0|expires=Thu, 01 Jan 1970/i)
    expect((await api.get('/api/auth/me')).status()).toBe(401)

    // Тот же токен, подставленный вручную, — тоже 401: сессия удалена, а не только cookie.
    const replay = await request.newContext({ baseURL, storageState: saved })
    expect((await replay.get('/api/auth/me')).status()).toBe(401)
    await replay.dispose()
  })

  test('выход без сессии — 204, без ошибки', async ({ apiAs }) => {
    expect((await (await apiAs('anon')).post('/api/auth/logout')).status()).toBe(204)
  })

  test('поддельный токен → 401', async ({ baseURL }) => {
    const api = await request.newContext({ baseURL, extraHTTPHeaders: { Cookie: 'import_calculator_session=forged-token-value' } })
    expect((await api.get('/api/auth/me')).status()).toBe(401)
    expect((await api.get('/api/goods-estimates')).status()).toBe(401)
    await api.dispose()
  })
})

test.describe('проверка тела запроса', () => {
  test('пустые поля → 401, битый JSON → 400', async ({ apiAs }) => {
    const api = await apiAs('anon')
    expect((await api.post('/api/auth/login', { data: {} })).status()).toBe(401)
    expect((await api.post('/api/auth/login', { data: { email: '', password: '' } })).status()).toBe(401)
    const broken = await api.post('/api/auth/login', { data: '{', headers: { 'Content-Type': 'application/json' } })
    expect(broken.status()).toBe(400)
    expect((await broken.json()).error.message).toBe('Введите email и пароль')
  })

  test('слишком большое тело → 413', async ({ apiAs }) => {
    const api = await apiAs('anon')
    const login = await api.post('/api/auth/login', { data: { email: `big-${uid()}@e2e.local`, password: 'x'.repeat(9000) } })
    expect(login.status()).toBe(413)
    expect((await login.json()).error.code).toBe('auth_request_too_large')
    const register = await api.post('/api/auth/register', { data: { name: 'x', email: `big-${uid()}@e2e.local`, password: 'x'.repeat(13000) } })
    expect(register.status()).toBe(413)
  })
})

test('лимит: 11-я попытка входа за минуту → 429', async ({ apiAs }) => {
  const api = await apiAs('anon')
  const email = `rate-${uid()}@e2e.local`
  for (let attempt = 1; attempt <= 10; attempt++) {
    expect((await api.post('/api/auth/login', { data: { email, password: 'wrong-password' } })).status(), `попытка ${attempt}`).toBe(401)
  }
  const limited = await api.post('/api/auth/login', { data: { email, password: 'wrong-password' } })
  expect(limited.status()).toBe(429)
  expect((await limited.json()).error.code).toBe('auth_rate_limited')
  // Лимит — на пару IP+email: другой email с того же адреса входит как обычно.
  expect((await api.post('/api/auth/login', { data: { email: `rate-other-${uid()}@e2e.local`, password: 'wrong-password' } })).status()).toBe(401)
})

test.describe('блокировка и смена роли', () => {
  test('заблокированный не входит, его открытая сессия сразу перестаёт работать', async ({ apiAs }) => {
    const admin = await apiAs('admin')
    const user = await createTempUser(admin)
    try {
      const api = await apiAs('anon')
      expect((await api.post('/api/auth/login', { data: { email: user.email, password: user.password } })).status()).toBe(200)
      expect((await api.get('/api/auth/me')).status()).toBe(200)

      expect((await admin.patch(`/api/employees/${user.id}`, { data: { disabled: true } })).status()).toBe(200)
      expect((await api.get('/api/auth/me')).status()).toBe(401)
      const blocked = await (await apiAs('anon')).post('/api/auth/login', { data: { email: user.email, password: user.password } })
      expect(blocked.status()).toBe(401)
      expect((await blocked.json()).error.code, 'тот же ответ, что на неверный пароль').toBe('invalid_credentials')

      expect((await admin.patch(`/api/employees/${user.id}`, { data: { disabled: false } })).status()).toBe(200)
      expect((await (await apiAs('anon')).post('/api/auth/login', { data: { email: user.email, password: user.password } })).status()).toBe(200)
    } finally {
      await deleteUser(admin, user.id)
    }
  })

  test('смена пароля администратором завершает сессии сотрудника', async ({ apiAs }) => {
    const admin = await apiAs('admin')
    const user = await createTempUser(admin)
    try {
      const api = await apiAs('anon')
      await api.post('/api/auth/login', { data: { email: user.email, password: user.password } })
      const newPassword = `new-${uid()}-password`
      expect((await admin.patch(`/api/employees/${user.id}`, { data: { password: newPassword } })).status()).toBe(200)
      expect((await api.get('/api/auth/me')).status()).toBe(401)
      const anon = await apiAs('anon')
      expect((await anon.post('/api/auth/login', { data: { email: user.email, password: user.password } })).status()).toBe(401)
      expect((await anon.post('/api/auth/login', { data: { email: user.email, password: newPassword } })).status()).toBe(200)
    } finally {
      await deleteUser(admin, user.id)
    }
  })

  test('смена роли меняет доступ в той же сессии сразу', async ({ apiAs }) => {
    const admin = await apiAs('admin')
    const user = await createTempUser(admin, 'invoicer')
    try {
      const api = await apiAs('anon')
      await api.post('/api/auth/login', { data: { email: user.email, password: user.password } })
      expect((await api.get('/api/goods-estimates')).status()).toBe(200)
      expect((await api.get('/api/translator/documents')).status()).toBe(403)
      expect((await api.get('/api/employees')).status()).toBe(403)

      expect((await admin.patch(`/api/employees/${user.id}`, { data: { role: 'translator' } })).status()).toBe(200)
      expect((await (await api.get('/api/auth/me')).json()).user.role).toBe('translator')
      expect((await api.get('/api/goods-estimates')).status()).toBe(403)
      expect((await api.get('/api/translator/documents')).status()).toBe(200)

      expect((await admin.patch(`/api/employees/${user.id}`, { data: { role: 'admin' } })).status()).toBe(200)
      expect((await api.get('/api/employees')).status()).toBe(200)
      expect((await api.get('/api/goods-estimates')).status()).toBe(200)
    } finally {
      await deleteUser(admin, user.id)
    }
  })
})

test.describe('регистрация', () => {
  const created: number[] = []
  test.afterAll(async ({ apiAs }) => {
    const admin = await apiAs('admin')
    for (const id of created) await deleteUser(admin, id)
  })

  test('новый аккаунт — инвойсер, сразу с сессией; повтор email → 409', async ({ apiAs }) => {
    const api = await apiAs('anon')
    const email = `e2e-reg-${uid()}@e2e.local`
    const response = await api.post('/api/auth/register', { data: { name: `${E2E} Регистрация`, email, password: 'register-pass-1' } })
    test.skip(response.status() === 403, 'регистрация закрыта (ALLOW_REGISTRATION=false)')
    expect(response.status()).toBe(201)
    const { user } = await response.json()
    created.push(user.id)
    expect(user).toMatchObject({ email, name: `${E2E} Регистрация`, role: 'invoicer' })
    expect((await api.get('/api/auth/me')).status()).toBe(200)
    expect((await api.get('/api/employees')).status(), 'регистрация не даёт прав администратора').toBe(403)

    const duplicate = await (await apiAs('anon')).post('/api/auth/register', { data: { name: 'Дубль', email: email.toUpperCase(), password: 'register-pass-1' } })
    expect(duplicate.status()).toBe(409)
    expect((await duplicate.json()).error).toMatchObject({ field: 'email', code: 'email_exists' })
  })

  test('проверка полей: имя, email, пароль от 8 символов', async ({ apiAs }) => {
    const api = await apiAs('anon')
    const cases = [
      { data: { name: '', email: `e2e-reg-${uid()}@e2e.local`, password: 'register-pass-1' }, field: 'name' },
      { data: { name: 'x'.repeat(121), email: `e2e-reg-${uid()}@e2e.local`, password: 'register-pass-1' }, field: 'name' },
      { data: { name: 'E2E', email: 'not-an-email', password: 'register-pass-1' }, field: 'email' },
      { data: { name: 'E2E', email: `e2e-reg-${uid()}@e2e.local`, password: '1234567' }, field: 'password' },
    ]
    for (const { data, field } of cases) {
      const response = await api.post('/api/auth/register', { data })
      if (response.status() === 403) test.skip(true, 'регистрация закрыта')
      expect(response.status(), JSON.stringify(data).slice(0, 80)).toBe(400)
      expect((await response.json()).error.field).toBe(field)
    }
  })
})
