import { execSync } from 'node:child_process'
import path from 'node:path'
import { existsSync, mkdirSync } from 'node:fs'
import { expect, request, test as setup } from '@playwright/test'
import { authFile, MAX_WORKERS, users, workerInvoicer, type UserKey, type WorkerKey } from '../support/users'

const root = path.join(import.meta.dirname, '..', '..')

// Первый администратор создаётся только через CLI bootstrap-admin внутри контейнера backend
// (dev-образ — `go run`, прод-образ — готовый бинарник).
function bootstrapAdmin() {
  const { email, password } = users.admin
  const script = 'if [ -x /app/bootstrap-admin ]; then /app/bootstrap-admin; else go run ./cmd/bootstrap-admin; fi'
  execSync(`docker compose exec -T -e BOOTSTRAP_ADMIN_EMAIL=${email} -e BOOTSTRAP_ADMIN_PASSWORD=${password} backend sh -c "${script}"`, { cwd: root, stdio: 'inherit', timeout: 180_000 })
}

type Account = { key: UserKey | WorkerKey; email: string; password: string; name: string; role: string }
const account = (key: UserKey): Account => ({ key, ...users[key] })

// Сохранённая сессия ещё жива? Тогда не входим заново: вход ограничен 10 попытками в минуту.
async function sessionAlive(baseURL: string | undefined, user: Account) {
  if (!existsSync(authFile(user.key))) return false
  const context = await request.newContext({ baseURL, storageState: authFile(user.key) })
  const me = await context.get('/api/auth/me')
  const body = me.ok() ? await me.json() : {}
  await context.dispose()
  return body.user?.email === user.email && body.user?.role === user.role
}

async function login(baseURL: string | undefined, user: Account) {
  const context = await request.newContext({ baseURL })
  const response = await context.post('/api/auth/login', { data: { email: user.email, password: user.password } })
  if (response.ok()) await context.storageState({ path: authFile(user.key) })
  const result = { status: response.status(), ok: response.ok(), text: await response.text() }
  await context.dispose()
  return result
}

setup('учётки E2E и сохранённые сессии', async ({ baseURL }) => {
  setup.setTimeout(240_000)
  mkdirSync(path.join(import.meta.dirname, '..', '.auth'), { recursive: true })

  const health = await (await request.newContext({ baseURL })).get('/api/health')
  expect(health.status(), `Стек не запущен на ${baseURL}: выполните docker compose up`).toBe(204)

  if (!await sessionAlive(baseURL, account('admin'))) {
    let response = await login(baseURL, account('admin'))
    if (response.status === 401) {
      bootstrapAdmin()
      response = await login(baseURL, account('admin'))
    }
    expect(response.status, response.text).toBe(200)
  }

  // Остальные роли и инвойсеры процессов: создать или вернуть в исходное состояние
  // (пароль, роль, без блокировки).
  const admin = await request.newContext({ baseURL, storageState: authFile('admin') })
  const accounts = [...(['invoicer', 'invoicer2', 'translator'] as const).map(account), ...Array.from({ length: MAX_WORKERS }, (_, index) => workerInvoicer(index))]
  for (const user of accounts) {
    if (await sessionAlive(baseURL, user)) continue
    let response = await login(baseURL, user)
    if (!response.ok) {
      const list = (await (await admin.get('/api/employees')).json()).items as Array<{ id: number; email: string }>
      const existing = list.find((item) => item.email.toLowerCase() === user.email)
      const saved = existing
        ? await admin.patch(`/api/employees/${existing.id}`, { data: { name: user.name, role: user.role, password: user.password, disabled: false } })
        : await admin.post('/api/employees', { data: { name: user.name, email: user.email, role: user.role, password: user.password } })
      expect(saved.ok(), await saved.text()).toBeTruthy()
      response = await login(baseURL, user)
    }
    expect(response.status, response.text).toBe(200)
  }
  await admin.dispose()
})
