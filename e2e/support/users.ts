import path from 'node:path'

export type Role = 'admin' | 'invoicer' | 'translator'

// Постоянные учётки для E2E. Создаются один раз (администратор — через bootstrap-admin,
// остальные — администратором через API) и переиспользуются между прогонами.
export const users = {
  admin: { email: process.env.E2E_ADMIN_EMAIL ?? 'e2e-admin@e2e.local', password: process.env.E2E_ADMIN_PASSWORD ?? 'e2e-admin-password-123', name: 'E2E Администратор', role: 'admin' },
  invoicer: { email: 'e2e-invoicer@e2e.local', password: 'e2e-invoicer-pass-123', name: 'E2E Инвойсер', role: 'invoicer' },
  invoicer2: { email: 'e2e-invoicer2@e2e.local', password: 'e2e-invoicer2-pass-123', name: 'E2E Инвойсер 2', role: 'invoicer' },
  translator: { email: 'e2e-translator@e2e.local', password: 'e2e-translator-pass-123', name: 'E2E Переводчик', role: 'translator' },
} as const

export type UserKey = keyof typeof users

// Сервер при создании сметы берёт заново пустую смету сотрудника и удаляет другие его пустые
// сметы. Параллельные тесты под одной учёткой отнимали бы сметы друг у друга, поэтому у каждого
// процесса Playwright свой инвойсер (то же имя, другой email): `as('invoicer')` и
// `apiAs('invoicer')` подставляют его сами.
export const MAX_WORKERS = 8
export type WorkerKey = `invoicer-w${number}`
export const workerInvoicer = (index: number) => ({
  key: `invoicer-w${index}` as WorkerKey,
  email: `e2e-invoicer-w${index}@e2e.local`,
  password: users.invoicer.password,
  name: users.invoicer.name,
  role: 'invoicer' as const,
})

export const authFile = (key: UserKey | WorkerKey) => path.join(import.meta.dirname, '..', '.auth', `${key}.json`)

// Префикс всех данных, которые создают тесты: по нему teardown подчищает остатки.
export const E2E = 'E2E'
export const uid = () => `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
