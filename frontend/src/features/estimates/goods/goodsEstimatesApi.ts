import { request } from '@/shared/api/client'
import { calculatorApi } from '../shared/api/calculatorApi'
import { GlovesCalculationInput, GlovesCalculationResult, GlovesEstimate, GlovesEstimateInput, GlovesEstimateSummary, GlovesParseResult } from '../shared/types'

type NBKRate = { currency: string; rate: string; date: string; source: string }

// Изменённые части черновика: поля ввода (строки — без фото), результат (null — убрать)
// и фото изменённых строк по id строки ([] — фото удалено).
export type DraftPatch = { input?: Record<string, unknown>; result?: GlovesCalculationResult | null; photos?: Record<string, string[]> }

// Сметы товаров (backend: /api/gloves-copy; адрес остался от первой версии калькулятора),
// а разбор Excel, расчёт и экспорт используют общий calculatorApi.
export const goodsEstimatesApi = {
  list: async () => (await request<{ estimates: GlovesEstimateSummary[] }>('/api/gloves-copy/estimates')).estimates,
  // Сметы сотрудника — только для администратора (страница «Сотрудники»).
  listForUser: async (userId: number) => (await request<{ estimates: GlovesEstimateSummary[] }>(`/api/gloves-copy/estimates?userId=${userId}`)).estimates,
  create: async (input: GlovesEstimateInput) => (await request<{ estimate: GlovesEstimateSummary }>('/api/gloves-copy/estimates', { method: 'POST', body: JSON.stringify(input) })).estimate,
  get: async (id: number) => (await request<{ estimate: GlovesEstimate }>('/api/gloves-copy/estimates/' + id)).estimate,
  update: async (id: number, input: GlovesEstimateInput) => (await request<{ estimate: GlovesEstimateSummary }>('/api/gloves-copy/estimates/' + id, { method: 'PATCH', body: JSON.stringify(input) })).estimate,
  remove: (id: number) => request<void>('/api/gloves-copy/estimates/' + id, { method: 'DELETE' }),
  patchDraft: async (id: number, patch: DraftPatch) => (await request<{ estimate: GlovesEstimateSummary }>('/api/gloves-copy/estimates/' + id + '/draft', { method: 'PATCH', body: JSON.stringify(patch) })).estimate,
  usdRate: async (assessmentDate: string): Promise<NBKRate> => (await request<{ rate: NBKRate }>(`/api/gloves-copy/usd-rate?date=${encodeURIComponent(assessmentDate)}`)).rate,
  // Курс евро — только для справки на главной; расчёт идёт в долларах.
  eurRate: async (assessmentDate: string): Promise<NBKRate> => (await request<{ rate: NBKRate }>(`/api/gloves-copy/usd-rate?date=${encodeURIComponent(assessmentDate)}&currency=EUR`)).rate,
  parseXlsx: (file: File, options?: { sheet?: string; headerRow?: number; dataStartRow?: number }): Promise<GlovesParseResult> => calculatorApi.parseXlsx(file, { ...options, shareImages: true }),
  calculate: (input: GlovesCalculationInput): Promise<GlovesCalculationResult> => calculatorApi.calculate(input),
  exportXlsx: (input: GlovesCalculationInput): Promise<Blob> => calculatorApi.exportXlsx(input),
}
