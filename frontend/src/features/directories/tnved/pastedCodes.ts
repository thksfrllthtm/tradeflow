import { TNVEDDirectoryEntry } from './types'

// Проверка списка кодов, вставленного из колонки Excel: какие коды уже есть в справочнике,
// каких нет и какие лежат только в архиве — чтобы добавить недостающие до начала расчёта.

export type PastedCode = { code: string; count: number; padded: boolean }
export type PastedCodes = { codes: PastedCode[]; invalid: string[]; rows: number }

export type CodeCheckStatus = 'missing' | 'archived' | 'found'
export type CodeCheckRow = PastedCode & { status: CodeCheckStatus; entries: TNVEDDirectoryEntry[] }

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
    const digits = value.replace(/[\s. -]/g, '')
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

const statusOrder: Record<CodeCheckStatus, number> = { missing: 0, archived: 1, found: 2 }

// Недостающие — наверху, внутри группы сохраняется порядок из Excel.
export function checkCodes(codes: PastedCode[], entries: TNVEDDirectoryEntry[]): CodeCheckRow[] {
  const byCode = new Map<string, TNVEDDirectoryEntry[]>()
  for (const entry of entries) byCode.set(entry.code, [...(byCode.get(entry.code) ?? []), entry])
  return codes.map((item) => {
    const matches = byCode.get(item.code) ?? []
    const active = matches.filter((entry) => !entry.archivedAt)
    const status: CodeCheckStatus = active.length > 0 ? 'found' : matches.length > 0 ? 'archived' : 'missing'
    return { ...item, status, entries: active.length > 0 ? active : matches }
  }).sort((a, b) => statusOrder[a.status] - statusOrder[b.status])
}
