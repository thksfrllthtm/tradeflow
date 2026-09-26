import { GlovesParseResult, GlovesParseRow } from '@/features/estimates/shared/types'
import { CeramicsProduct, CeramicSize, PriceResult, Resolution, ResolutionStatus, SizeUnit } from '../types'
import { normalizeMark } from './marks'
import { toDataUrl } from './photos'
import { baseName, formatSize } from './sizes'

// Колонки файла клиента. Фото — картинки, привязанные к строке Excel.
export const PHOTO_FROM_IMAGES = '__images__'
export type MatchMapping = { mark: string; chineseName: string; russianName: string; photo: string }

// Тип и размер нужны для подбора цены; в базу уходят только наименование, марка и цена.
export type Draft = { productType: string; sizeValue: string; sizeUnit: SizeUnit | ''; price: string; name: string }

// Одна марка файла: строки с одной маркой анализируются один раз.
export type MatchGroup = {
  key: string
  mark: string
  markKey: string
  chineseName: string
  russianName: string
  photo: string
  sourceRows: number[]
  state: 'pending' | 'running' | 'done' | 'error'
  error?: string
  resolution?: Resolution
  // Что видит и правит оператор; сохраняется в базу только по кнопке.
  draft: Draft
  // Цена для выбранных оператором типа и размера.
  draftPrice?: PriceResult
  saved: boolean
}

const patterns: Record<keyof Omit<MatchMapping, 'photo'>, RegExp> = {
  mark: /марк|артикул|型号|货号|款号|编号|代码|编码|item\s*no|model|^art|^code$|^sku$/i,
  chineseName: /中文|品名|名称|китай|chinese/i,
  russianName: /русск|наимен|назван|俄|russian/i,
}

const HAN = /\p{Script=Han}/u

// Колонки по содержимому, когда названия не подошли: китайское — где в большинстве ячеек есть
// иероглифы; марка — короткие коды с буквами или дефисом (6G, 721-7, A004-1), не количества.
function guessByContent(headers: string[], rows: GlovesParseRow[], used: Set<string>) {
  const sample = rows.filter((row) => row.included).slice(0, 50)
  const share = (header: string, test: (value: string) => boolean) => {
    const values = sample.map((row) => (row.values[header] ?? '').trim()).filter(Boolean)
    return values.length >= Math.min(3, sample.length) && values.length > 0 ? values.filter(test).length / values.length : 0
  }
  const best = (test: (value: string) => boolean) => {
    let result = ''
    let top = 0.6
    for (const header of headers) {
      if (used.has(header)) continue
      const value = share(header, test)
      if (value > top) { top = value; result = header }
    }
    if (result) used.add(result)
    return result
  }
  const chineseName = best((value) => HAN.test(value) && value.length >= 2)
  const mark = best((value) => value.length <= 24 && !HAN.test(value) && /\d/.test(value) && /[A-Za-z-]/.test(value) && /^[A-Za-z0-9][A-Za-z0-9\-./ ]*$/.test(value))
  return { mark, chineseName }
}

// Колонки по названиям заголовков, а если не нашлись — по содержимому; оператор может поменять.
export function guessMapping(headers: string[], hasImages: boolean, rows: GlovesParseRow[] = []): MatchMapping {
  const used = new Set<string>()
  const pick = (pattern: RegExp) => {
    const header = headers.find((value) => !used.has(value) && pattern.test(value)) ?? ''
    if (header) used.add(header)
    return header
  }
  let mark = pick(patterns.mark)
  let chineseName = pick(patterns.chineseName)
  const russianName = pick(patterns.russianName)
  if ((!mark || !chineseName) && rows.length) {
    const byContent = guessByContent(headers, rows, used)
    mark ||= byContent.mark
    chineseName ||= byContent.chineseName
  }
  return { mark, chineseName, russianName, photo: hasImages ? PHOTO_FROM_IMAGES : '' }
}

export function groupKey(mark: string, chineseName: string) {
  const markKey = normalizeMark(mark)
  return markKey || `zh:${chineseName.trim()}`
}

export function buildGroups(parsed: GlovesParseResult, mapping: MatchMapping): MatchGroup[] {
  const imageByRow = new Map<number, string>()
  if (mapping.photo === PHOTO_FROM_IMAGES) {
    for (const image of parsed.images) if (!imageByRow.has(image.sourceRow)) imageByRow.set(image.sourceRow, toDataUrl(image))
  }
  const groups = new Map<string, MatchGroup>()
  for (const row of parsed.rows) {
    if (!row.included) continue
    const mark = (mapping.mark ? row.values[mapping.mark] ?? '' : '').trim()
    const chineseName = (mapping.chineseName ? row.values[mapping.chineseName] ?? '' : '').trim()
    const russianName = (mapping.russianName ? row.values[mapping.russianName] ?? '' : '').trim()
    if (!mark && !chineseName) continue
    const key = groupKey(mark, chineseName)
    const existing = groups.get(key)
    if (existing) {
      existing.sourceRows.push(row.sourceRow)
      existing.chineseName ||= chineseName
      existing.russianName ||= russianName
      existing.photo ||= imageByRow.get(row.sourceRow) ?? ''
      continue
    }
    groups.set(key, {
      key, mark, markKey: normalizeMark(mark), chineseName, russianName, photo: imageByRow.get(row.sourceRow) ?? '',
      sourceRows: [row.sourceRow], state: 'pending', draft: { productType: '', sizeValue: '', sizeUnit: '', price: '', name: '' }, saved: false,
    })
  }
  return [...groups.values()]
}

// chineseName — китайское название из файла клиента: оно становится частью наименования новой марки.
export function draftFromResolution(resolution: Resolution, chineseName: string): Draft {
  if (resolution.product) {
    const product = resolution.product
    return { productType: product.productType, sizeValue: product.sizeValue, sizeUnit: product.sizeUnit, price: product.price, name: product.name }
  }
  return {
    productType: resolution.productType,
    sizeValue: resolution.size?.value ?? '',
    sizeUnit: resolution.size?.unit ?? '',
    price: resolution.price.status === 'MATCHED' ? resolution.price.price ?? '' : '',
    name: resolution.displayName ? baseName(resolution.displayName, chineseName) : '',
  }
}

// Результат сохранённой оператором записи: дальше марка находится в базе.
export function savedResolution(group: MatchGroup, product: CeramicsProduct): Resolution {
  const size: CeramicSize | null = product.sizeValue && product.sizeUnit ? { value: product.sizeValue, unit: product.sizeUnit } : null
  return {
    key: group.key, mark: product.mark, markKey: normalizeMark(product.mark), status: 'FOUND_BY_MARK', product,
    productType: product.productType, typeStatus: 'MATCHED', typeCandidates: [], size, sizeStatus: size ? 'MATCHED' : 'NOT_NEEDED', sizeOptions: [],
    price: { status: 'MATCHED', price: product.price, options: [], total: 1, availableSizes: [], hasUnsized: false, message: '', name: product.name },
    displayName: product.name, chineseTerms: group.resolution?.chineseTerms ?? [],
    // Решение оператора — первым: по нему строится колонка «Источник»; подбор остаётся ниже для истории.
    evidence: [
      { field: 'type', source: 'manual', text: `Сохранено в базу оператором: «», цена ` },
      ...(size ? [{ field: 'size' as const, source: 'manual', text: 'Размер подтверждён оператором' }] : []),
      ...(group.resolution?.evidence ?? []).filter((item) => item.source !== 'mark_exact'),
    ],
    warnings: [], aiUsed: group.resolution?.aiUsed ?? false,
  }
}

export function draftSize(draft: Draft): CeramicSize | null {
  return draft.sizeValue && draft.sizeUnit ? { value: draft.sizeValue, unit: draft.sizeUnit } : null
}

export const statusLabels: Record<ResolutionStatus, string> = {
  FOUND_BY_MARK: '✅ Найдено по марке',
  AUTO: '🤖 Определено автоматически',
  NEED_REVIEW: '⚠ Требует проверки',
  NOT_DETERMINED: '❌ Не определено',
}

export function groupStatusLabel(group: MatchGroup) {
  if (group.state === 'pending') return 'Ожидает'
  if (group.state === 'running') return 'Анализ…'
  if (group.state === 'error' || !group.resolution) return `❌ Ошибка${group.error ? `: ${group.error}` : ''}`
  return statusLabels[group.resolution.status]
}

export function chunk<T>(items: T[], size: number): T[][] {
  const result: T[][] = []
  for (let index = 0; index < items.length; index += size) result.push(items.slice(index, index + size))
  return result
}

function csvCell(value: string | number) {
  const text = String(value ?? '')
  return /[";\n\r]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text
}

// Таблица по строкам исходного файла (одна марка в нескольких строках повторяется).
// «;» и BOM — чтобы русский Excel открыл файл без настройки.
export function resultCsv(groups: MatchGroup[]) {
  const header = ['Строка Excel', 'Марка', 'Китайское наименование', 'Наименование', 'Цена', 'Тип товара', 'Размер', 'Статус']
  const rows = groups
    .flatMap((group) => group.sourceRows.map((sourceRow) => ({ sourceRow, group })))
    .sort((a, b) => a.sourceRow - b.sourceRow)
    .map(({ sourceRow, group }) => {
      const size = draftSize(group.draft)
      return [sourceRow, group.mark, group.chineseName, group.draft.name, group.draft.price, group.draft.productType, formatSize(size), groupStatusLabel(group)]
    })
  return '﻿' + [header, ...rows].map((row) => row.map(csvCell).join(';')).join('\r\n')
}

// Строка заголовков, если в первой строке файла не названия колонок, а, например, номер
// заказа («P70-3817606-3474件»): первая из строк файла, где есть ячейка «型号», «Марка», «Артикул»…
// null — такой строки нет.
export function findHeaderRow(parsed: GlovesParseResult): number | null {
  for (const row of parsed.rows.slice(0, 15)) {
    const cells = Object.values(row.values).map((value) => value.trim()).filter(Boolean)
    if (cells.some((value) => value.length <= 30 && patterns.mark.test(value)) && cells.length >= 2) return row.sourceRow
  }
  return null
}
