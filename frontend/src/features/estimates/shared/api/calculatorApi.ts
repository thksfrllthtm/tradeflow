import { request } from '@/shared/api/client'
import { GlovesCalculationInput, GlovesCalculationResult, GlovesParseResult } from '../types'
import { compressPhoto } from '../lib/compressPhoto'

type ParseXlsxResponse = Omit<GlovesParseResult, 'images'> & {
  images?: { sourceRow: number; mimeType: string; dataIndex: number }[]
  imageData?: string[]
}

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
    const body = await response.json().catch(() => ({})) as { error?: { message?: string } } & ParseXlsxResponse
    if (!response.ok) throw new Error(body.error?.message ?? 'Не удалось обработать файл')
    // Сервер присылает каждую картинку один раз (imageData), а строки ссылаются на неё по номеру:
    // одна фотография часто покрывает несколько строк. Каждая картинка сжимается один раз, по очереди,
    // чтобы не держать в памяти все распакованные фото сразу; строки в JS при присваивании не копируются.
    const { imageData = [], images = [], ...rest } = body
    const photos: { data: string; mimeType: string }[] = []
    for (const [index, data] of imageData.entries()) {
      const mimeType = images.find((image) => image.dataIndex === index)?.mimeType ?? 'image/jpeg'
      photos.push(await compressPhoto({ data, mimeType }))
    }
    return { ...rest, images: images.map(({ sourceRow, dataIndex }) => ({ sourceRow, ...(photos[dataIndex] ?? { data: '', mimeType: 'image/jpeg' }) })) }
  },
  calculate: async (input: GlovesCalculationInput): Promise<GlovesCalculationResult> =>
    request<GlovesCalculationResult>('/api/gloves/calculate', { method: 'POST', body: JSON.stringify(input) }),
  exportXlsx: async (input: GlovesCalculationInput): Promise<Blob> => {
    const response = await fetch('/api/gloves/export', { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) })
    if (!response.ok) { const body = await response.json().catch(() => ({})) as { error?: { message?: string } }; throw new Error(body.error?.message ?? 'Не удалось экспортировать') }
    return response.blob()
  },
}
