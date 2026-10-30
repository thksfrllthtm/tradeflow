import { expect, request, type APIRequestContext } from '@playwright/test'
import { E2E, uid } from './users'

export type Employee = { id: number; name: string; email: string; role: string; disabled: boolean; estimateCount: number }

export const tmpEmail = () => `e2e-tmp-${uid()}@e2e.local`
export const tmpPassword = 'e2e-tmp-pass-123'

// Временный сотрудник; id копятся в created, чтобы afterEach их удалил.
export async function createTemp(admin: APIRequestContext, created: number[], role = 'invoicer', password = tmpPassword) {
  const email = tmpEmail()
  const response = await admin.post('/api/employees', { data: { name: `${E2E} ${uid()}`, email, password, role } })
  expect(response.status(), await response.text()).toBe(201)
  const item = (await response.json()).item as Employee
  created.push(item.id)
  return { ...item, password }
}

export async function removeTemp(admin: APIRequestContext, created: number[]) {
  for (const id of created.splice(0)) await admin.delete(`/api/employees/${id}`)
}

// Отдельный клиент со своей сессией: вход под временным сотрудником.
export async function loginAs(baseURL: string | undefined, email: string, password: string) {
  const context = await request.newContext({ baseURL })
  const response = await context.post('/api/auth/login', { data: { email, password } })
  return { context, status: response.status() }
}

export async function listEmployees(admin: APIRequestContext) {
  return (await (await admin.get('/api/employees')).json()).items as Employee[]
}
