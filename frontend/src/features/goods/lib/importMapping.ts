import { GoodsItemInput, GoodsParseRow } from '@/shared/types/goods'
import { GoodsMapping } from '../types'
import { excelPrice, imageDataUrl, mappedValue, rowId } from './excelColumns'
import { lookupNomenclature, normalizeTnvedCode, withNomenclatureRates } from './nomenclatureRates'

// Этап 1: сопоставление колонок Excel с полями строки и сборка строк сметы.
const mappingPatterns: Record<keyof GoodsMapping, string[]> = {
  name: ['Наименование', 'Название', 'Товар', '品名', '品类', '品种', '名称', 'name'],
  name2: [],
  name3: [],
  code: ['Код ТН ВЭД', 'Код ТНВЭД', 'ТН ВЭД', 'Код', '海关编码', '税号', 'HS Code', 'HS код', 'tnved', 'hs code'],
  pairs: ['Количество товара', 'Количество', 'Пары', 'Количество пар', '数量', '总双数', '双数', 'quantity', 'pieces', 'pairs'],
  places: ['Места', 'Количество мест', '件数', '箱数', 'places', 'carton'],
  pairsPerPlace: ['Количество товара в одном месте', 'Количество в месте', 'Пар в месте', 'Пар/место', '数量/件', '双数/件', '双数/箱', 'quantity per place', 'pieces per place', 'pairs per place'],
  grossKg: ['Брутто', 'Вес брутто', '总毛重', '总重量', 'gross'],
  netKg: ['Нетто', 'Вес нетто', '净重', 'net'],
  pricePerPair: ['Цена за пару', 'Цена за единицу', 'Цена', '单价', 'unit price', 'price'],
}

export function emptyMapping(): GoodsMapping { return { name: '', name2: '', name3: '', code: '', pairs: '', places: '', pairsPerPlace: '', grossKg: '', netKg: '', pricePerPair: '' } }

export const mappingFieldOptions: Array<{ value: keyof GoodsMapping; label: string }> = [
  { value: 'name', label: 'Наименование' },
  { value: 'name2', label: 'Наименование 2' },
  { value: 'name3', label: 'Наименование 3' },
  { value: 'code', label: 'Код ТН ВЭД' },
  { value: 'pairs', label: 'Количество товара' },
  { value: 'places', label: 'Количество мест' },
  { value: 'pairsPerPlace', label: 'Количество товара в одном месте' },
  { value: 'grossKg', label: 'Вес брутто, кг' },
  { value: 'netKg', label: 'Вес нетто, кг' }, 
  { value: 'pricePerPair', label: 'Цена за единицу (пару), USD' },
]

export function buildGloveItems(rows: GoodsParseRow[], images: { sourceRow: number; data: string; mimeType: string }[], mapping: GoodsMapping, packageWeightKg: string, offset: number): GoodsItemInput[] {
  return rows.filter((row) => row.included).map((row, index) => {
    // Несколько колонок наименования: первое непустое — наименование строки, остальные (если
    // отличаются) — дополнительные: по ним ИИ подбирает код и ищется тот же товар в базе.
    const names = [...new Set([mapping.name, mapping.name2, mapping.name3].filter(Boolean).map((header) => (row.values[header] ?? '').trim()).filter(Boolean))]
    const name = names[0] ?? mappedValue(row.values, mapping.name, mappingPatterns.name)
    const otherNames = names.slice(1)
    const code = mappedValue(row.values, mapping.code, mappingPatterns.code)
    const rowImages = images.filter((candidate) => candidate.sourceRow === row.sourceRow)
    const pairs = mappedValue(row.values, mapping.pairs, mappingPatterns.pairs)
    const places = mappedValue(row.values, mapping.places, mappingPatterns.places)
    const pairsPerPlace = mappedValue(row.values, mapping.pairsPerPlace, mappingPatterns.pairsPerPlace)
    const grossKg = mappedValue(row.values, mapping.grossKg, mappingPatterns.grossKg)
    const netKg = mappedValue(row.values, mapping.netKg, mappingPatterns.netKg)
    const price = excelPrice(mappedValue(row.values, mapping.pricePerPair, mappingPatterns.pricePerPair))
    return {
      id: rowId(row.sourceRow, index + offset), sourceRow: row.sourceRow, name, supplierName: name, ...(otherNames.length ? { otherNames } : {}), standardName: '', code,
      pairs, places, pairsPerPlace, grossKg, grossWeightUnit: grossKg ? 'kg' : '', netKg, netWeightUnit: netKg ? 'kg' : '', netSource: netKg ? 'excel' : '', packageWeightKg,
      coefficient: '', deliveryUsd: '', deliveryManual: false, deliveryIncluded: false, excluded: false, exclusionReason: '',
      deliveryAlreadyInCost: false,
      pricePerPair: price, sourcePricePerPair: price, costInput: '', costCurrency: '', extraExpenses: '', customsValueDirectKzt: '',
      photoUrl: imageDataUrl(rowImages[0]), photoUrls: rowImages.map(imageDataUrl), directoryEntryId: undefined,
    }
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
    return { ...item, code, standardName: String(snapshot.name), directoryEntryId: undefined, productDirectoryId: undefined, directorySnapshot: snapshot }
  })
  return { items, matched, missing, nomenclatureLoaded: true }
}
