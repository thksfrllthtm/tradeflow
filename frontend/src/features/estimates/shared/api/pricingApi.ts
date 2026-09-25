import { request } from '@/shared/api/client'

export type PriceGroupInput = { code: string; description: string; quantity: number; unit: string; places?: number; grossKg?: number; netKg?: number }
export type SuggestedPrice = { code: string; unitPrice: number; reason: string }
export type UnitPrice = { code: string; unitPrice: number }

// Подсказки для распределения бюджета: соотношение цен от ИИ (DeepSeek) и цены за единицу
// из прошлых смет. Бюджет на сервер не отправляется — итоговые цены считает браузер.
export const pricingApi = {
  suggestPrices: async (groups: PriceGroupInput[]) =>
    (await request<{ prices: SuggestedPrice[] }>('/api/estimates/price-weights', { method: 'POST', body: JSON.stringify({ currency: 'USD', groups }) })).prices,
  priceHistory: async (kind: 'goods' | 'gloves', estimateId: number, codes: string[]) =>
    (await request<{ prices: UnitPrice[] }>('/api/estimates/price-history', { method: 'POST', body: JSON.stringify({ kind, estimateId, codes }) })).prices,
}
