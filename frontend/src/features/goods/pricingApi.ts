import { request } from '@/shared/api/client'

// Товар кода: строки одного товара (тот же код и название) склеены, у товара одна цена.
type PriceProductInput = { id: string; name: string; quantity: number; places?: number; grossKg?: number; netKg?: number }
export type PriceGroupInput = { code: string; unit: string; products: PriceProductInput[] }
export type SuggestedPrice = { id: string; code: string; unitPrice: number; reason: string }
type UnitPrice = { code: string; unitPrice: number }

// Подсказки для распределения бюджета: цена каждого товара от ИИ (DeepSeek) и цены за единицу
// из прошлых смет. Бюджет на сервер не отправляется — итоговые цены считает браузер.
export const pricingApi = {
  suggestPrices: async (groups: PriceGroupInput[]) =>
    (await request<{ prices: SuggestedPrice[] }>('/api/estimates/price-weights', { method: 'POST', body: JSON.stringify({ currency: 'USD', groups }) })).prices,
  priceHistory: async (estimateId: number, codes: string[]) =>
    (await request<{ prices: UnitPrice[] }>('/api/estimates/price-history', { method: 'POST', body: JSON.stringify({ estimateId, codes }) })).prices,
}
