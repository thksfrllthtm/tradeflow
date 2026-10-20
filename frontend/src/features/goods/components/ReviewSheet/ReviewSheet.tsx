import { MutableRefObject, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type jspreadsheet from 'jspreadsheet-ce'
import { GoodsItemInput } from '@/shared/types/goods'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { photosOf } from '@/shared/lib/itemPhotos'
import { formatCode } from '@/shared/tnved/codes'
import { autoNetFor, DEFAULT_PACKAGE_WEIGHT, effectivePackageWeight, formatWeight, netHint } from '../../lib/netWeight'
import { autoQuantity, autoQuantityHint, QuantityField } from '../../lib/quantities'
import { ReviewProblem } from '../../lib/reviewProblems'
import { hasNomenclatureRates, reviewStatus } from '../ReviewRow/reviewStatus'
import { dutyRateText, fixedNumber, itemNames } from '../ResultStep/resultValues'
import { AgentRowStatus } from '../../agent/agentApi'
import { canSaveRowDecision, RowDecisionState } from '../../hooks/useRowDecisions'
import { columnIndex, copyValue, NUMBER_FIELDS, SHEET_COLUMNS, SheetChange, SheetKey, sheetRow } from './sheetModel'
import './ReviewSheet.css'

export type SheetRowView = {
  item: GoodsItemInput
  status: ReturnType<typeof reviewStatus>
  // Проблемы строки по ячейкам (для подсветки) и код, предложенный подбором ИИ и совпадающий с кодом строки.
  problems: ReviewProblem[]
  aiCode: boolean
  visible: boolean
  selected: boolean
  agent?: AgentRowStatus
  decision?: RowDecisionState
}

export type RowAction = 'photos' | 'upload' | 'picker' | 'agent' | 'decision' | 'select'

// Итоги включённых строк — строка «Итого в расчёте» под колонками.
export type SheetTotals = { places: number; quantity: number; grossKg: number; netKg: number; included: number; all: number }

// Управление листом снаружи: F8 ставит курсор на ячейку проблемы.
export type SheetApi = { focusCell: (index: number, key: SheetKey) => void; currentRow: () => number }

// Выбранная ячейка — для строки значения над таблицей.

type Props = {
  rows: SheetRowView[]
  defaultPackageWeight: string
  showPhoto: boolean
  showPackage: boolean
  compact: boolean
  totals: SheetTotals
  apiRef: MutableRefObject<SheetApi | null>
  onChanges: (changes: SheetChange[]) => void
  onAction: (action: RowAction, index: number) => void
  onSelectAll: () => void
  onInsertRow: (index: number) => void
  onDeleteRows: (indexes: number[]) => void
  onPickRows: (indexes: number[]) => void
  onUndo: () => void
  onRedo: () => void
}

type Worksheet = jspreadsheet.WorksheetInstance

// Лист «как в Excel» на Jspreadsheet CE (MIT): выделение диапазонов, Ctrl+C / Ctrl+V с Excel,
// Delete, протягивание за уголок, Ctrl+Z / Ctrl+Y. Строки сметы — источник правды: правки листа
// уходят в onChanges, а лист после каждого рендера приводится к строкам (sync). Отмена — своя,
// по строкам сметы (onUndo / onRedo): история библиотеки не знает о выборе кода, фото и т. п.
// Лист без своей прокрутки: прокручивается страница, шапка прилипает к верху, «Итого» — к низу окна.
export function ReviewSheet(props: Props) {
  const host = useRef<HTMLDivElement>(null)
  const sheet = useRef<Worksheet | null>(null)
  const latest = useRef(props)
  latest.current = props
  // id строк в порядке листа: по ним видно, что строки добавили, удалили или переставили.
  const idsRef = useRef<string[]>([])
  const syncRef = useRef(() => {})
  const [error, setError] = useState('')
  const [selectionSum, setSelectionSum] = useState('')

  useLayoutEffect(() => {
    syncRef.current = () => {
      const ws = sheet.current
      if (!ws) return
      syncSheet(ws, latest.current, idsRef)
      // Сумма выделенного — по значениям после правки (вставка выделяет диапазон раньше, чем строки обновятся).
      const range = ws.selectedCell?.map(Number)
      setSelectionSum(range ? selectionSummary(ws, range[0], range[1], range[2], range[3]) : '')
    }
  })
  // Каждый рендер: строки, статусы подбора, база решений, фильтр — всё приводится к листу.
  useEffect(() => { syncRef.current() })
  // Сумма выделенного — в строке «Итого», рядом с подписью: своя строка под листом появлялась и пропадала
  // при выделении, и лист прыгал по высоте.
  useEffect(() => {
    const slot = sheet.current?.table.querySelector('.xs-foot-sel')
    if (slot && slot.textContent !== selectionSum) slot.textContent = selectionSum
  })

  useEffect(() => {
    let cancelled = false
    let created: typeof jspreadsheet | null = null
    let resized: ResizeObserver | null = null
    const element = host.current
    if (!element) return
    // Кнопки внутри ячеек (фото, значки у кода, «Указать код», выбор строки, флажок «все»).
    const onClick = (event: MouseEvent) => {
      const target = (event.target as HTMLElement).closest<HTMLElement>('[data-act]')
      if (!target || !element.contains(target)) return
      // Флажок «все» ставит синхронизация. После preventDefault браузер возвращает флажок в прежнее
      // состояние уже после обработчиков — поэтому сверяем его ещё раз, когда это произойдёт.
      if (target.dataset.act === 'select-all') { event.preventDefault(); latest.current.onSelectAll(); window.setTimeout(() => syncRef.current()); return }
      const y = Number(target.closest<HTMLElement>('td[data-y]')?.dataset.y ?? target.closest<HTMLElement>('tr[data-y]')?.dataset.y)
      if (!Number.isInteger(y)) return
      if (target.dataset.act === 'select') { event.preventDefault(); latest.current.onAction('select', y); return }
      latest.current.onAction(target.dataset.act as RowAction, y)
      // Значок открывает окно (справочник, чат, фото): второй щелчок двойного щелчка попал бы
      // в открывшееся окно — например, выбрал бы первый код списка. Его пропускаем.
      const swallow = (next: MouseEvent) => { if (next.detail > 1) { next.preventDefault(); next.stopPropagation() } }
      document.addEventListener('click', swallow, { capture: true, once: true })
      window.setTimeout(() => document.removeEventListener('click', swallow, { capture: true }), 600)
    }
    // Двойной щелчок по значку или кнопке в ячейке — не правка ячейки; флажок выбора строки не выделяет строку.
    const onDoubleClick = (event: MouseEvent) => {
      if ((event.target as HTMLElement).closest('[data-act]')) event.stopPropagation()
    }
    const onMouseDown = (event: MouseEvent) => {
      if ((event.target as HTMLElement).closest('[data-act="select"], [data-act="select-all"]')) event.stopPropagation()
    }
    // Библиотека слушает клавиши на всём документе и печатает в выбранную ячейку, даже когда
    // курсор в поле поверх таблицы (поиск в окне выбора кода). Ввод в чужое поле снимает с листа фокус.
    const onKeyDownOutside = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement
      if (created && !element.contains(target) && target.matches('input, textarea, select, [contenteditable]')) {
        (created as unknown as { current: unknown }).current = null
      }
    }
    element.addEventListener('click', onClick)
    element.addEventListener('dblclick', onDoubleClick, true)
    element.addEventListener('mousedown', onMouseDown, true)
    document.addEventListener('keydown', onKeyDownOutside, true)
    void (async () => {
      try {
        const [{ default: jss }] = await Promise.all([import('jspreadsheet-ce'), import('jspreadsheet-ce/dist/jspreadsheet.css'), import('jsuites/dist/jsuites.css')])
        if (cancelled) return
        const rows = latest.current.rows
        const [ws] = jss(element as unknown as jspreadsheet.JspreadsheetInstanceElement, {
          about: false,
          parseFormulas: false,
          // Протягивание копирует значение, как в Excel без Ctrl (а не 200, 201, 202…).
          autoIncrement: false,
          allowExport: false,
          worksheets: [{
            data: rows.map((row) => sheetRow(row.item)),
            columns: SHEET_COLUMNS.map((column) => ({ type: column.type, title: column.title, width: column.width, readOnly: column.readOnly, align: column.align })),
            tableOverflow: false,
            tableWidth: '100%',
            columnSorting: false,
            columnDrag: false,
            rowDrag: false,
            // Вставка из Excel длиннее таблицы добавляет строки; Enter на последней строке — нет.
            allowInsertRow: true,
            allowManualInsertRow: false,
            allowInsertColumn: false,
            allowManualInsertColumn: false,
            allowDeleteRow: false,
            allowDeleteColumn: false,
            allowRenameColumn: false,
            allowComments: false,
            wordWrap: false,
          }],
          onafterchanges: (_instance, changes) => {
            latest.current.onChanges(changes.map((change) => ({ x: Number(change.x), y: Number(change.y), value: change.value })))
            // После рендера с новыми строками лист покажет нормализованные значения.
            requestAnimationFrame(() => syncRef.current())
          },
          // Esc или ввод того же значения событий изменения не дают — вид ячейки возвращаем сами.
          oneditionend: () => { requestAnimationFrame(() => syncRef.current()) },
          oncopy: (instance, range) => copiedText(instance, range),
          onresizecolumn: (instance) => fitNameColumn(instance),
          onselection: (instance, x1, y1, x2, y2) => {
            setSelectionSum(selectionSummary(instance, x1, y1, x2, y2))
          },
          contextMenu: (instance, _x, y) => contextMenu(instance, Number(y), latest.current),
        })
        created = jss
        sheet.current = ws
        idsRef.current = rows.map((row) => row.item.id ?? '')
        // Ctrl+Z / Ctrl+Y — наша отмена по строкам сметы; Ctrl+S — сохранение черновика, а не CSV.
        ws.undo = () => latest.current.onUndo()
        ws.redo = () => latest.current.onRedo()
        ws.download = () => undefined
        // Пустой заголовок библиотека заменяет буквой колонки (A, B…) — оставляем пустым.
        SHEET_COLUMNS.forEach((column, x) => {
          if (column.hint) ws.headers[x].title = column.hint
          if (!column.title) ws.headers[x].textContent = ''
        })
        // Флажок «выбрать все показанные» — в углу шапки, над номерами строк.
        const corner = ws.headerContainer.children[0] as HTMLElement | undefined
        if (corner) corner.innerHTML = '<input type="checkbox" class="xs-pick-all" data-act="select-all" aria-label="Выбрать все показанные строки" title="Выбрать все показанные строки">'
        // Строка «Итого в расчёте» — своя строка таблицы.
        const foot = document.createElement('tfoot')
        foot.className = 'xs-foot'
        foot.innerHTML = '<tr></tr>'
        ws.table.appendChild(foot)
        latest.current.apiRef.current = {
          focusCell: (index, key) => {
            const x = columnIndex(key)
            ws.updateSelectionFromCoords(x, index, x, index)
            ws.records[index]?.[x]?.element.scrollIntoView({ block: 'center' })
          },
          currentRow: () => ws.selectedCell ? Math.min(Number(ws.selectedCell[1]), Number(ws.selectedCell[3])) : -1,
        }
        // «Товар» без своей ширины забирает всё свободное место (таблица — на всю ширину, раскладку
        // при любом изменении окна делает браузер); min-width не даёт ему сжаться меньше обычного.
        ws.colgroupContainer.children[columnIndex('name') + 1]?.removeAttribute('width')
        // Колонка номера: флажок слева, номер справа — шире стандартной, чтобы они не наезжали друг на друга.
        ws.colgroupContainer.children[0]?.setAttribute('width', '72')
        fitNameColumn(ws)
        // Рамку выделения и маркер протягивания библиотека ставит один раз, при выделении. Колонки
        // меняют ширину (открылась панель справа, изменилось окно) — ставим их заново.
        // --xs-top — где лист начинается на странице, --xs-below — сколько под ним до конца сметы (подпись,
        // отступы): смета ровно в окно, без прокрутки страницы (CSS). Страница сметы — тоже: над листом и под ним
        // появляются строки («Строка удалена», сумма выделенного), и лист должен под них ужаться.
        resized = new ResizeObserver(() => {
          const box = element.getBoundingClientRect()
          const page = element.closest('.goods-page')?.getBoundingClientRect()
          element.style.setProperty('--xs-top', `${box.top + window.scrollY}px`)
          element.style.setProperty('--xs-below', `${page ? Math.max(0, page.bottom - box.bottom) : 0}px`)
          const cell = ws.selectedCell
          if (cell) ws.updateSelectionFromCoords(Number(cell[0]), Number(cell[1]), Number(cell[2]), Number(cell[3]))
        })
        resized.observe(ws.table)
        resized.observe(document.body)
        const page = element.closest('.goods-page')
        if (page) resized.observe(page)
        syncRef.current()
        setError('')
      } catch (reason) {
        if (!cancelled) setError(reason instanceof Error ? reason.message : 'Не удалось открыть таблицу')
      }
    })()
    return () => {
      cancelled = true
      sheet.current = null
      latest.current.apiRef.current = null
      element.removeEventListener('click', onClick)
      element.removeEventListener('dblclick', onDoubleClick, true)
      element.removeEventListener('mousedown', onMouseDown, true)
      document.removeEventListener('keydown', onKeyDownOutside, true)
      resized?.disconnect()
      if (created) created.destroy(element as unknown as jspreadsheet.JspreadsheetInstanceElement, true)
      element.innerHTML = ''
    }
  }, [])

  return <div className={props.compact ? 'xs-wrap compact' : 'xs-wrap'}>
    {error && <p className="xp-error">Не удалось открыть таблицу: {error}</p>}
    <div ref={host} className="xs-host" aria-label="Строки сметы: стрелки — переход, Enter или F2 — правка, Ctrl+C / Ctrl+V — копирование и вставка с Excel, Delete — очистить, Ctrl+Z — отменить" />
  </div>
}

// Наименьшая ширина листа: все колонки по своей ширине, «Товар» — не уже обычной (или той, что
// задали, потянув границу колонки). Остальное место «Товар» получает сам.
function fitNameColumn(ws: Worksheet) {
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

function syncSheet(ws: Worksheet, props: Props, idsRef: MutableRefObject<string[]>) {
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

function decorate(ws: Worksheet, props: Props) {
  const visibleRows = props.rows.filter((row) => row.visible)
  const all = visibleRows.length > 0 && visibleRows.every((row) => row.selected)
  const selectAll = ws.headerContainer.querySelector<HTMLInputElement>('input[data-act="select-all"]')
  if (selectAll) { selectAll.checked = all; selectAll.indeterminate = !all && visibleRows.some((row) => row.selected) }
  const toggled = [setColumnShown(ws, 'photo', props.showPhoto), setColumnShown(ws, 'packageWeightKg', props.showPackage)].some(Boolean)
  const hidden = new Set<number>([...(props.showPhoto ? [] : [columnIndex('photo')]), ...(props.showPackage ? [] : [columnIndex('packageWeightKg')])])
  drawTotals(ws, props.totals, hidden)
  if (toggled) fitNameColumn(ws)
  // Повтор кода строки выше (среди показанных) — название, код и ставка серым; начало новой группы кодов — линия темнее.
  let previousCode: string | null = null
  props.rows.forEach((row, y) => {
    const tr = ws.rows[y]?.element
    if (!tr) return
    tr.style.display = row.visible ? '' : 'none'
    const code = row.item.code.trim()
    const repeat = row.visible && previousCode !== null && code !== '' && code === previousCode
    const groupStart = row.visible && previousCode !== null && code !== previousCode
    if (row.visible) previousCode = code
    tr.classList.toggle('xs-excluded', row.item.excluded)
    tr.classList.toggle('xs-selected', row.selected)
    tr.classList.toggle('xs-group-start', groupStart)
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

type CellView = { html: string; title: string; className?: string; key?: string }

const escape = (text: string) => text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
const ru = (text: string) => text.replace(/(\d)\.(\d)/g, '$1,$2')
const auto = (text: string) => `<span class="xs-auto">${escape(text)}</span>`
// Количество — целое; дробное (места из количества ÷ штук в месте) — как есть, чтобы ошибку было видно.
const quantityText = (value: number) => formatRuNumber(Math.abs(value - Math.round(value)) < 1e-9 ? String(Math.round(value)) : String(Number(value.toFixed(3))))
// Вес в ячейке — один знак после запятой («20 230,0»); точное значение — в строке значения над таблицей.
const weightCell = (value: string) => { const text = fixedNumber(value, 1); return text === '—' ? '' : text }

// Ячейка номера: номер, цветная точка состояния (ошибка — красная, предупреждение — жёлтая),
// флажок выбора строки — при наведении и у выбранных строк.
function numberView(row: SheetRowView, y: number): CellView {
  const tone = row.problems.some((problem) => problem.tone === 'error') ? 'error' : row.problems.length > 0 ? 'warn' : ''
  const changed = row.status.tone === 'changed'
  const dot = tone ? `<i class="xs-dot ${tone}" aria-hidden="true"></i>` : changed ? '<i class="xs-dot changed" aria-hidden="true"></i>' : ''
  const pick = `<input type="checkbox" class="xs-pick" data-act="select"${row.selected ? ' checked' : ''} aria-label="Выбрать строку ${y + 1}">`
  const title = [row.status.label, ...row.problems.map((problem) => problem.text)].filter(Boolean).join('\n')
  return { html: `${pick}${dot}<span class="xs-num">${y + 1}</span>`, title, className: ['xs-index', row.selected ? 'xs-picked' : '', tone ? `xs-row-${tone}` : ''].filter(Boolean).join(' ') }
}

// Подсветка ячейки с проблемой: ошибка важнее предупреждения; текст проблемы — в подсказке.
function problemOf(row: SheetRowView, key: SheetKey) {
  const own = row.problems.filter((problem) => problem.column === key)
  if (own.length === 0) return { className: '', text: '' }
  return { className: own.some((problem) => problem.tone === 'error') ? 'xs-err' : 'xs-warn', text: own.map((problem) => problem.text).join('\n') }
}

function cellView(key: SheetKey, row: SheetRowView, defaultPackageWeight: string, repeat: boolean): CellView | null {
  const { item } = row
  const problem = problemOf(row, key)
  const withProblem = (view: CellView): CellView => problem.className
    ? { ...view, title: [problem.text, view.title].filter(Boolean).join('\n'), className: [view.className, problem.className].filter(Boolean).join(' ') }
    : view
  switch (key) {
    case 'photo': {
      const photos = photosOf(item)
      const add = `<button type="button" class="xs-photo-add${photos.length ? '' : ' empty'}" data-act="upload" title="Добавить фото" aria-label="Добавить фото">+</button>`
      if (photos.length === 0) return { html: add, title: '' }
      const html = `<button type="button" class="xs-thumb" data-act="photos" title="Фото строки: посмотреть, удалить, перенести"><img src="${escape(photos[0])}" alt="">${photos.length > 1 ? `<b>${photos.length}</b>` : ''}</button>${add}`
      return { html, title: '', key: `${photos.length}:${photos[0].length}:${photos[0].slice(-24)}` }
    }
    case 'name': {
      // Русское название и через точку серым — китайское (или исходное из Excel).
      const names = itemNames(item, item)
      const other = item.otherNames?.length ? `\nДругие наименования из файла: ${item.otherNames.join(' · ')}` : ''
      const html = item.name ? `${escape(names.main)}${names.sub ? ` <span class="xs-zh">· ${escape(names.sub)}</span>` : ''}` : auto('Название товара')
      return withProblem({ html, title: item.name + other, className: repeat ? 'xs-repeat' : '' })
    }
    case 'code': {
      // Нет кода — кнопка «Указать код» (справочник); код — моноширинным, метка «ИИ», если его предложил подбор;
      // значки справочника, подбора ИИ и базы решений — у выбранной строки и при наведении.
      if (!item.code) {
        const html = item.excluded ? '' : '<button type="button" class="xs-set-code" data-act="picker">Указать код</button>'
        return withProblem({ html, title: 'Выберите код в справочнике или введите его (двойной щелчок или просто начните печатать)', className: 'xs-code-empty' })
      }
      const agent = agentView(row.agent, item.code)
      const saved = row.decision === 'saved' || typeof row.decision === 'number'
      const decision = !canSaveRowDecision(item) ? ''
        : saved
          ? `<span class="xs-tool saved" title="${typeof row.decision === 'number' ? `Этот товар уже сохранён в базу решений из строки ${row.decision}` : 'Строка сохранена в базу решений (ждёт проверки администратором)'}">${ICONS.saved}</span>`
          : `<button type="button" class="xs-tool" data-act="decision"${row.decision === 'saving' ? ' disabled' : ''} title="${row.decision === 'saving' ? 'Сохраняем в базу решений…' : 'В базу решений: код, наименование и фото строки (ждёт проверки администратором)'}">${ICONS.decision}</button>`
      const tools = `<span class="xs-tools">`
        + `<button type="button" class="xs-tool" data-act="picker" title="Выбрать товар или код ТН ВЭД из справочника">${ICONS.picker}</button>`
        + `<button type="button" class="xs-tool agent ${agent.tone}" data-act="agent" title="${escape(agent.title)}">${ICONS.agent}${agent.mark ? `<i>${escape(agent.mark)}</i>` : ''}</button>`
        + `${decision}</span>`
      const verified = hasNomenclatureRates(item)
      const ai = row.aiCode ? '<span class="xs-ai" title="Код предложил подбор ИИ">ИИ</span>' : ''
      const title = verified ? 'Ставки из номенклатуры ТН ВЭД ЕАЭС' : 'Кода нет в номенклатуре ТН ВЭД ЕАЭС или она не загружена'
      return withProblem({ html: `<span class="xs-code-cell"><span class="xs-code-text">${escape(formatCode(item.code))}</span>${ai}${tools}</span>`, title, className: ['xs-code', repeat ? 'xs-repeat' : ''].filter(Boolean).join(' ') })
    }
    case 'duty': {
      const rate = dutyRateText(item.directorySnapshot).replace(/^ЕТТ\s*/, '')
      return { html: rate ? escape(rate) : '—', title: rate ? `Пошлина ЕТТ из номенклатуры: ${rate}` : 'Ставки нет: код не указан или его нет в номенклатуре', className: ['xs-duty', repeat ? 'xs-repeat' : ''].filter(Boolean).join(' ') }
    }
    case 'places': case 'pairsPerPlace': case 'pairs': {
      if (item[key]) return withProblem({ html: escape(formatRuNumber(item[key])), title: '' })
      // Пустое поле, которое рассчитается: значение серым, формула — в подсказке.
      const hint = autoQuantityHint(item, key as QuantityField)
      const value = autoQuantity(item, key as QuantityField)
      return withProblem({ html: value !== null ? auto(quantityText(value)) : '', title: hint ? ru(hint.title.replace('Рассчитается автоматически', 'Рассчитано')) : '' })
    }
    case 'grossKg': {
      const need = !item.excluded && !item.grossKg.trim() && !item.netKg.trim()
      return withProblem({ html: escape(weightCell(item.grossKg)), title: need ? 'Укажите брутто: без него не посчитать авто-нетто' : '', className: need ? 'xs-need' : '' })
    }
    case 'packageWeightKg': {
      const own = item.packageWeightKg.trim() !== '' && item.packageWeightKg !== defaultPackageWeight
      const title = own ? `Своя упаковка места для этой строки, кг (общая — ${ru(defaultPackageWeight || DEFAULT_PACKAGE_WEIGHT)} кг)` : 'Вес упаковки одного места, кг; влияет только на авто-нетто'
      return { html: item.packageWeightKg ? escape(formatRuNumber(item.packageWeightKg)) : auto(ru(defaultPackageWeight || DEFAULT_PACKAGE_WEIGHT)), title, className: [own ? 'xs-own' : '', item.netKg ? 'xs-muted' : ''].filter(Boolean).join(' ') }
    }
    case 'netKg': {
      const netItem = { ...item, packageWeightKg: effectivePackageWeight(item, defaultPackageWeight) }
      const hint = netHint(netItem)
      const autoNet = autoNetFor(netItem)
      const html = item.netKg ? escape(weightCell(item.netKg)) : autoNet !== null ? auto(weightCell(String(autoNet))) : ''
      return withProblem({ html, title: ru(hint.text) + (item.netKg && item.netSource !== 'excel' ? '\nDelete — вернуть авто-нетто' : ''), className: hint.className === 'net-bad' && !problem.className ? 'xs-err' : '' })
    }
    default:
      return null
  }
}

// Значки кнопок в ячейке кода: справочник (лупа), подбор ИИ (искра), база решений (закладка).
const icon = (body: string) => `<svg width="15" height="15" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">${body}</svg>`
const ICONS = {
  picker: icon('<circle cx="7" cy="7" r="4.2"/><path d="m10.2 10.2 3.3 3.3"/>'),
  agent: icon('<path d="M8 1.8 9.4 6.6 14.2 8 9.4 9.4 8 14.2 6.6 9.4 1.8 8 6.6 6.6Z"/>'),
  decision: icon('<path d="M4 2.5h8v11L8 10.8 4 13.5Z"/><path d="M8 5v3.4M6.3 6.7h3.4"/>'),
  saved: icon('<path d="M4 2.5h8v11L8 10.8 4 13.5Z" fill="currentColor" fill-opacity=".18"/><path d="m6 6.6 1.5 1.5 2.6-2.8"/>'),
}

// Кнопка чата подбора ИИ: отметка состояния (…, ?, !, ✓ или предложенный код).
function agentView(status: AgentRowStatus | undefined, code: string): { tone: string; mark: string; title: string } {
  if (status?.status === 'running') return { tone: 'running', mark: '…', title: 'Идёт подбор кода — открыть чат' }
  if (status?.status === 'searching') return { tone: 'running', mark: '…', title: 'Поиск в базе решений — открыть чат' }
  if (status?.status === 'ready') return { tone: 'idle', mark: '', title: 'Поиск в базе закончен — открыть чат и начать подбор' }
  if (status?.status === 'waiting') return { tone: 'waiting', mark: '?', title: 'Подбор ждёт ответа на вопрос — открыть чат' }
  if (status?.status === 'error') return { tone: 'error', mark: '!', title: 'Подбор прервался — открыть чат' }
  if (status?.status === 'done' && status.bestCode) {
    return status.bestCode === code
      ? { tone: 'same', mark: '✓', title: 'Код совпадает с подбором — открыть чат' }
      : { tone: 'suggest', mark: '≠', title: `Подбор предлагает другой код ${formatCode(status.bestCode)} — открыть чат` }
  }
  return { tone: 'idle', mark: '', title: status ? 'Открыть чат подбора' : 'Подобрать код ТН ВЭД с ИИ' }
}

// ── Буфер обмена, строка состояния, контекстное меню ──

function selectedRange(ws: Worksheet, fallbackRow: number) {
  const cell = ws.selectedCell?.map(Number) ?? [0, fallbackRow, 0, fallbackRow]
  return { left: Math.min(cell[0], cell[2]), top: Math.min(cell[1], cell[3]), right: Math.max(cell[0], cell[2]), bottom: Math.max(cell[1], cell[3]) }
}

const shown = (ws: Worksheet, y: number) => ws.rows[y]?.element.style.display !== 'none'

// Копирование для Excel: без скрытых фильтром строк, числа с запятой, без служебных колонок.
function copiedText(ws: Worksheet, [x1, y1, x2, y2]: [number, number, number, number]) {
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
function selectionSummary(ws: Worksheet, x1: number, y1: number, x2: number, y2: number) {
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

function contextMenu(ws: Worksheet, clickedRow: number, props: Props): jspreadsheet.ContextMenuItem[] {
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
