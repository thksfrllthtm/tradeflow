import { NomenclatureCode } from '../nomenclature/nomenclatureApi'
import { TNVEDDirectoryEntry } from './types'

// Проверка списка кодов, вставленного из колонки Excel. На странице справочника — какие коды
// уже есть в справочнике, каких нет и какие лежат только в архиве; на странице номенклатуры —
// какие коды есть в номенклатуре ТН ВЭД ЕАЭС, а каких нет (скорее всего, ошибка или устаревший код).

export type PastedCode = { code: string; count: number; padded: boolean }
export type PastedCodes = { codes: PastedCode[]; invalid: string[]; rows: number }

// Справочник: missing / archived / found. Номенклатура: unknown (нет в ТН ВЭД ЕАЭС) / official.
export type CodeCheckStatus = 'unknown' | 'missing' | 'archived' | 'found' | 'official'
export type CodeCheckRow = PastedCode & { status: CodeCheckStatus; entries: TNVEDDirectoryEntry[]; nomenclature: NomenclatureCode | null }

// Колонка из Excel приходит строками через перевод строки, несколько колонок — через табуляцию.
// В ячейке допускаются пробелы и точки внутри кода («4015 19 000 0»). Если Excel хранил код
// числом, ведущий ноль теряется: 9 цифр дополняем нулём слева и помечаем.
export function parsePastedCodes(text: string): PastedCodes {
  const byCode = new Map<string, PastedCode>()
  const invalid: string[] = []
  let rows = 0
  for (const cell of text.split(/[\r\n\t;,]+/)) {
    const value = cell.trim()
    // Ячейки без цифр — заголовок колонки или подпись, их пропускаем молча.
    if (!/\d/.test(value)) continue
    rows++
    const digits = value.replace(/[\s. -]/g, '')
    const code = /^\d{9}$/.test(digits) ? `0${digits}` : digits
    if (!/^\d{10}$/.test(code)) {
      if (!invalid.includes(value)) invalid.push(value)
      continue
    }
    const existing = byCode.get(code)
    if (existing) existing.count++
    else byCode.set(code, { code, count: 1, padded: code !== digits })
  }
  return { codes: [...byCode.values()], invalid, rows }
}

const statusOrder: Record<CodeCheckStatus, number> = { unknown: 0, missing: 1, archived: 2, found: 3, official: 4 }
const byStatus = (a: CodeCheckRow, b: CodeCheckRow) => statusOrder[a.status] - statusOrder[b.status]

// Справочник: недостающие — наверху, внутри группы сохраняется порядок из Excel. Статус — только
// по справочнику; nomenclature — найденные в номенклатуре коды, из них «Добавить» заполняет карточку.
export function checkCodes(codes: PastedCode[], entries: TNVEDDirectoryEntry[], nomenclature: NomenclatureCode[] = []): CodeCheckRow[] {
  const byCode = new Map<string, TNVEDDirectoryEntry[]>()
  for (const entry of entries) byCode.set(entry.code, [...(byCode.get(entry.code) ?? []), entry])
  const inNomenclature = new Map(nomenclature.map((item) => [item.code, item]))
  return codes.map((item) => {
    const matches = byCode.get(item.code) ?? []
    const active = matches.filter((entry) => !entry.archivedAt)
    const status: CodeCheckStatus = active.length > 0 ? 'found' : matches.length > 0 ? 'archived' : 'missing'
    return { ...item, status, entries: active.length > 0 ? active : matches, nomenclature: inNomenclature.get(item.code) ?? null }
  }).sort(byStatus)
}

// Номенклатура: коды, которых нет в ТН ВЭД ЕАЭС, — наверху.
export function checkNomenclature(codes: PastedCode[], found: NomenclatureCode[]): CodeCheckRow[] {
  const byCode = new Map(found.map((item) => [item.code, item]))
  return codes.map((item): CodeCheckRow => {
    const official = byCode.get(item.code) ?? null
    return { ...item, status: official ? 'official' : 'unknown', entries: [], nomenclature: official }
  }).sort(byStatus)
}
