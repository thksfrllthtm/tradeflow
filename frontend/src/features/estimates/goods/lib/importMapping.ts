import { tnvedDirectoryApi } from '@/features/directories/tnved/tnvedDirectoryApi'
import { GlovesItemInput, GlovesParseRow } from '../../shared/types'
import { TNVEDDirectoryEntry } from '@/features/directories/tnved/types'
import { GlovesMapping } from '../types'
import { excelPrice, imageDataUrl, mappedValue, rowId } from '../../shared/lib/excelColumns'

// Этап 1: сопоставление колонок Excel с полями строки и сборка строк сметы.
const mappingPatterns: Record<keyof GlovesMapping, string[]> = {
  name: ['Наименование', 'Название', 'Товар', '品名', '品类', '品种', '名称', 'name'],
  code: ['Код ТН ВЭД', 'Код ТНВЭД', 'ТН ВЭД', 'Код', '海关编码', '税号', 'HS Code', 'HS код', 'tnved', 'hs code'],
  pairs: ['Количество товара', 'Количество', 'Пары', 'Количество пар', '数量', '总双数', '双数', 'quantity', 'pieces', 'pairs'],
  places: ['Места', 'Количество мест', '件数', '箱数', 'places', 'carton'],
  pairsPerPlace: ['Количество товара в одном месте', 'Количество в месте', 'Пар в месте', 'Пар/место', '数量/件', '双数/件', '双数/箱', 'quantity per place', 'pieces per place', 'pairs per place'],
  grossKg: ['Брутто', 'Вес брутто', '总毛重', '总重量', 'gross'],
  netKg: ['Нетто', 'Вес нетто', '净重', 'net'],
  pricePerPair: ['Цена за пару', 'Цена за единицу', 'Цена', '单价', 'unit price', 'price'],
}

export function emptyMapping(): GlovesMapping { return { name: '', code: '', pairs: '', places: '', pairsPerPlace: '', grossKg: '', netKg: '', pricePerPair: '' } }

export const mappingFieldOptions: Array<{ value: keyof GlovesMapping; label: string }> = [
  { value: 'name', label: 'Наименование' },
  { value: 'code', label: 'Код ТН ВЭД' },
  { value: 'pairs', label: 'Количество товара' },
  { value: 'places', label: 'Количество мест' },
  { value: 'pairsPerPlace', label: 'Количество товара в одном месте' },
  { value: 'grossKg', label: 'Вес брутто, кг' },
  { value: 'netKg', label: 'Вес нетто, кг' }, 
  { value: 'pricePerPair', label: 'Цена за единицу (пару), USD' },
]

export function buildGloveItems(rows: GlovesParseRow[], images: { sourceRow: number; data: string; mimeType: string }[], mapping: GlovesMapping, packageWeightKg: string, offset: number): GlovesItemInput[] {
  return rows.filter((row) => row.included).map((row, index) => {
    const name = mappedValue(row.values, mapping.name, mappingPatterns.name)
    const code = mappedValue(row.values, mapping.code, mappingPatterns.code)
    const rowImages = images.filter((candidate) => candidate.sourceRow === row.sourceRow)
    const pairs = mappedValue(row.values, mapping.pairs, mappingPatterns.pairs)
    const places = mappedValue(row.values, mapping.places, mappingPatterns.places)
    const pairsPerPlace = mappedValue(row.values, mapping.pairsPerPlace, mappingPatterns.pairsPerPlace)
    const grossKg = mappedValue(row.values, mapping.grossKg, mappingPatterns.grossKg)
    const netKg = mappedValue(row.values, mapping.netKg, mappingPatterns.netKg)
    const price = excelPrice(mappedValue(row.values, mapping.pricePerPair, mappingPatterns.pricePerPair))
    return {
      id: rowId(row.sourceRow, index + offset), sourceRow: row.sourceRow, name, supplierName: name, standardName: '', code,
      pairs, places, pairsPerPlace, grossKg, grossWeightUnit: grossKg ? 'kg' : '', netKg, netWeightUnit: netKg ? 'kg' : '', netSource: netKg ? 'excel' : '', packageWeightKg,
      coefficient: '', deliveryUsd: '', deliveryManual: false, deliveryIncluded: false, excluded: false, exclusionReason: '',
      deliveryAlreadyInCost: false,
      pricePerPair: price, sourcePricePerPair: price, costInput: '', costCurrency: '', extraExpenses: '', customsValueDirectKzt: '',
      photoUrl: imageDataUrl(rowImages[0]), photoUrls: rowImages.map(imageDataUrl), directoryEntryId: undefined,
    }
  })
}

function normalizeTnvedCode(value: string): string {
  const compact = String(value ?? '').trim().replace(/^['`]/, '').replace(/\s+/g, '')
  if (/^\d{10}$/.test(compact)) return compact
  const decimalCode = compact.match(/^(\d{10})\.0+$/)
  return decimalCode?.[1] ?? ''
}

function tnvedDisplayName(entry: TNVEDDirectoryEntry): string {
  return [entry.name, entry.nameZh].map((value) => value?.trim()).filter(Boolean).join(' / ')
}

export async function matchImportedTnvedCodes(input: GlovesItemInput[]): Promise<{ items: GlovesItemInput[]; matched: number; missing: number }> {
  const rowsWithCodes = input.filter((item) => normalizeTnvedCode(item.code) !== '')
  if (rowsWithCodes.length === 0) return { items: input, matched: 0, missing: 0 }

  const entries = await tnvedDirectoryApi.list()
  const byCode = new Map<string, TNVEDDirectoryEntry>()
  entries.filter((entry) => !entry.archivedAt).forEach((entry) => {
    const code = normalizeTnvedCode(entry.code)
    if (code && !byCode.has(code)) byCode.set(code, entry)
  })

  let matched = 0
  let missing = 0
  const items = input.map((item) => {
    const code = normalizeTnvedCode(item.code)
    if (!code) return item
    const entry = byCode.get(code)
    if (!entry) {
      missing += 1
      return { ...item, code }
    }
    matched += 1
    const displayName = tnvedDisplayName(entry)
    const snapshot = {
      entryId: entry.id,
      category: entry.category,
      name: entry.name,
      nameZh: entry.nameZh,
      code: entry.code,
      antiDumpingPercent: entry.antiDumpingPercent,
      duty: entry.duty,
      vat: entry.vat,
      verificationStatus: entry.verificationStatus,
      sourceKind: 'operator',
    }
    return {
      ...item,
      code: entry.code,
      standardName: displayName,
      directoryEntryId: entry.id,
      productDirectoryId: undefined,
      directorySnapshot: snapshot,
    }
  })
  return { items, matched, missing }
}
