import type { APIRequestContext } from '@playwright/test'
import { E2E } from './users'

// Записи базы решений для тестов. Без фото: фото запускают фоновое описание платной моделью.
// Поиск по тексту с буквами идёт ещё и через платные эмбеддинги, поэтому тесты ищут только по коду:
// у каждого теста свой «код» 99xxxxxxxx — таких групп в ТН ВЭД нет, поиск по нему находит только свои записи.

export const realCode = '8471300000'

// 8 случайных цифр: префикс кода теста (99 + 7 цифр), десятая цифра — номер записи.
export function codePrefix() {
  return '99' + String(Date.now() % 1e4).padStart(4, '0') + String(Math.floor(Math.random() * 1e3)).padStart(3, '0')
}

export const decisionName = (tag: string) => `${E2E}-${tag} тестовый товар`

// Запоминает созданные записи и удаляет их администратором в afterEach.
export class DecisionTracker {
  ids = new Set<number>()
  constructor(private admin: () => Promise<APIRequestContext>) {}

  track(id: number) { this.ids.add(id); return id }

  async cleanup() {
    if (this.ids.size === 0) return
    const admin = await this.admin()
    for (const id of this.ids) await admin.delete(`/api/code-decisions/${id}`)
    this.ids.clear()
  }

  // Подчистить по коду: записи, созданные через UI или импорт, id которых тест не знает.
  async cleanupByCode(prefix: string) {
    const admin = await this.admin()
    const list = await (await admin.get(`/api/code-decisions?q=${prefix}&limit=200`)).json()
    for (const item of list.items ?? []) if (String(item.code).startsWith(prefix)) await admin.delete(`/api/code-decisions/${item.id}`)
  }
}

export async function createDecision(api: APIRequestContext, input: Record<string, unknown>) {
  const response = await api.post('/api/code-decisions', { data: input })
  return { response, body: await response.json() }
}
