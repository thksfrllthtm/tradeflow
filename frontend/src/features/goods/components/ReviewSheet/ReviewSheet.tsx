import { MutableRefObject, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type jspreadsheet from 'jspreadsheet-ce'
import { GoodsItemInput } from '@/shared/types/goods'
import { ReviewProblem } from '../../lib/reviewProblems'
import { reviewStatus } from '../../lib/reviewStatus'
import { AgentRowStatus } from '../../agent/agentApi'
import { RowDecisionState } from '../../hooks/useRowDecisions'
import { columnIndex, SHEET_COLUMNS, SheetChange, SheetKey, sheetRow } from './sheetModel'
import { fitNameColumn, syncSheet, Worksheet } from './sheetSync'
import { contextMenu, copiedText, selectionSummary } from './sheetClipboard'
import { spaceBelow } from '../../lib/spaceBelow'
import { trackFooterGap } from '../../lib/footerGap'
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

export type ReviewSheetProps = {
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
  // Строка, открытая в панели «Подбор кода»: выделена фоном и полоской слева.
  agentRowId?: string | null
}

// Лист «как в Excel» на Jspreadsheet CE (MIT): выделение диапазонов, Ctrl+C / Ctrl+V с Excel,
// Delete, протягивание за уголок, Ctrl+Z / Ctrl+Y. Строки сметы — источник правды: правки листа
// уходят в onChanges, а лист после каждого рендера приводится к строкам (sync). Отмена — своя,
// по строкам сметы (onUndo / onRedo): история библиотеки не знает о выборе кода, фото и т. п.
// Лист без своей прокрутки: прокручивается страница, шапка прилипает к верху, «Итого» — к низу окна.
export function ReviewSheet(props: ReviewSheetProps) {
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
    let stopGap: (() => void) | undefined
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
        // «Итого» — внизу области листа, даже когда строк мало.
        stopGap = trackFooterGap(element, ws.table)
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
          element.style.setProperty('--xs-top', `${box.top + window.scrollY}px`)
          element.style.setProperty('--xs-below', `${spaceBelow(element)}px`)
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
      stopGap?.()
      if (created) created.destroy(element as unknown as jspreadsheet.JspreadsheetInstanceElement, true)
      element.innerHTML = ''
    }
  }, [])

  return <div className={props.compact ? 'xs-wrap compact' : 'xs-wrap'}>
    {error && <p className="xp-error">Не удалось открыть таблицу: {error}</p>}
    <div ref={host} className="xs-host" aria-label="Строки сметы: стрелки — переход, Enter или F2 — правка, Ctrl+C / Ctrl+V — копирование и вставка с Excel, Delete — очистить, Ctrl+Z — отменить" />
  </div>
}
