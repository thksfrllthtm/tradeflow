import { request } from '@/shared/api/client'
import { goodsApi } from '@/shared/api/goodsApi'
import { GoodsCalculationInput, GoodsCalculationResult, GoodsEstimate, GoodsEstimateInput, GoodsEstimateSummary, GoodsParseResult } from '@/shared/types/goods'

type NBKRate = { currency: string; rate: string; date: string; source: string }

// Изменённые части черновика: поля ввода (строки — без фото), результат (null — убрать)
// и фото изменённых строк по id строки ([] — фото удалено).
export type DraftPatch = { input?: Record<string, unknown>; result?: GoodsCalculationResult | null; photos?: Record<string, string[]> }

// Сметы товаров (/api/goods-estimates) и курс НБРК (/api/rates/nbk); разбор Excel, расчёт
// и экспорт — в goodsApi.
export const goodsEstimatesApi = {
  list: async () => (await request<{ estimates: GoodsEstimateSummary[] }>('/api/goods-estimates')).estimates,
  // Сметы сотрудника — только для администратора (страница «Сотрудники»).
  listForUser: async (userId: number) => (await request<{ estimates: GoodsEstimateSummary[] }>(`/api/goods-estimates?userId=${userId}`)).estimates,
  create: async (input: GoodsEstimateInput) => (await request<{ estimate: GoodsEstimateSummary }>('/api/goods-estimates', { method: 'POST', body: JSON.stringify(input) })).estimate,
  get: async (id: number) => (await request<{ estimate: GoodsEstimate }>('/api/goods-estimates/' + id)).estimate,
  update: async (id: number, input: GoodsEstimateInput) => (await request<{ estimate: GoodsEstimateSummary }>('/api/goods-estimates/' + id, { method: 'PATCH', body: JSON.stringify(input) })).estimate,
  remove: (id: number) => request<void>('/api/goods-estimates/' + id, { method: 'DELETE' }),
  patchDraft: async (id: number, patch: DraftPatch) => (await request<{ estimate: GoodsEstimateSummary }>('/api/goods-estimates/' + id + '/draft', { method: 'PATCH', body: JSON.stringify(patch) })).estimate,
  usdRate: async (assessmentDate: string): Promise<NBKRate> => (await request<{ rate: NBKRate }>(`/api/rates/nbk?date=${encodeURIComponent(assessmentDate)}`)).rate,
  // Курс евро — только для справки на главной; расчёт идёт в долларах.
  eurRate: async (assessmentDate: string): Promise<NBKRate> => (await request<{ rate: NBKRate }>(`/api/rates/nbk?date=${encodeURIComponent(assessmentDate)}&currency=EUR`)).rate,
  parseXlsx: (file: File, options?: { sheet?: string; headerRow?: number; dataStartRow?: number }): Promise<GoodsParseResult> => goodsApi.parseXlsx(file, { ...options, shareImages: true }),
  calculate: (input: GoodsCalculationInput): Promise<GoodsCalculationResult> => goodsApi.calculate(input),
  exportXlsx: (input: GoodsCalculationInput): Promise<Blob> => goodsApi.exportXlsx(input),
}
