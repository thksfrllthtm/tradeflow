import { GoodsItemInput, GoodsParseRow } from '@/shared/types/goods'
import { GoodsMapping } from '../types'
import { excelPrice, imageDataUrl, mappedValue, rowId } from './excelColumns'
import { lookupNomenclature, normalizeTnvedCode, withNomenclatureRates } from './nomenclatureRates'

// Этап 1: сопоставление колонок Excel с полями строки и сборка строк сметы.
export function emptyMapping(): GoodsMapping { return { name: '', data1: '', data2: '', data3: '', data4: '', data5: '', code: '', pairs: '', places: '', pairsPerPlace: '', grossKg: '', netKg: '', pricePerPair: '' } }

export const mappingFieldOptions: Array<{ value: keyof GoodsMapping; label: string }> = [
  { value: 'name', label: 'Наименование' },
  { value: 'code', label: 'Код ТН ВЭД' },
  { value: 'pairs', label: 'Количество товара' },
  { value: 'places', label: 'Количество мест' },
  { value: 'pairsPerPlace', label: 'Количество товара в одном месте' },
  { value: 'grossKg', label: 'Вес брутто, кг' },
  { value: 'netKg', label: 'Вес нетто, кг' },
  { value: 'pricePerPair', label: 'Цена за единицу (пару), USD' },
  { value: 'data1', label: 'Данные 1' },
  { value: 'data2', label: 'Данные 2' },
  { value: 'data3', label: 'Данные 3' },
  { value: 'data4', label: 'Данные 4' },
  { value: 'data5', label: 'Данные 5' },
]

const DATA_FIELDS = ['data1', 'data2', 'data3', 'data4', 'data5'] as const

// «Данные» строки: значения колонок data1…data5 с подписью из заголовка колонки («Материал: сталь»); у колонки
// без заголовка («Колонка 5») — только значение. Пустые ячейки пропускаются.
function rowDetails(values: Record<string, string>, mapping: GoodsMapping): string[] {
  return DATA_FIELDS.flatMap((field) => {
    const header = mapping[field]
    const value = header ? (values[header] ?? '').trim() : ''
    if (!value) return []
    return [/^Колонка \d+$/.test(header) ? value : `${header}: ${value}`]
  })
}

// Сохранённое раньше сопоставление: «Наименование 2/3» (name2, name3) стали «Данные 1/2».
export function upgradeLegacyMapping(mapping: GoodsMapping & { name2?: string; name3?: string }): GoodsMapping {
  const { name2, name3, ...rest } = mapping
  return { ...rest, data1: rest.data1 || name2 || '', data2: rest.data2 || name3 || '' }
}

export function buildGloveItems(rows: GoodsParseRow[], images: { sourceRow: number; data: string; mimeType: string }[], mapping: GoodsMapping, packageWeightKg: string, offset: number): GoodsItemInput[] {
  const included = rows.filter((row) => row.included)
  const items = included.map((row, index): GoodsItemInput => {
    const name = mappedValue(row.values, mapping.name)
    const details = rowDetails(row.values, mapping)
    const code = mappedValue(row.values, mapping.code)
    const rowImages = images.filter((candidate) => candidate.sourceRow === row.sourceRow)
    const pairs = mappedValue(row.values, mapping.pairs)
    const places = mappedValue(row.values, mapping.places)
    const pairsPerPlace = mappedValue(row.values, mapping.pairsPerPlace)
    const grossKg = mappedValue(row.values, mapping.grossKg)
    const netKg = mappedValue(row.values, mapping.netKg)
    const price = excelPrice(mappedValue(row.values, mapping.pricePerPair))
    return {
      id: rowId(row.sourceRow, index + offset), sourceRow: row.sourceRow, name, supplierName: name, ...(details.length ? { details } : {}), standardName: '', code,
      pairs, places, pairsPerPlace, grossKg, grossWeightUnit: grossKg ? 'kg' : '', netKg, netWeightUnit: netKg ? 'kg' : '', netSource: netKg ? 'excel' : '', packageWeightKg,
      coefficient: '', deliveryUsd: '', deliveryManual: false, deliveryIncluded: false, excluded: false,
      deliveryAlreadyInCost: false,
      pricePerPair: price, sourcePricePerPair: price, costInput: '', costCurrency: '', extraExpenses: '', customsValueDirectKzt: '',
      photoUrl: imageDataUrl(rowImages[0]), photoUrls: rowImages.map(imageDataUrl),
    }
  })
  return foldSetRows(included, items, mapping)
}

// Набор: количество или цена объединены в Excel на несколько строк («E4:E6») — это один товар.
// Строки набора склеиваются в первую: наименования через «+», фото — все; количество, вес и цена —
// из объединённых ячеек первой строки.
function foldSetRows(rows: GoodsParseRow[], items: GoodsItemInput[], mapping: GoodsMapping) {
  const bySource = new Map(items.map((item) => [item.sourceRow, item]))
  return items.filter((item, index) => {
    const merged = rows[index].merged
    const first = merged?.[mapping.pairs] ?? merged?.[mapping.pricePerPair]
    const parent = first ? bySource.get(first) : undefined
    if (!parent || parent === item) return true
    const name = [...new Set([parent.name, item.name].filter(Boolean))].join(' + ')
    parent.name = name
    parent.supplierName = name
    parent.photoUrls = [...new Set([...(parent.photoUrls ?? []), ...(item.photoUrls ?? [])])]
    parent.photoUrl ||= item.photoUrl
    return false
  })
}

// Коды из файла сверяются с номенклатурой ТН ВЭД ЕАЭС: найденный код получает наименование и
// ставки из номенклатуры (пошлина ЕТТ и НДС), справочник ТН ВЭД для ставок не используется.
// Номенклатура не загружена (nomenclatureLoaded = false) — коды остаются как есть.
export async function matchImportedTnvedCodes(input: GoodsItemInput[]): Promise<{ items: GoodsItemInput[]; matched: number; missing: number; nomenclatureLoaded: boolean }> {
  const codes = input.map((item) => normalizeTnvedCode(item.code)).filter(Boolean)
  if (codes.length === 0) return { items: input, matched: 0, missing: 0, nomenclatureLoaded: true }
  const found = await lookupNomenclature(codes)
  if (!found) return { items: input.map((item) => ({ ...item, code: normalizeTnvedCode(item.code) || item.code })), matched: 0, missing: codes.length, nomenclatureLoaded: false }

  let matched = 0
  let missing = 0
  const items = input.map((item) => {
    const code = normalizeTnvedCode(item.code)
    if (!code) return item
    const official = found.byCode.get(code)
    if (!official) {
      missing += 1
      return { ...item, code }
    }
    matched += 1
    const snapshot = withNomenclatureRates(official, found.source)
    return { ...item, code, standardName: String(snapshot.name), productDirectoryId: undefined, directorySnapshot: snapshot }
  })
  return { items, matched, missing, nomenclatureLoaded: true }
}
