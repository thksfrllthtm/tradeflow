import { request } from '@/shared/api/client'
import { AIStatus, AnalyzeItem, CeramicsProduct, CeramicsProductInput, CeramicsType, PriceResult, Resolution, SizeUnit } from './types'

const post = (body: unknown): RequestInit => ({ method: 'POST', body: JSON.stringify(body) })

// API модуля керамики. С расчётами смет и растаможки не связан.
export const ceramicsApi = {
  status: () => request<{ ai: AIStatus }>('/api/ceramics/status').then((body) => body.ai),
  // Типы товаров — русские наименования базы без размера.
  types: () => request<{ items: CeramicsType[] }>('/api/ceramics/types').then((body) => body.items),

  products: (query: { q?: string; type?: string; limit?: number; offset?: number }) => {
    const params = new URLSearchParams()
    if (query.q) params.set('q', query.q)
    if (query.type) params.set('type', query.type)
    params.set('limit', String(query.limit ?? 100))
    params.set('offset', String(query.offset ?? 0))
    return request<{ items: CeramicsProduct[]; total: number }>(`/api/ceramics/products?${params}`)
  },
  createProduct: (input: CeramicsProductInput) => request<{ item: CeramicsProduct }>('/api/ceramics/products', post(input)).then((body) => body.item),
  updateProduct: (id: number, input: CeramicsProductInput) =>
    request<{ item: CeramicsProduct }>(`/api/ceramics/products/${id}`, { method: 'PATCH', body: JSON.stringify(input) }).then((body) => body.item),
  deleteProduct: (id: number) => request<void>(`/api/ceramics/products/${id}`, { method: 'DELETE' }),
  importProducts: (items: CeramicsProductInput[], overwrite: boolean) =>
    request<{ created: number; updated: number; skipped: number }>('/api/ceramics/products/import', post({ items, overwrite })),

  // Точный поиск марок в базе, без ИИ.
  lookup: (marks: string[]) => request<{ items: Resolution[] }>('/api/ceramics/lookup', post({ marks })).then((body) => body.items),
  // Подбор неизвестных марок небольшими порциями (до 5 товаров).
  analyze: (items: AnalyzeItem[]) => request<{ items: Resolution[] }>('/api/ceramics/analyze', post({ items })).then((body) => body.items),
  price: (productType: string, sizeValue: string, sizeUnit: SizeUnit | '') =>
    request<{ price: PriceResult }>('/api/ceramics/price', post({ productType, sizeValue, sizeUnit })).then((body) => body.price),
}
