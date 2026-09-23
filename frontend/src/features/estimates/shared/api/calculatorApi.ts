import { request } from '@/shared/api/client'
import { GlovesCalculationInput, GlovesCalculationResult, GlovesParseResult } from '../types'

// Разбор Excel, расчёт и экспорт — общий backend для обеих смет (/api/gloves/*).
export const calculatorApi = {
  parseXlsx: async (file: File, options?: { sheet?: string; headerRow?: number; dataStartRow?: number; shareImages?: boolean }): Promise<GlovesParseResult> => {
    const form = new FormData(); form.append('file', file)
    const query = new URLSearchParams()
    if (options?.sheet) query.set('sheet', options.sheet)
    if (options?.headerRow) query.set('headerRow', String(options.headerRow))
    if (options?.dataStartRow) query.set('dataStartRow', String(options.dataStartRow))
    if (options?.shareImages) query.set('shareImages', 'true')
    const response = await fetch(`/api/gloves/parse-xlsx?${query}`, { method: 'POST', credentials: 'include', body: form })
    const body = await response.json().catch(() => ({})) as { error?: { message?: string } } & GlovesParseResult
    if (!response.ok) throw new Error(body.error?.message ?? 'Не удалось обработать файл')
    return body
  },
  calculate: async (input: GlovesCalculationInput): Promise<GlovesCalculationResult> =>
    request<GlovesCalculationResult>('/api/gloves/calculate', { method: 'POST', body: JSON.stringify(input) }),
  exportXlsx: async (input: GlovesCalculationInput): Promise<Blob> => {
    const response = await fetch('/api/gloves/export', { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) })
    if (!response.ok) { const body = await response.json().catch(() => ({})) as { error?: { message?: string } }; throw new Error(body.error?.message ?? 'Не удалось экспортировать') }
    return response.blob()
  },
}
