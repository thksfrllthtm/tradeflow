import { request } from '@/shared/api/client'
import { formatRuDate } from '@/shared/lib/dates'

// Таможенный реестр объектов интеллектуальной собственности КГД (ТРОИС): знаки, найденные сервером
// в наименованиях товара, список реестра, загрузка файла КГД и подпись логотипа (администратор).

// kind: brand — знак как в реестре; lookalike — похожее написание (возможная имитация).
// name — наименование знака; fromLogo — это надпись логотипа, прочитанная ИИ или подписанная вручную.
export type TrademarkHit = {
  regNo: string; name: string; fromLogo: boolean; hasLogo: boolean; holder: string; goods: string
  protectedUntil?: string; kind: 'brand' | 'lookalike'; found: string
  // Группа товаров: та же по коду или по описанию товаров записи; otherGoods — код известен, а знак для других товаров.
  sameGoods: boolean; goodsMatch?: 'code' | 'description'; otherGoods?: boolean
  // byPicture — найдено сверкой фото товара с логотипом реестра; why — что похоже.
  byPicture?: boolean; why?: string
}
export type TrademarkSummary = { total: number; logos: number; logosRead: number; importedAt: string; importedByName: string }
export type TrademarkItem = {
  regNo: string; name: string; hasLogo: boolean; logoName: string; logoNameSource: '' | 'ai' | 'admin'; logoRead: boolean
  goods: string; codes: string[]; holder: string; document: string; protectedUntil: string; representatives: string
  // Что нарисовано на логотипе (по описанию ИИ).
  logoPicture: string
}
export type TrademarkImport = { summary: TrademarkSummary; excluded: number; badLogos: string[]; skipped: string[] }
export type TrademarkCheckRow = { texts: string[]; code: string }

export const trademarkLogoUrl = (regNo: string) => `/api/trademarks/logo?reg=${encodeURIComponent(regNo)}`

export const trademarksApi = {
  list: (query: string, unnamed: boolean, page: number) =>
    request<{ summary: TrademarkSummary; items: TrademarkItem[]; total: number }>(`/api/trademarks?q=${encodeURIComponent(query)}&unnamed=${unnamed ? 1 : 0}&page=${page}`),
  check: (rows: TrademarkCheckRow[]) =>
    request<{ results: TrademarkHit[][] }>('/api/trademarks/check', { method: 'POST', body: JSON.stringify({ rows }) }).then((body) => body.results),
  setLogoName: (regNo: string, name: string) =>
    request<void>('/api/trademarks/logo-name', { method: 'PATCH', body: JSON.stringify({ regNo, name }) }),
  async upload(file: File): Promise<TrademarkImport> {
    const form = new FormData()
    form.append('file', file)
    const response = await fetch('/api/trademarks/import', { method: 'POST', credentials: 'include', body: form })
    if (!response.ok) {
      const body = await response.json().catch(() => ({})) as { error?: { message?: string } }
      throw new Error(body.error?.message ?? 'Не удалось загрузить файл')
    }
    return response.json() as Promise<TrademarkImport>
  },
}

// Название знака для сотрудника: знак-изображение без подписи — «(изображение)».
export const trademarkName = (hit: Pick<TrademarkHit, 'name' | 'fromLogo'>) => hit.name ? `«${hit.name}»${hit.fromLogo ? ' (логотип)' : ''}` : '(изображение)'

// Правообладатель без адреса и реквизитов, до 60 символов.
export function shortHolder(holder: string) {
  const name = holder.replace(/[.,;]?\s*(юридический\s+|фактический\s+)?(адрес|регистрационный номер|бин[:\s]).*$/is, '').trim()
  return name.length > 60 ? `${name.slice(0, 59)}…` : name
}

// Одной строкой: что найдено, номер, правообладатель, срок и в каком слове товара.
export function trademarkText(hit: TrademarkHit) {
  const holder = shortHolder(hit.holder)
  const details = [`№ ${hit.regNo}`, holder, hit.protectedUntil ? `защита до ${formatRuDate(hit.protectedUntil)}` : ''].filter(Boolean).join(', ')
  const what = hit.kind === 'lookalike' ? `Похоже на знак ${trademarkName(hit)} — возможная имитация` : `Знак ${trademarkName(hit)}`
  return `ТРОИС: ${what} (${details}); в товаре — «${hit.found}»${goodsNote(hit) ? `. ${goodsNote(hit)}` : ''}`
}

// Совпадение группы товаров: «та же группа товаров (по коду)»; «в реестре — для других товаров»; "" — неизвестно.
export function goodsNote(hit: TrademarkHit) {
  if (hit.goodsMatch === 'code') return 'Та же группа товаров (по коду)'
  if (hit.goodsMatch === 'description') return 'Та же группа товаров (по описанию в реестре)'
  return hit.otherGoods ? 'В реестре знак для других товаров' : ''
}

// Описание товаров записи без классов и кодов: «Наушники; микрофоны» из «Наушники; микрофоны / 9 / 851800».
export const registryGoods = (goods: string) => goods.split('/')[0].trim()
