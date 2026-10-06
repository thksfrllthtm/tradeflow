import { NomenclatureCode } from '@/shared/tnved/nomenclature'

// Проверка списка кодов, вставленного из колонки Excel: какие коды есть в номенклатуре ТН ВЭД ЕАЭС,
// а каких нет (скорее всего, ошибка или устаревший код).

export type PastedCode = { code: string; count: number; padded: boolean }
export type PastedCodes = { codes: PastedCode[]; invalid: string[]; rows: number }

// unknown — кода нет в ТН ВЭД ЕАЭС, official — есть.
export type CodeCheckStatus = 'unknown' | 'official'
export type CodeCheckRow = PastedCode & { status: CodeCheckStatus; nomenclature: NomenclatureCode | null }

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

const statusOrder: Record<CodeCheckStatus, number> = { unknown: 0, official: 1 }
const byStatus = (a: CodeCheckRow, b: CodeCheckRow) => statusOrder[a.status] - statusOrder[b.status]

// Номенклатура: коды, которых нет в ТН ВЭД ЕАЭС, — наверху.
export function checkNomenclature(codes: PastedCode[], found: NomenclatureCode[]): CodeCheckRow[] {
  const byCode = new Map(found.map((item) => [item.code, item]))
  return codes.map((item): CodeCheckRow => {
    const official = byCode.get(item.code) ?? null
    return { ...item, status: official ? 'official' : 'unknown', nomenclature: official }
  }).sort(byStatus)
}
