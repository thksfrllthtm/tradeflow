import { request } from '@/shared/api/client'
import { plural } from '@/shared/lib/plural'

// Перевод Word-документов (.docx) по сегментам: документы видят и правят все сотрудники.

// Языки перевода: новый язык — строка здесь и в backend (service/translator).
export const languages = [
  { code: 'zh', label: 'Китайский' },
  { code: 'ru', label: 'Русский' },
  { code: 'en', label: 'Английский' },
  { code: 'kk', label: 'Казахский' },
] as const

export const languageLabel = (code: string) => languages.find((item) => item.code === code)?.label ?? code

// Пределы размера файла: Word — 20 МБ, Excel с фото поставщика — 100 МБ (как в сметах).
export const maxFileMb = { docx: 20, xlsx: 100 }
export type DocumentFormat = keyof typeof maxFileMb

export type DocumentStatus = 'parsed' | 'translating' | 'translated' | 'failed'

export const documentStatusLabels: Record<DocumentStatus, string> = {
  parsed: 'Не переведён',
  translating: 'Переводится',
  translated: 'Переведён',
  failed: 'Ошибка',
}

export type TranslatorDocument = {
  id: number
  name: string
  format: DocumentFormat
  sourceLang: string
  targetLang: string
  status: DocumentStatus
  error: string
  // Что осталось без перевода: {"textboxes": 2, "headers": 1, …}.
  skipped: Record<string, number>
  createdBy: number | null
  createdByName: string
  segmentCount: number
  confirmedCount: number
  createdAt: string
  updatedAt: string
}

// Тег сегмента: фрагмент текста со своим форматированием или элемент Word без текста.
export type TagView = { kind: 'text' | 'tab' | 'br' | 'image' | 'field' | 'formula' | 'other'; bold?: boolean; italic?: boolean; underline?: boolean }

export type SegmentStatus = 'machine' | 'edited' | 'confirmed' | 'source_changed'

export type Segment = {
  id: number
  kind: 'paragraph' | 'table_cell'
  sourceText: string
  mtText: string
  targetText: string
  status: SegmentStatus
  locked: boolean
  qa: { code: string; message: string }[]
  tags: Record<string, TagView>
}

export const segmentStatusLabels: Record<SegmentStatus, string> = {
  machine: 'Машинный',
  edited: 'Исправлен',
  confirmed: 'Подтверждён',
  source_changed: 'Оригинал изменён',
}

export type SegmentVersion = { id: number; targetText: string; changedByName: string; changedAt: string }

// Пропущенное при разборе — в порядке показа: с числом или просто названием.
const skippedLabels: [string, string | [string, string, string]][] = [
  ['textboxes', ['надпись', 'надписи', 'надписей']],
  ['headers', 'колонтитулы'],
  ['footnotes', 'сноски'],
  ['formulas', ['формула', 'формулы', 'формул']],
  ['fields', ['поле формы', 'поля формы', 'полей формы']],
  ['charts', ['диаграмма', 'диаграммы', 'диаграмм']],
  ['sheets', ['название листа', 'названия листов', 'названий листов']],
  ['comments', 'примечания'],
  ['inline', 'текст, записанный прямо в ячейки'],
]

// «2 надписи, колонтитулы» — для предупреждения «Не переведено: …»; '' — всё разобрано.
export function skippedSummary(skipped: Record<string, number>) {
  return skippedLabels.filter(([kind]) => (skipped[kind] ?? 0) > 0).map(([kind, label]) => {
    const count = skipped[kind]
    return typeof label === 'string' ? label : `${count} ${plural(count, ...label)}`
  }).join(', ')
}

export const translatorApi = {
  list: (q: string) => request<{ items: TranslatorDocument[] }>(`/api/translator/documents?${new URLSearchParams(q ? { q } : {})}`).then((body) => body.items),
  // since — время прошлого ответа (now): вернутся только сегменты, изменённые после него.
  get: (id: number, since?: string) => request<{ document: TranslatorDocument; segments: Segment[]; now: string }>(`/api/translator/documents/${id}${since ? `?since=${encodeURIComponent(since)}` : ''}`),
  translate: (id: number) => request<void>(`/api/translator/documents/${id}/translate`, { method: 'POST' }),
  exportPath: (id: number, mode: 'target' | 'bilingual') => `/api/translator/documents/${id}/export?mode=${mode}`,
  // Правка перевода (статус → «исправлен») и/или статус: confirmed — подтвердить, edited — снять подтверждение.
  editSegment: (id: number, edit: { targetText?: string; status?: 'edited' | 'confirmed' }) =>
    request<Segment>(`/api/translator/segments/${id}`, { method: 'PATCH', body: JSON.stringify(edit) }),
  retranslate: (id: number) => request<Segment>(`/api/translator/segments/${id}/retranslate`, { method: 'POST' }),
  history: (id: number) => request<{ items: SegmentVersion[] }>(`/api/translator/segments/${id}/history`).then((body) => body.items),
  remove: (id: number) => request<void>(`/api/translator/documents/${id}`, { method: 'DELETE' }),
  // Файл уходит multipart-формой, поэтому мимо request() с его JSON-заголовком.
  async upload(file: File, sourceLang: string, targetLang: string): Promise<number> {
    const form = new FormData()
    form.append('file', file)
    form.append('sourceLang', sourceLang)
    form.append('targetLang', targetLang)
    const response = await fetch('/api/translator/documents', { method: 'POST', credentials: 'include', body: form })
    const body = await response.json().catch(() => ({})) as { id?: number; error?: { message?: string } }
    if (!response.ok || !body.id) throw new Error(body.error?.message ?? 'Не удалось загрузить документ')
    return body.id
  },
}
