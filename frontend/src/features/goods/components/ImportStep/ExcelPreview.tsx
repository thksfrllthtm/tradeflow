import { useEffect, useRef, useState } from 'react'
import type jspreadsheet from 'jspreadsheet-ce'
import { GoodsParseRow } from '@/shared/types/goods'
import { ColumnField, ColumnFieldMenu, ColumnMenuAnchor } from './ColumnFieldMenu'
import { spaceBelow } from '../../lib/spaceBelow'
import './ExcelPreview.css'

type Props = {
  headers: string[]
  rows: GoodsParseRow[]
  // Поля сметы, которые можно назначить колонке, и текущее назначение колонки ('' — не используется).
  fields: ColumnField[]
  fieldForHeader: (header: string) => string
  onFieldChange: (header: string, field: string) => void
  onIncludedChange: (sourceRow: number, included: boolean) => void
}

// Колонки перед колонками файла: флажок импорта и номер строки Excel.
const leadColumns = 2

// Лист «Как в Excel» на шаге «Файл и колонки»: строки файла в Jspreadsheet CE (MIT).
// Ячейки только для чтения, но их можно выделять мышью и копировать (Ctrl+C) прямо в Excel;
// первая колонка — флажок импорта строки, клик по заголовку колонки файла открывает список
// полей сметы. Формулы из файла не вычисляются. Библиотека загружается только здесь.
export function ExcelPreview({ headers, rows, fields, fieldForHeader, onFieldChange, onIncludedChange }: Props) {
  const host = useRef<HTMLDivElement>(null)
  const sheet = useRef<jspreadsheet.WorksheetInstance | null>(null)
  const rowsRef = useRef(rows)
  const changeRef = useRef(onIncludedChange)
  const [error, setError] = useState('')
  const [menu, setMenu] = useState<ColumnMenuAnchor | null>(null)
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
    // Лист — до низа окна, как на этапе проверки: --xp-top — где он начинается, --xp-below — что под ним до
    // конца сметы (подпись, отступ). Перемер — при изменении высоты страницы сметы и окна.
    const measure = () => {
      const box = element.getBoundingClientRect()
      element.style.setProperty('--xp-top', `${box.top + window.scrollY}px`)
      element.style.setProperty('--xp-below', `${spaceBelow(element)}px`)
    }
    const resized = new ResizeObserver(measure)
    const pageElement = element.closest('.goods-page')
    if (pageElement) resized.observe(pageElement)
    window.addEventListener('resize', measure)
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
              { type: 'checkbox', title: '✓', width: 32 },
              { type: 'text', title: 'Стр.', width: 48, readOnly: true, align: 'right' },
              ...headers.map((_header, index) => ({ type: 'text' as const, title: titles[index], width: 160, readOnly: true, align: 'left' as const })),
            ],
            // Прокрутка — у .xp-host (высота до низа окна, ниже), а не у библиотеки: так шапка прилипает к верху листа.
            tableOverflow: false,
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
        measure()
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
      resized.disconnect()
      window.removeEventListener('resize', measure)
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

  const chosen = menu ? fieldForHeader(menu.header) : ''
  // Колонка, у которой поле уже назначено: выбор здесь переносит его сюда.
  const holder = (field: string) => headers.find((header) => header !== menu?.header && fieldForHeader(header) === field)
  return <div className="xp-wrap">
    {error && <p className="xp-error">Не удалось открыть таблицу: {error}. Обновите страницу.</p>}
    <div ref={host} className="xp-host" />
    {menu && <ColumnFieldMenu anchor={menu} fields={fields} chosen={chosen} holder={holder} ignoreSelector=".xp-host thead td.xp-pick"
      onChoose={(field) => { onFieldChange(menu.header, field); setMenu(null) }} onClose={() => setMenu(null)} />}
  </div>
}

// Строки, не попадающие в импорт, — бледные, как в обычной таблице.
function markExcluded(element: HTMLElement, rows: GoodsParseRow[]) {
  element.querySelectorAll('.jss_content tbody tr, .jexcel_content tbody tr').forEach((tr, index) => {
    tr.classList.toggle('xp-excluded', rows[index] ? !rows[index].included : false)
  })
}
