import type jspreadsheet from 'jspreadsheet-ce'
import { formatWeight } from '../../lib/netWeight'
import { copyValue, NUMBER_FIELDS, SHEET_COLUMNS } from './sheetModel'
import { ru } from './sheetCells'
import type { Worksheet } from './sheetSync'
import type { ReviewSheetProps } from './ReviewSheet'

// ── Буфер обмена, строка состояния, контекстное меню ──

function selectedRange(ws: Worksheet, fallbackRow: number) {
  const cell = ws.selectedCell?.map(Number) ?? [0, fallbackRow, 0, fallbackRow]
  return { left: Math.min(cell[0], cell[2]), top: Math.min(cell[1], cell[3]), right: Math.max(cell[0], cell[2]), bottom: Math.max(cell[1], cell[3]) }
}

const shown = (ws: Worksheet, y: number) => ws.rows[y]?.element.style.display !== 'none'

// Копирование для Excel: без скрытых фильтром строк, числа с запятой, без служебных колонок.
export function copiedText(ws: Worksheet, [x1, y1, x2, y2]: [number, number, number, number]) {
  const data = ws.options.data as unknown[][]
  const lines: string[] = []
  for (let y = y1; y <= y2; y++) {
    if (!shown(ws, y)) continue
    const cells: string[] = []
    for (let x = x1; x <= x2; x++) cells.push(copyValue(SHEET_COLUMNS[x].key, data[y]?.[x]))
    lines.push(cells.join('\t'))
  }
  return lines.join('\r\n')
}

// Как строка состояния Excel: сумма и количество чисел в выделении.
export function selectionSummary(ws: Worksheet, x1: number, y1: number, x2: number, y2: number) {
  const data = ws.options.data as unknown[][]
  let sum = 0
  let count = 0
  for (let y = Math.min(y1, y2); y <= Math.max(y1, y2); y++) {
    if (!shown(ws, y)) continue
    for (let x = Math.min(x1, x2); x <= Math.max(x1, x2); x++) {
      if (!(NUMBER_FIELDS as string[]).includes(SHEET_COLUMNS[x]?.key)) continue
      const text = String(data[y]?.[x] ?? '').trim()
      if (text === '' || !Number.isFinite(Number(text))) continue
      sum += Number(text)
      count++
    }
  }
  return count > 1 ? `Количество: ${count} · Сумма: ${ru(formatWeight(sum))}` : ''
}

export function contextMenu(ws: Worksheet, clickedRow: number, props: ReviewSheetProps): jspreadsheet.ContextMenuItem[] {
  const { left, top, bottom } = selectedRange(ws, clickedRow)
  const rows: number[] = []
  for (let y = top; y <= bottom; y++) if (y >= 0 && shown(ws, y)) rows.push(y)
  const line = { type: 'line' as const, title: '' }
  return [
    { title: 'Вставить строку выше', onclick: () => props.onInsertRow(Math.max(top, 0)) },
    { title: 'Вставить строку ниже', onclick: () => props.onInsertRow(bottom + 1) },
    { title: rows.length > 1 ? `Удалить строки (${rows.length})` : 'Удалить строку', disabled: rows.length === 0, onclick: () => props.onDeleteRows(rows) },
    line,
    { title: rows.length > 1 ? `Товар или код для ${rows.length} строк…` : 'Выбрать товар или код…', disabled: rows.length === 0, onclick: () => props.onPickRows(rows) },
    line,
    { title: 'Копировать', shortcut: 'Ctrl+C', onclick: () => ws.copy() },
    { title: 'Вставить', shortcut: 'Ctrl+V', disabled: !navigator.clipboard?.readText, onclick: () => { void navigator.clipboard.readText().then((text) => ws.paste(left, top, text)).catch(() => undefined) } },
    line,
    { title: 'Отменить', shortcut: 'Ctrl+Z', onclick: props.onUndo },
    { title: 'Повторить', shortcut: 'Ctrl+Y', onclick: props.onRedo },
  ]
}
