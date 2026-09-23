import { request } from '@/shared/api/client'
import { calculatorApi } from '../shared/api/calculatorApi'
import { GlovesCalculationInput, GlovesCalculationResult, GlovesDraft, GlovesEstimate, GlovesEstimateInput, GlovesEstimateSummary, GlovesParseResult } from '../shared/types'

type NBKRate = { currency: string; rate: string; date: string; source: string }

// Сметы товаров. Хранятся отдельно от смет перчаток (backend: /api/gloves-copy),
// а разбор Excel, расчёт и экспорт используют общий calculatorApi.
export const goodsEstimatesApi = {
  list: async () => (await request<{ estimates: GlovesEstimateSummary[] }>('/api/gloves-copy/estimates')).estimates,
  create: async (input: GlovesEstimateInput) => (await request<{ estimate: GlovesEstimateSummary }>('/api/gloves-copy/estimates', { method: 'POST', body: JSON.stringify(input) })).estimate,
  get: async (id: number) => (await request<{ estimate: GlovesEstimate }>('/api/gloves-copy/estimates/' + id)).estimate,
  update: async (id: number, input: GlovesEstimateInput) => (await request<{ estimate: GlovesEstimateSummary }>('/api/gloves-copy/estimates/' + id, { method: 'PATCH', body: JSON.stringify(input) })).estimate,
  remove: (id: number) => request<void>('/api/gloves-copy/estimates/' + id, { method: 'DELETE' }),
  saveDraft: async (id: number, draft: Pick<GlovesDraft, 'input' | 'result'>) => (await request<{ estimate: GlovesEstimate }>('/api/gloves-copy/estimates/' + id + '/draft', { method: 'PUT', body: JSON.stringify(draft) })).estimate,
  usdRate: async (assessmentDate: string): Promise<NBKRate> => (await request<{ rate: NBKRate }>(`/api/gloves-copy/usd-rate?date=${encodeURIComponent(assessmentDate)}`)).rate,
  parseXlsx: (file: File, options?: { sheet?: string; headerRow?: number; dataStartRow?: number }): Promise<GlovesParseResult> => calculatorApi.parseXlsx(file, { ...options, shareImages: true }),
  calculate: (input: GlovesCalculationInput): Promise<GlovesCalculationResult> => calculatorApi.calculate(input),
  exportXlsx: (input: GlovesCalculationInput): Promise<Blob> => calculatorApi.exportXlsx(input),
}
