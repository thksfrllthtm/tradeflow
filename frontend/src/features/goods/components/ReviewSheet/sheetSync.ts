import { MutableRefObject } from 'react'
import type jspreadsheet from 'jspreadsheet-ce'
import { fixedNumber } from '../../lib/resultValues'
import { columnIndex, SHEET_COLUMNS, SheetKey, sheetRow } from './sheetModel'
import { CellView, cellView, escape, numberView, quantityText } from './sheetCells'
import type { ReviewSheetProps, SheetTotals } from './ReviewSheet'

export type Worksheet = jspreadsheet.WorksheetInstance

// Наименьшая ширина листа: все колонки по своей ширине, «Товар» — не уже обычной (или той, что
// задали, потянув границу колонки). Остальное место «Товар» получает сам.
export function fitNameColumn(ws: Worksheet) {
  const cols = [...ws.colgroupContainer.children]
  const name = cols[columnIndex('name') + 1]
  const width = (col: Element) => (col as HTMLElement).style.display === 'none' ? 0 : Number(col.getAttribute('width')) || 0
  const others = cols.reduce((sum, col) => col === name ? sum : sum + width(col), 0)
  // min-width таблица с table-layout: fixed не соблюдает — поэтому width: max(…).
  ws.table.style.width = `max(100%, ${others + Math.max(width(name), SHEET_COLUMNS[columnIndex('name')].width)}px)`
}

// ── Синхронизация: строки сметы → лист ──

function silently(ws: Worksheet, action: () => void) {
  const events = ws.parent.ignoreEvents
  const history = ws.ignoreHistory
  ws.parent.ignoreEvents = true
  ws.ignoreHistory = true
  try { action() } finally {
    ws.parent.ignoreEvents = events
    ws.ignoreHistory = history
  }
}

export function syncSheet(ws: Worksheet, props: ReviewSheetProps, idsRef: MutableRefObject<string[]>) {
  const ids = props.rows.map((row) => row.item.id ?? '')
  const known = idsRef.current
  const length = ws.options.data?.length ?? 0
  // Те же строки (или лист уже дополнен вставкой из Excel) — обновляем ячейки на месте:
  // выделение и прокрутка не сбрасываются. Строки дописали в конец — дописываем в лист.
  // Иначе строки удалили или переставили — лист заполняется заново.
  const prefix = known.every((id, y) => y >= ids.length || ids[y] === id)
  silently(ws, () => {
    if (prefix && length < ids.length && known.length === length) ws.insertRow(ids.length - length)
    else if (!prefix || length !== ids.length) ws.setData(props.rows.map((row) => sheetRow(row.item)))
    idsRef.current = ids
    const data = ws.options.data as unknown[][]
    props.rows.forEach((row, y) => {
      sheetRow(row.item).forEach((value, x) => {
        if (String(data[y]?.[x] ?? '') === String(value)) return
        if (ws.records[y][x].element.classList.contains('editor')) return
        ws.setValueFromCoords(x, y, value, true)
      })
    })
  })
  decorate(ws, props)
}

// Колонку можно спрятать: своё скрытие, а не ws.hideColumn — та сбрасывает выделение и не прячет
// ячейки строк, добавленных позже. Возвращает true, если видимость поменялась.
function setColumnShown(ws: Worksheet, key: SheetKey, shown: boolean) {
  const x = columnIndex(key)
  const display = shown ? '' : 'none'
  if (ws.headers[x].style.display === display) return false
  ws.headers[x].style.display = display
  ws.cols[x].colElement.style.display = display
  return true
}

// Вид ячеек, которого нет в значениях: номер с состоянием и выбором, фото, кнопки, авто-значения,
// подсветка проблем, повторы кода. drawn — что уже нарисовано в ячейке: ключ вида и разметка, как её
// вернул браузер. Фото не перерисовываются на каждый рендер, а ячейку, которую переписала библиотека,
// видно по разметке.
const drawn = new WeakMap<HTMLElement, { key: string; html: string }>()

function draw(td: HTMLElement, view: CellView, readOnly: boolean) {
  const key = `${view.key ?? view.html}|${view.title}|${view.className ?? ''}`
  const before = drawn.get(td)
  // Ячейку со значением библиотека перерисовывает сама при правке — сверяем и разметку.
  if (before?.key === key && (readOnly || td.innerHTML === before.html)) return
  td.innerHTML = view.html
  td.title = view.title
  td.className = td.className.replace(/\bxs-\S+/g, '').trim()
  if (view.className) td.classList.add(...view.className.split(' '))
  drawn.set(td, { key, html: readOnly ? '' : td.innerHTML })
}

function decorate(ws: Worksheet, props: ReviewSheetProps) {
  const visibleRows = props.rows.filter((row) => row.visible)
  const all = visibleRows.length > 0 && visibleRows.every((row) => row.selected)
  const selectAll = ws.headerContainer.querySelector<HTMLInputElement>('input[data-act="select-all"]')
  if (selectAll) { selectAll.checked = all; selectAll.indeterminate = !all && visibleRows.some((row) => row.selected) }
  const toggled = [setColumnShown(ws, 'photo', props.showPhoto), setColumnShown(ws, 'packageWeightKg', props.showPackage)].some(Boolean)
  const hidden = new Set<number>([...(props.showPhoto ? [] : [columnIndex('photo')]), ...(props.showPackage ? [] : [columnIndex('packageWeightKg')])])
  drawTotals(ws, props.totals, hidden)
  if (toggled) fitNameColumn(ws)
  // Повтор строки выше (среди показанных): то же наименование И тот же код — название и код серым (совпал только
  // код или только наименование — обычным цветом); начало новой группы кодов — линия темнее.
  let previousCode: string | null = null
  let previousName = ''
  props.rows.forEach((row, y) => {
    const tr = ws.rows[y]?.element
    if (!tr) return
    tr.style.display = row.visible ? '' : 'none'
    const code = row.item.code.trim()
    const name = row.item.name.trim()
    const repeat = row.visible && previousCode !== null && code !== '' && code === previousCode && name !== '' && name === previousName
    const groupStart = row.visible && previousCode !== null && code !== previousCode
    if (row.visible) { previousCode = code; previousName = name }
    tr.classList.toggle('xs-excluded', row.item.excluded)
    tr.classList.toggle('xs-selected', row.selected)
    tr.classList.toggle('xs-group-start', groupStart)
    tr.classList.toggle('xs-agent-row', Boolean(props.agentRowId) && row.item.id === props.agentRowId)
    // Ячейку номера библиотека переписывает сама (вставка строк) — сверяем разметку, как у ячеек со значением.
    const index = tr.children[0] as HTMLElement | undefined
    if (index) draw(index, numberView(row, y), false)
    SHEET_COLUMNS.forEach((column, x) => {
      const td = ws.records[y][x].element
      td.style.display = hidden.has(x) ? 'none' : ''
      const view = cellView(column.key, row, props.defaultPackageWeight, repeat)
      if (!view || td.classList.contains('editor')) return
      draw(td, view, Boolean(column.readOnly))
    })
  })
}

// Строка «Итого в расчёте» под колонками: места, количество, брутто, нетто и сколько строк в расчёте.
function drawTotals(ws: Worksheet, totals: SheetTotals, hidden: Set<number>) {
  const tr = ws.table.querySelector<HTMLTableRowElement>('tfoot.xs-foot > tr')
  if (!tr) return
  const value = (key: SheetKey) => {
    if (key === 'name') return '<b>Итого в расчёте</b><span class="xs-foot-sel" role="status"></span>'
    if (key === 'places') return escape(quantityText(totals.places))
    if (key === 'pairs') return escape(quantityText(totals.quantity))
    if (key === 'grossKg') return escape(fixedNumber(totals.grossKg, 1))
    if (key === 'netKg') return escape(fixedNumber(totals.netKg, 1))
    if (key === 'included') return `<span class="xs-foot-count">${totals.included} из ${totals.all}</span>`
    return ''
  }
  const html = `<td></td>${SHEET_COLUMNS.map((column, x) => `<td class="xs-foot-${column.key}"${hidden.has(x) ? ' style="display:none"' : ''}>${value(column.key)}</td>`).join('')}`
  // Сверяем с прошлой разметкой, а не с innerHTML: в ячейке «Итого» ещё и сумма выделенного (ставит ReviewSheet).
  if (tr.dataset.html !== html) { tr.innerHTML = html; tr.dataset.html = html }
}
