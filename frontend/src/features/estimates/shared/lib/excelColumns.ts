// Чтение значений из строк Excel по выбранным колонкам и работа с сопоставлением
// «поле строки → заголовок колонки». Набор полей у каждой сметы свой (Mapping).
type Mapping = Record<string, string>

export function rowId(sourceRow: number, index: number) {
  return `glove-${Date.now()}-${sourceRow}-${index}`
}

function normalizeHeader(value: string) {
  return value.toLowerCase().replace(/[\s_./\\()-]+/g, '')
}

function valueFor(values: Record<string, string>, patterns: string[]) {
  const entries = Object.entries(values)
  const exact = entries.find(([header]) => patterns.some((pattern) => normalizeHeader(header) === normalizeHeader(pattern)))
  if (exact) return exact[1]
  const match = entries.find(([header]) => patterns.some((pattern) => normalizeHeader(header).includes(normalizeHeader(pattern))))
  return match?.[1] ?? ''
}

// '__auto__' — устаревший режим автопоиска колонки по шаблонам названий.
export function mappedValue(values: Record<string, string>, selected: string, patterns: string[]) {
  if (selected === '__auto__') return valueFor(values, patterns)
  if (selected) return values[selected] ?? ''
  return ''
}

export function imageDataUrl(image?: { data: string; mimeType: string }) {
  return image ? `data:${image.mimeType};base64,${image.data}` : ''
}

export function mappedFieldForHeader<M extends Mapping>(header: string, mapping: M): keyof M | '' {
  const match = (Object.keys(mapping) as (keyof M)[]).find((field) => mapping[field] === header)
  return match ?? ''
}

// Сопоставление из сохранённого черновика: недостающие поля пустые, '__auto__' сбрасывается.
export function normalizeStoredMapping<M extends Mapping>(empty: M, value: Partial<M> | undefined): M {
  const result = { ...empty, ...(value ?? {}) } as M
  ;(Object.keys(result) as (keyof M)[]).forEach((field) => {
    if (result[field] === '__auto__') result[field] = '' as M[keyof M]
  })
  return result
}

// Цена за единицу из ячейки Excel в USD: «$1,25», «1.25 USD», «1,25» → «1.25». Другое содержимое
// (пусто, «¥12», текст) даёт пустую цену: такая строка считается как обычно, цену вводят на этапе результата.
export function excelPrice(value: string) {
  const compact = String(value ?? '').replace(/\s+/g, '').replace(/^\$|\$$/g, '').replace(/usd$/i, '').replace(',', '.')
  return /^\d+(\.\d+)?$/.test(compact) ? compact : ''
}
