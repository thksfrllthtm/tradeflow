import { excelPrice, imageDataUrl, mappedValue, rowId } from '../../shared/lib/excelColumns'
import { GlovesItemInput, GlovesParseRow } from '../../shared/types'
import { GlovesMapping } from '../types'

// Этап 1 сметы перчаток: какие колонки Excel бывают у поставщиков перчаток
// и как из выбранных колонок собираются строки сметы (без кода ТН ВЭД).
const mappingPatterns: Record<keyof GlovesMapping, string[]> = {
  name: ['Наименование', 'Название', 'Товар', '品名', '品类', '品种', '名称', 'name'],
  pairs: ['Пары', 'Количество пар', '总双数', '双数', 'pairs'],
  places: ['Места', 'Количество мест', '件数', '箱数', 'places', 'carton'],
  pairsPerPlace: ['Пар в месте', 'Пар/место', '双数/件', '双数/箱', 'pairs per place'],
  grossKg: ['Брутто', 'Вес брутто', '总毛重', '总重量', 'gross'],
  netKg: ['Нетто', 'Вес нетто', '净重', 'net'],
  pricePerPair: ['Цена за пару', 'Цена за единицу', 'Цена', '单价', 'unit price', 'price'],
}

export function emptyMapping(): GlovesMapping { return { name: '', pairs: '', places: '', pairsPerPlace: '', grossKg: '', netKg: '', pricePerPair: '' } }

export const mappingFieldOptions: Array<{ value: keyof GlovesMapping; label: string }> = [
  { value: 'name', label: 'Название поставщика' },
  { value: 'pairs', label: 'Количество пар' },
  { value: 'places', label: 'Количество мест' },
  { value: 'pairsPerPlace', label: 'Пар в одном месте' },
  { value: 'grossKg', label: 'Вес брутто, кг' },
  { value: 'netKg', label: 'Вес нетто, кг' },
  { value: 'pricePerPair', label: 'Цена за пару, USD' },
]

export function buildGloveItems(rows: GlovesParseRow[], images: { sourceRow: number; data: string; mimeType: string }[], mapping: GlovesMapping, packageWeightKg: string, offset: number): GlovesItemInput[] {
  return rows.filter((row) => row.included).map((row, index) => {
    const name = mappedValue(row.values, mapping.name, mappingPatterns.name)
    const rowImages = images.filter((candidate) => candidate.sourceRow === row.sourceRow)
    const pairs = mappedValue(row.values, mapping.pairs, mappingPatterns.pairs)
    const places = mappedValue(row.values, mapping.places, mappingPatterns.places)
    const pairsPerPlace = mappedValue(row.values, mapping.pairsPerPlace, mappingPatterns.pairsPerPlace)
    const grossKg = mappedValue(row.values, mapping.grossKg, mappingPatterns.grossKg)
    const netKg = mappedValue(row.values, mapping.netKg, mappingPatterns.netKg)
    const price = excelPrice(mappedValue(row.values, mapping.pricePerPair, mappingPatterns.pricePerPair))
    return {
      id: rowId(row.sourceRow, index + offset), sourceRow: row.sourceRow, name, supplierName: name, standardName: '', code: '',
      pairs, places, pairsPerPlace, grossKg, grossWeightUnit: grossKg ? 'kg' : '', netKg, netWeightUnit: netKg ? 'kg' : '', netSource: netKg ? 'excel' : '', packageWeightKg,
      coefficient: '', deliveryUsd: '', deliveryManual: false, deliveryIncluded: false, excluded: false, exclusionReason: '',
      deliveryAlreadyInCost: false,
      pricePerPair: price, sourcePricePerPair: price, costInput: '', costCurrency: '', extraExpenses: '', customsValueDirectKzt: '',
      photoUrl: imageDataUrl(rowImages[0]), photoUrls: rowImages.map(imageDataUrl), directoryEntryId: undefined,
    }
  })
}
