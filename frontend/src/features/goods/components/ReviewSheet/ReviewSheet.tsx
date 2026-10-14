import { MutableRefObject, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type jspreadsheet from 'jspreadsheet-ce'
import { GoodsItemInput } from '@/shared/types/goods'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { photosOf } from '@/shared/lib/itemPhotos'
import { formatCode } from '@/shared/tnved/codes'
import { autoNetFor, DEFAULT_PACKAGE_WEIGHT, effectivePackageWeight, formatWeight, netHint } from '../../lib/netWeight'
import { autoQuantityHint, QuantityField } from '../../lib/quantities'
import { hasNomenclatureRates, reviewStatus, ReviewTone } from '../ReviewRow/reviewStatus'
import { AgentRowStatus } from '../../agent/agentApi'
import { canSaveRowDecision, RowDecisionState } from '../../hooks/useRowDecisions'
import { columnIndex, copyValue, NUMBER_FIELDS, SHEET_COLUMNS, SheetChange, SheetKey, sheetRow } from './sheetModel'
import './ReviewSheet.css'

export type SheetRowView = {
  item: GoodsItemInput
  status: ReturnType<typeof reviewStatus>
  visible: boolean
  selected: boolean
  agent?: AgentRowStatus
  decision?: RowDecisionState
}

export type RowAction = 'photos' | 'upload' | 'picker' | 'agent' | 'decision'

// Управление листом снаружи: F8 ставит курсор на строку.
export type SheetApi = { focusRow: (index: number) => void; currentRow: () => number }

type Props = {
  rows: SheetRowView[]
  defaultPackageWeight: string
  hidePackage: boolean
  compact: boolean
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

  useEffect(() => {
    let cancelled = false
    let created: typeof jspreadsheet | null = null
    const element = host.current
    if (!element) return
    // Кнопки внутри ячеек (фото, значки у кода, флажок «все»).
    const onClick = (event: MouseEvent) => {
      const target = (event.target as HTMLElement).closest<HTMLElement>('[data-act]')
      if (!target || !element.contains(target)) return
      if (target.dataset.act === 'select-all') { event.preventDefault(); latest.current.onSelectAll(); return }
      const y = Number(target.closest<HTMLElement>('td[data-y]')?.dataset.y)
      if (!Number.isInteger(y)) return
      latest.current.onAction(target.dataset.act as RowAction, y)
      // Значок открывает окно (справочник, чат, фото): второй щелчок двойного щелчка попал бы
      // в открывшееся окно — например, выбрал бы первый код списка. Его пропускаем.
      const swallow = (next: MouseEvent) => { if (next.detail > 1) { next.preventDefault(); next.stopPropagation() } }
      document.addEventListener('click', swallow, { capture: true, once: true })
      window.setTimeout(() => document.removeEventListener('click', swallow, { capture: true }), 600)
    }
    // Двойной щелчок по значку у кода — не правка ячейки.
    const onDoubleClick = (event: MouseEvent) => {
      if ((event.target as HTMLElement).closest('[data-act]')) event.stopPropagation()
    }
    element.addEventListener('click', onClick)
    element.addEventListener('dblclick', onDoubleClick, true)
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
            data: rows.map((row) => sheetRow(row.item, row.selected)),
            columns: SHEET_COLUMNS.map((column) => ({ type: column.type, title: column.title, width: column.width, readOnly: column.readOnly, align: column.align })),
            tableOverflow: true,
            tableHeight: 'max(300px, calc(100vh - 330px))',
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
          onselection: (instance, x1, y1, x2, y2) => setSelectionSum(selectionSummary(instance, x1, y1, x2, y2)),
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
        ws.headers[columnIndex('select')].innerHTML = '<input type="checkbox" data-act="select-all" aria-label="Выбрать все показанные строки">'
        latest.current.apiRef.current = {
          focusRow: (index) => {
            const x = columnIndex('name')
            ws.updateSelectionFromCoords(x, index, x, index)
            ws.records[index]?.[x]?.element.scrollIntoView({ block: 'center' })
          },
          currentRow: () => ws.selectedCell ? Math.min(Number(ws.selectedCell[1]), Number(ws.selectedCell[3])) : -1,
        }
        // «Товар» без своей ширины забирает всё свободное место (таблица — на всю ширину, раскладку
        // при любом изменении окна делает браузер); min-width не даёт ему сжаться меньше обычного.
        ws.colgroupContainer.children[columnIndex('name') + 1]?.removeAttribute('width')
        fitNameColumn(ws)
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
      if (created) created.destroy(element as unknown as jspreadsheet.JspreadsheetInstanceElement, true)
      element.innerHTML = ''
    }
  }, [])

  return <div className={props.compact ? 'xs-wrap compact' : 'xs-wrap'}>
    {error && <p className="xp-error">Не удалось открыть таблицу: {error}</p>}
    <div ref={host} className="xs-host" aria-label="Строки сметы: стрелки — переход, Enter или F2 — правка, Ctrl+C / Ctrl+V — копирование и вставка с Excel, Delete — очистить, Ctrl+Z — отменить" />
    {selectionSum && <p className="xs-selection" role="status">{selectionSum}</p>}
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
    else if (!prefix || length !== ids.length) ws.setData(props.rows.map((row) => sheetRow(row.item, row.selected)))
    idsRef.current = ids
    const data = ws.options.data as unknown[][]
    props.rows.forEach((row, y) => {
      sheetRow(row.item, row.selected).forEach((value, x) => {
        if (String(data[y]?.[x] ?? '') === String(value)) return
        if (ws.records[y][x].element.classList.contains('editor')) return
        ws.setValueFromCoords(x, y, value, true)
      })
    })
  })
  decorate(ws, props)
}

// Вид ячеек, которого нет в значениях: состояние, фото, кнопки, авто-значения, подсветка.
// drawn — что уже нарисовано в ячейке: ключ вида и разметка, как её вернул браузер. Фото не
// перерисовываются на каждый рендер, а ячейку, которую переписала библиотека, видно по разметке.
const drawn = new WeakMap<HTMLElement, { key: string; html: string }>()

function decorate(ws: Worksheet, props: Props) {
  const visibleRows = props.rows.filter((row) => row.visible)
  const all = visibleRows.length > 0 && visibleRows.every((row) => row.selected)
  const selectAll = ws.headers[columnIndex('select')]?.querySelector('input')
  if (selectAll) { selectAll.checked = all; selectAll.indeterminate = !all && visibleRows.some((row) => row.selected) }
  // Колонка упаковки, когда нетто есть везде. Своё скрытие, а не ws.hideColumn: та сбрасывает
  // выделение и не прячет ячейки строк, добавленных позже.
  const packageColumn = columnIndex('packageWeightKg')
  const packageDisplay = props.hidePackage ? 'none' : ''
  const toggled = ws.headers[packageColumn].style.display !== packageDisplay
  ws.headers[packageColumn].style.display = packageDisplay
  ws.cols[packageColumn].colElement.style.display = packageDisplay
  ws.records.forEach((record) => { if (record[packageColumn]) record[packageColumn].element.style.display = packageDisplay })
  if (toggled) fitNameColumn(ws)
  props.rows.forEach((row, y) => {
    const tr = ws.rows[y]?.element
    if (!tr) return
    tr.style.display = row.visible ? '' : 'none'
    tr.classList.toggle('xs-excluded', row.item.excluded)
    tr.classList.toggle('xs-selected', row.selected)
    tr.classList.toggle('xs-attention', row.status.attention)
    SHEET_COLUMNS.forEach((column, x) => {
      const view = cellView(column.key, row, props.defaultPackageWeight)
      if (!view) return
      const td = ws.records[y][x].element
      if (td.classList.contains('editor')) return
      const key = `${view.key ?? view.html}|${view.title}|${view.className ?? ''}`
      const before = drawn.get(td)
      // Ячейку со значением библиотека перерисовывает сама при правке — сверяем и разметку.
      if (before?.key === key && (column.readOnly || td.innerHTML === before.html)) return
      td.innerHTML = view.html
      td.title = view.title
      td.className = td.className.replace(/\bxs-\S+/g, '').trim()
      if (view.className) td.classList.add(...view.className.split(' '))
      drawn.set(td, { key, html: column.readOnly ? '' : td.innerHTML })
    })
  })
}

type CellView = { html: string; title: string; className?: string; key?: string }

const escape = (text: string) => text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
const ru = (text: string) => text.replace(/(\d)\.(\d)/g, '$1,$2')
const auto = (text: string) => `<span class="xs-auto">${escape(text)}</span>`

function cellView(key: SheetKey, row: SheetRowView, defaultPackageWeight: string): CellView | null {
  const { item, status } = row
  const value = (field: keyof GoodsItemInput) => String(item[field] ?? '')
  switch (key) {
    case 'status':
      return { html: `<span class="xs-state ${status.tone}${status.source === 'past' ? ' past' : ''}" role="img" aria-label="${escape(status.label)}">${STATE_ICONS[status.tone]}</span>`, title: [status.label, ...status.issues].join('\n') }
    case 'photo': {
      const photos = photosOf(item)
      const add = `<button type="button" class="xs-photo-add${photos.length ? '' : ' empty'}" data-act="upload" title="Добавить фото" aria-label="Добавить фото">+</button>`
      if (photos.length === 0) return { html: add, title: '' }
      const html = `<button type="button" class="xs-thumb" data-act="photos" title="Фото строки: посмотреть, удалить, перенести"><img src="${escape(photos[0])}" alt="">${photos.length > 1 ? `<b>${photos.length}</b>` : ''}</button>${add}`
      return { html, title: '', key: `${photos.length}:${photos[0].length}:${photos[0].slice(-24)}` }
    }
    case 'name': {
      const other = item.otherNames?.length ? `\nДругие наименования из файла: ${item.otherNames.join(' · ')}` : ''
      return { html: item.name ? escape(item.name) : auto('Название товара'), title: item.name + other }
    }
    case 'code': {
      // Код слева, справа — значки: справочник, подбор ИИ (с его состоянием), база решений.
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
      const text = item.code ? escape(formatCode(item.code)) : item.excluded ? '' : auto('нужен код')
      const title = !item.code ? 'Введите или вставьте код ТН ВЭД (двойной щелчок или просто начните печатать)'
        : verified ? 'Ставки из номенклатуры ТН ВЭД ЕАЭС' : 'Кода нет в номенклатуре ТН ВЭД ЕАЭС или она не загружена'
      const className = !item.code ? (item.excluded ? '' : 'xs-missing') : verified ? 'xs-code' : 'xs-code xs-warn'
      return { html: `<span class="xs-code-cell"><span class="xs-code-text">${text}</span>${tools}</span>`, title, className }
    }
    case 'places': case 'pairsPerPlace': case 'pairs': {
      if (item[key]) return { html: escape(formatRuNumber(item[key])), title: '' }
      const hint = autoQuantityHint(item, key as QuantityField)
      return { html: hint ? auto(ru(hint.placeholder)) : '', title: hint ? ru(hint.title) : '' }
    }
    case 'grossKg': {
      const need = !item.excluded && !item.grossKg.trim() && !item.netKg.trim()
      return { html: escape(formatRuNumber(item.grossKg)), title: need ? 'Укажите брутто: без него не посчитать авто-нетто' : '', className: need ? 'xs-need' : '' }
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
      const html = item.netKg ? escape(formatRuNumber(value('netKg'))) : autoNet !== null ? auto(`авто ${ru(formatWeight(autoNet))}`) : ''
      return { html, title: ru(hint.text) + (item.netKg && item.netSource !== 'excel' ? '\nDelete — вернуть авто-нетто' : ''), className: hint.className === 'net-bad' ? 'xs-bad' : '' }
    }
    default:
      return null
  }
}

// Значок состояния строки: форма отличается, а не только цвет (✓, !, ?, ↻, –).
const svg = (body: string) => `<svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">${body}</svg>`
const question = '<path d="M4.4 4.4a1.7 1.7 0 1 1 2.4 1.5c-.5.3-.8.6-.8 1.2v.2"/><circle cx="6" cy="9.4" r=".5" fill="currentColor"/>'
const STATE_ICONS: Record<ReviewTone, string> = {
  ok: svg('<path d="M2.8 6.3 5 8.4l4.2-4.6"/>'),
  bad: svg('<path d="M6 2.8v4"/><circle cx="6" cy="9.2" r=".5" fill="currentColor"/>'),
  warn: svg(question),
  todo: svg(question),
  changed: svg('<path d="M9.3 4.6A3.6 3.6 0 1 0 9.6 7"/><path d="M9.6 2.4v2.4H7.2"/>'),
  off: svg('<path d="M3.4 6h5.2"/>'),
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
