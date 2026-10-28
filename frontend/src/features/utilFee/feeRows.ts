// Строки таблицы утильсбора и вставка из Excel. Суммы, расчётное нетто и упаковку считает сервер.

// МРП на 2026 год — значение по умолчанию; сотрудник может поменять.
export const DEFAULT_MRP = '4325'

export type FeeRow ={ code: string; grossKg: string; netKg: string }
export const FEE_COLUMNS = ['code', 'grossKg', 'netKg'] as const

export const emptyRow = (): FeeRow => ({ code: '', grossKg: '', netKg: '' })
const isEmpty = (row: FeeRow) => !row.code.trim() && !row.grossKg.trim() && !row.netKg.trim()

// Всегда одна пустая строка в конце — в неё печатают или вставляют следующие.
export function withTail(rows: FeeRow[]): FeeRow[] {
  let end = rows.length
  while (end > 0 && isEmpty(rows[end - 1])) end--
  return [...rows.slice(0, end), emptyRow()]
}

// Вставка из Excel с ячейки (row, column): строки — вниз, колонки — вправо (код → брутто → нетто);
// лишние колонки справа отбрасываются, недостающие строки добавляются.
export function pasteRows(rows: FeeRow[], row: number, column: number, text: string): FeeRow[] {
  const lines = text.replace(/\r\n?/g, '\n').split('\n')
  if (lines.length > 1 && lines[lines.length - 1] === '') lines.pop()
  const next = rows.map((item) => ({ ...item }))
  lines.forEach((line, offset) => {
    const target = next[row + offset] ?? (next[row + offset] = emptyRow())
    line.split('\t').forEach((cell, shift) => {
      const key = FEE_COLUMNS[column + shift]
      if (key) target[key] = cell.trim()
    })
  })
  return withTail(next)
}

// Текст из буфера — таблица (несколько ячеек), а не одно значение, которое браузер вставит сам.
export const isTable = (text: string) => /[\t\n]/.test(text.replace(/\r?\n$/, ''))
