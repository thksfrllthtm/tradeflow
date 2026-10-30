import type { APIRequestContext } from '@playwright/test'
import { E2E, uid, type Role } from './users'

export type TempUser = { id: number; email: string; password: string; name: string; role: Role }

// Временный сотрудник для тестов, которые выходят, блокируют или меняют роль: общие учётки
// из .auth трогать нельзя — их сессии нужны остальным тестам.
export async function createTempUser(admin: APIRequestContext, role: Role = 'invoicer'): Promise<TempUser> {
  const id = uid()
  const user = { email: `e2e-tmp-${id}@e2e.local`, password: `pass-${id}-xyz`, name: `${E2E} Временный ${id}`, role }
  const response = await admin.post('/api/employees', { data: user })
  if (!response.ok()) throw new Error(`createTempUser: ${response.status()} ${await response.text()}`)
  return { ...user, id: (await response.json()).item.id }
}

export async function deleteUser(admin: APIRequestContext, id: number) {
  await admin.delete(`/api/employees/${id}`)
}
