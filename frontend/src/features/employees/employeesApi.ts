import { request } from '@/shared/api/client'
import { Role } from '@/shared/types/user'

export type Employee = { id: number; name: string; email: string; role: Role; disabled: boolean; createdAt: string; estimateCount: number }
export type EmployeeInput = { name: string; email: string; password: string; role: Role }
export type EmployeeUpdate = Partial<{ name: string; email: string; role: Role; disabled: boolean; password: string }>

// Управление сотрудниками — только для администратора (/api/employees).
export const employeesApi = {
  list: async () => (await request<{ items: Employee[] }>('/api/employees')).items,
  create: async (input: EmployeeInput) => (await request<{ item: Employee }>('/api/employees', { method: 'POST', body: JSON.stringify(input) })).item,
  update: async (id: number, update: EmployeeUpdate) => (await request<{ item: Employee }>(`/api/employees/${id}`, { method: 'PATCH', body: JSON.stringify(update) })).item,
  // Сметы и записи справочников сотрудника переходят к администратору; ответ — сколько смет перешло.
  remove: async (id: number) => (await request<{ movedEstimates: number }>(`/api/employees/${id}`, { method: 'DELETE' })).movedEstimates,
}
