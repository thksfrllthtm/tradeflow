import { useEffect, useRef, useState } from 'react'
import type jspreadsheet from 'jspreadsheet-ce'
import { GoodsParseRow } from '@/shared/types/goods'
import './ExcelPreview.css'

type Field = { value: string; label: string }

type Props = {
  headers: string[]
  rows: GoodsParseRow[]
  // Поля сметы, которые можно назначить колонке, и текущее назначение колонки ('' — не используется).
  fields: Field[]
  fieldForHeader: (header: string) => string
  onFieldChange: (header: string, field: string) => void
  onIncludedChange: (sourceRow: number, included: boolean) => void
}

// Открытый список назначения колонки: заголовок и где его показать (координаты окна).
type Menu = { header: string; left: number; top: number; width: number }

// Колонки перед колонками файла: флажок импорта и номер строки Excel.
const leadColumns = 2

// Пробный режим «Как в Excel» на шаге «Файл и колонки»: строки файла в Jspreadsheet CE (MIT).
// Ячейки только для чтения, но их можно выделять мышью и копировать (Ctrl+C) прямо в Excel;
// первая колонка — флажок импорта строки, клик по заголовку колонки файла открывает список
// полей сметы. Формулы из файла не вычисляются. Библиотека загружается только здесь.
export function ExcelPreview({ headers, rows, fields, fieldForHeader, onFieldChange, onIncludedChange }: Props) {
  const host = useRef<HTMLDivElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const sheet = useRef<jspreadsheet.WorksheetInstance | null>(null)
  const rowsRef = useRef(rows)
  const changeRef = useRef(onIncludedChange)
  const [error, setError] = useState('')
  const [menu, setMenu] = useState<Menu | null>(null)
  // Растёт при каждом создании таблицы — по нему заголовки подписываются заново.
  const [built, setBuilt] = useState(0)
  rowsRef.current = rows
  changeRef.current = onIncludedChange

  const labelOf = (header: string) => fields.find((field) => field.value === fieldForHeader(header))?.label ?? ''

  // Таблица пересоздаётся, только когда меняются колонки файла или сам набор строк. Назначение
  // колонок и отметки импорта обновляются на месте (ниже) — прокрутка и выделение не сбрасываются.
  const titles = headers.map((header) => { const label = labelOf(header); return label ? `${label} ← ${header}` : header })
  const titlesKey = JSON.stringify(titles)
  const shape = JSON.stringify([headers, rows.map((row) => row.sourceRow)])

  useEffect(() => {
    let cancelled = false
    // Модуль библиотеки, когда таблица создана: очистка уничтожает именно её и сразу.
    let created: typeof jspreadsheet | null = null
    const element = host.current
    if (!element) return
    // Клик по заголовку колонки файла — открыть (или закрыть) список назначения.
    const openMenu = (event: MouseEvent) => {
      const cell = (event.target as HTMLElement).closest<HTMLElement>('thead td[data-x]')
      const index = cell ? Number(cell.dataset.x) - leadColumns : -1
      if (!cell || index < 0 || index >= headers.length) return
      const rect = cell.getBoundingClientRect()
      setMenu((open) => open?.header === headers[index] ? null : { header: headers[index], left: rect.left, top: rect.bottom, width: rect.width })
    }
    element.addEventListener('click', openMenu)
    void (async () => {
      try {
        const [{ default: jss }] = await Promise.all([import('jspreadsheet-ce'), import('jspreadsheet-ce/dist/jspreadsheet.css'), import('jsuites/dist/jsuites.css')])
        if (cancelled) return
        const current = rowsRef.current
        const worksheets = jss(element as unknown as jspreadsheet.JspreadsheetInstanceElement, {
          about: false,
          parseFormulas: false,
          worksheets: [{
            data: current.map((row) => [row.included, String(row.sourceRow), ...headers.map((header) => row.values[header] ?? '')]),
            columns: [
              { type: 'checkbox', title: '✓', width: 40 },
              { type: 'text', title: 'Стр.', width: 56, readOnly: true, align: 'right' },
              ...headers.map((_header, index) => ({ type: 'text' as const, title: titles[index], width: 180, readOnly: true, align: 'left' as const })),
            ],
            tableOverflow: true,
            // Высота — по окну, чтобы таблица и кнопка «Применить» помещались на экране при любом масштабе.
            tableHeight: 'calc(100vh - 380px)',
            tableWidth: '100%',
            columnSorting: false,
            columnDrag: false,
            rowDrag: false,
            allowInsertRow: false,
            allowManualInsertRow: false,
            allowInsertColumn: false,
            allowManualInsertColumn: false,
            allowDeleteRow: false,
            allowDeleteColumn: false,
            allowRenameColumn: false,
            allowComments: false,
            wordWrap: false,
          }],
          // Отметка импорта: флажок первой колонки меняет строку в смете.
          onchange: (_instance, cell, colIndex, rowIndex, newValue) => {
            if (Number(colIndex) !== 0) return
            const row = rowsRef.current[Number(rowIndex)]
            const included = newValue === true || newValue === 'true'
            cell.parentElement?.classList.toggle('xp-excluded', !included)
            if (row && row.included !== included) changeRef.current(row.sourceRow, included)
          },
          // Контекстное меню — только копирование: файл здесь не редактируется.
          contextMenu: (instance) => [{ title: 'Копировать', shortcut: 'Ctrl + C', onclick: () => instance.copy() }],
        })
        created = jss
        sheet.current = worksheets[0]
        // Свои номера строк библиотеки не нужны: номер строки Excel — в колонке «Стр.».
        worksheets[0].hideIndex()
        markExcluded(element, current)
        setBuilt((count) => count + 1)
        setError('')
      } catch (reason) {
        if (!cancelled) setError(reason instanceof Error ? reason.message : 'Не удалось открыть таблицу')
      }
    })()
    return () => {
      cancelled = true
      sheet.current = null
      element.removeEventListener('click', openMenu)
      if (created) created.destroy(element as unknown as jspreadsheet.JspreadsheetInstanceElement, true)
      element.innerHTML = ''
    }
    // shape описывает всё, от чего зависит построение таблицы.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [shape])

  // Подписи заголовков по назначению колонок — прямо в DOM, без пересоздания таблицы и без
  // setHeader библиотеки (он пишет в историю отмены, и Ctrl+Z вернул бы старую подпись).
  useEffect(() => {
    const element = host.current
    if (!element || !sheet.current) return
    element.querySelectorAll<HTMLElement>('thead td[data-x]').forEach((cell) => {
      const index = Number(cell.dataset.x) - leadColumns
      if (index < 0 || index >= headers.length) return
      if (cell.textContent !== titles[index]) cell.textContent = titles[index]
      cell.classList.add('xp-pick')
      cell.classList.toggle('xp-mapped', titles[index] !== headers[index])
      cell.title = 'Выбрать, что в этой колонке'
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [titlesKey, built])

  // «Импортировать все» и другие изменения снаружи — обновляем флажки на месте.
  useEffect(() => {
    const instance = sheet.current
    const element = host.current
    if (!instance || !element) return
    rows.forEach((row, index) => {
      const value = instance.getValueFromCoords(0, index)
      const shown = value === true || value === 'true'
      if (shown !== row.included) instance.setValueFromCoords(0, index, row.included, true)
    })
    markExcluded(element, rows)
  }, [rows])

  // Список закрывается кликом мимо, Esc и прокруткой (он привязан к месту заголовка).
  useEffect(() => {
    if (!menu) return
    const close = (event: Event) => {
      const target = event.target as HTMLElement
      if (menuRef.current?.contains(target)) return
      // Клик по заголовку обрабатывает openMenu: тот же — закрыть, другой — открыть там.
      if (event.type === 'mousedown' && target.closest?.('.xp-host thead td.xp-pick')) return
      setMenu(null)
    }
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') setMenu(null) }
    document.addEventListener('mousedown', close)
    document.addEventListener('scroll', close, true)
    window.addEventListener('resize', close)
    document.addEventListener('keydown', escape)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('scroll', close, true)
      window.removeEventListener('resize', close)
      document.removeEventListener('keydown', escape)
    }
  }, [menu])

  function choose(field: string) {
    if (menu) onFieldChange(menu.header, field)
    setMenu(null)
  }

  const chosen = menu ? fieldForHeader(menu.header) : ''
  // Колонка, у которой поле уже назначено: выбор здесь переносит его сюда.
  const holder = (field: string) => headers.find((header) => header !== menu?.header && fieldForHeader(header) === field)
  return <div className="xp-wrap">
    {error && <p className="xp-error">Не удалось открыть режим «Как в Excel»: {error}. Переключитесь на обычную таблицу.</p>}
    <div ref={host} className="xp-host" />
    {menu && <div ref={menuRef} className="xp-menu" role="listbox" aria-label={`Что в колонке «${menu.header}»`} style={{ left: Math.max(8, Math.min(menu.left, window.innerWidth - 268)), top: menu.top + 2, minWidth: Math.max(menu.width, 220) }}>
      <div className="xp-menu-head">{menu.header}</div>
      <button type="button" role="option" aria-selected={!chosen} className={!chosen ? 'active' : undefined} onClick={() => choose('')}>Не использовать</button>
      {fields.map((field) => {
        const other = holder(field.value)
        return <button key={field.value} type="button" role="option" aria-selected={chosen === field.value} className={chosen === field.value ? 'active' : undefined} onClick={() => choose(field.value)}>
          <span>{field.label}</span>{other && <small>сейчас: {other}</small>}
        </button>
      })}
    </div>}
  </div>
}

// Строки, не попадающие в импорт, — бледные, как в обычной таблице.
function markExcluded(element: HTMLElement, rows: GoodsParseRow[]) {
  element.querySelectorAll('.jss_content tbody tr, .jexcel_content tbody tr').forEach((tr, index) => {
    tr.classList.toggle('xp-excluded', rows[index] ? !rows[index].included : false)
  })
}
