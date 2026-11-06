import { ClipboardEvent, KeyboardEvent, useEffect, useRef, useState } from 'react'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatMoney, formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { excelColumn, excelColumnHtml } from '@/shared/lib/excelColumn'
import { CopyColumn } from '@/shared/components/CopyCode/CopyColumn'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { UtilFeeMaterial, UtilFeeRowResult, UtilFeeRowsResult, utilFeeApi } from './utilFeeApi'
import { DEFAULT_MRP, emptyRow, FEE_COLUMNS, FeeRow, isTable, pasteRows, withTail } from './feeRows'

const STORAGE_KEY = 'utilFee.table'
const payLabels = { product: 'товар', packaging: 'упаковку', vehicle: 'транспорт' } as const

type Saved = { rows: FeeRow[]; mrp: string; packagingPercent: string; material: string; productWeight: 'net' | 'gross' }
const initial: Saved = { rows: [emptyRow()], mrp: DEFAULT_MRP, packagingPercent: '20', material: 'paper', productWeight: 'net' }

// Таблица — в браузере сотрудника, чтобы не пропала при обновлении страницы (в базе не храним).
function load(): Saved {
  try {
    const saved = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? 'null') as Saved | null
    return saved?.rows ? { ...initial, ...saved, rows: withTail(saved.rows) } : initial
  } catch {
    return initial
  }
}

// «548.800» → «548,8»: вес без лишних нулей.
const kg = (value: string) => formatRuNumber(value.includes('.') ? value.replace(/0+$/, '').replace(/\.$/, '') : value)
const money = (value: string) => value ? `${formatRuNumber(value)} KZT` : ''
const usd = (value: string) => value ? formatMoney(value, 'USD') : ''

// Таблица утильсбора по Excel поставщика: код, брутто, нетто (пустое — брутто минус % упаковки).
// Колонку из Excel вставляют Ctrl+V в первую ячейку — строки заполняются вниз. Суммы (и в долларах — по курсу
// НБРК на сегодня) считает сервер; колонки сумм копируются кнопкой в заголовке для вставки обратно в Excel.
export function UtilFeeRows({ materials, usdRate }: { materials: UtilFeeMaterial[]; usdRate?: string }) {
  const [state, setState] = useState<Saved>(load)
  const [result, setResult] = useState<UtilFeeRowsResult | null>(null)
  const [error, setError] = useState('')
  const table = useRef<HTMLTableElement>(null)
  const debounced = useDebouncedValue(state, 400)

  useEffect(() => {
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify(debounced)) } catch { /* приватный режим — без сохранения */ }
    const rows = debounced.rows.slice(0, -1)
    if (rows.length === 0) { setResult(null); setError(''); return }
    let current = true
    utilFeeApi.calculateRows({ ...debounced, rows, usdRate })
      .then((next) => { if (current) { setResult(next); setError('') } })
      .catch((reason) => { if (current) setError(errorMessage(reason)) })
    return () => { current = false }
  }, [debounced, usdRate])

  const set = (change: Partial<Saved>) => setState((value) => ({ ...value, ...change }))
  const setCell = (row: number, key: keyof FeeRow, value: string) =>
    setState((current) => ({ ...current, rows: withTail(current.rows.map((item, index) => index === row ? { ...item, [key]: value } : item)) }))

  function onPaste(event: ClipboardEvent<HTMLInputElement>, row: number, column: number) {
    const text = event.clipboardData.getData('text/plain')
    if (!isTable(text)) return
    event.preventDefault()
    setState((current) => ({ ...current, rows: pasteRows(current.rows, row, column, text) }))
  }

  // Enter и стрелки вверх/вниз — по колонке, как в Excel.
  function onKeyDown(event: KeyboardEvent<HTMLInputElement>, row: number, column: number) {
    const step = event.key === 'ArrowUp' ? -1 : event.key === 'ArrowDown' || event.key === 'Enter' ? 1 : 0
    if (!step) return
    event.preventDefault()
    table.current?.querySelector<HTMLInputElement>(`[data-cell="${row + step}-${column}"]`)?.focus()
  }

  const cell = (row: number, column: number, value: string, placeholder = '') =>
    <input value={value} data-cell={`${row}-${column}`} placeholder={placeholder} inputMode={column === 0 ? 'numeric' : 'decimal'}
      onChange={(event) => setCell(row, FEE_COLUMNS[column], event.target.value)}
      onPaste={(event) => onPaste(event, row, column)} onKeyDown={(event) => onKeyDown(event, row, column)} />

  const filled = state.rows.length - 1
  // Кнопка «скопировать колонку»: суммы всех строк по порядку (пустая строка — пустая ячейка), числами для Excel.
  const copy = (label: string, get: (row: UtilFeeRowResult) => string) => {
    if (!result) return null
    const values = () => state.rows.slice(0, filled).map((_, index) => { const row = result.rows[index]; return row ? get(row) : '' })
    return <CopyColumn label={label} text={() => excelColumn(values())} html={() => excelColumnHtml(values())} />
  }
  return <section className="uf-rows" aria-label="Таблица утильсбора">
    <div className="uf-filterbar">
      <label className="uf-mrp" title="Месячный расчётный показатель. По умолчанию — на 2026 год; поменяйте, если МРП изменился">МРП, KZT<input value={state.mrp} onChange={(event) => set({ mrp: event.target.value })} inputMode="decimal" /></label>
      <label className="uf-mrp" title="Для строк без нетто: нетто = брутто минус этот процент, остальное — вес упаковки">Нет нетто — вычесть из брутто<input value={state.packagingPercent} onChange={(event) => set({ packagingPercent: event.target.value })} inputMode="decimal" /><span>%</span></label>
      <label title="Из чего упаковка — от этого зависит коэффициент k в сборе за упаковку (Методика № 448)">Упаковка из
        <select value={state.material} onChange={(event) => set({ material: event.target.value })}>
          {materials.map((material) => <option key={material.id} value={material.id}>{material.label} (k {formatRuNumber(material.k)})</option>)}
        </select>
      </label>
      <label title="От какого веса считать сбор за товар. Сбор за упаковку всегда — от брутто минус нетто">Сбор за товар от
        <select className="uf-select-short" value={state.productWeight} onChange={(event) => set({ productWeight: event.target.value as Saved['productWeight'] })}>
          <option value="net">нетто</option>
          <option value="gross">брутто</option>
        </select>
      </label>
      <span className="uf-tip" />
      {filled > 0 && <button type="button" className="danger" title="Удалить все строки таблицы"
        onClick={() => { if (window.confirm('Очистить таблицу? Все строки будут удалены.')) { set({ rows: [emptyRow()] }); setResult(null) } }}>Очистить таблицу</button>}
    </div>
    {error && <p className="banner error" role="alert">{error}</p>}
    <div className="uf-scroll">
      <table className="sheet uf-table uf-rows-table" ref={table}>
        {/* Ширины колонок — здесь, одним списком; «Примечание» забирает остаток. */}
        <colgroup>{[48, 120, 110, 150, 150, 220, 130, 130, 145, 125].map((width, index) => <col key={index} style={{ width }} />)}<col /></colgroup>
        <thead><tr>
          {/* Шапка в две строки: название (одна строка) и под ним серое пояснение — у всех колонок одинаково. */}
          <th className="c-n">№</th>
          <th className="c-in">Код ТН ВЭД<small>из Excel</small></th>
          <th className="c-in" title="Общий вес строки">Брутто<small>кг</small></th>
          <th className="c-in" title="Пусто — программа вычтет из брутто процент, указанный выше (серым)">Нетто<small>кг · <i>серое</i> — расчёт</small></th>
          <th className="c-num" title="Брутто минус нетто">Упаковка<small>кг · брутто − нетто</small></th>
          <th className="c-list" title="За что платится утильсбор и раздел перечня № 689">Платится за<small>раздел перечня</small></th>
          <th className="c-num" title={`${state.productWeight === 'gross' ? 'Брутто' : 'Нетто'} (т) × МРП × k раздела`}>{copy('За товар, KZT', (row) => row.product)}За товар<small>KZT · от {state.productWeight === 'gross' ? 'брутто' : 'нетто'}</small></th>
          <th className="c-num" title="Упаковка (т) × МРП × k материала">{copy('За упаковку, KZT', (row) => row.packaging)}За упаковку<small>KZT</small></th>
          <th className="c-num">{copy('Итого, KZT', (row) => row.total)}Итого<small>KZT</small></th>
          <th className="c-num" title="Итого в тенге по курсу НБРК на сегодня">{copy('Итого, USD', (row) => row.totalUsd)}Итого<small>USD{usdRate && ` · ${formatRuNumber(usdRate)} KZT`}</small></th>
          <th className="c-note">Примечание<small>&nbsp;</small></th>
        </tr></thead>
        <tbody>
          {state.rows.map((row, index) => {
            const out = index < filled ? result?.rows[index] : undefined
            const note = out?.error || out?.warnings.join('; ') || ''
            return <tr key={index}>
              <td className="c-n">{index + 1}</td>
              <td className="c-in c-code">{cell(index, 0, row.code, index === filled ? 'вставьте Ctrl+V' : '')}</td>
              <td className="c-in">{cell(index, 1, row.grossKg)}</td>
              <td className="c-in">{cell(index, 2, row.netKg, out?.netCalculated ? kg(out.netKg) : '')}</td>
              <td className="c-num">{out?.packagingKg ? kg(out.packagingKg) : ''}</td>
              <td className="c-list">{out && !out.error && (out.matches.length > 0
                ? out.matches.map((match) => <span key={match.item} className={`uf-pay ${match.kind}`} title={`Раздел ${match.section}: ${match.title}`}>{payLabels[match.kind]} <small>разд. {match.section}</small></span>)
                : <span className="uf-hint">{out.possible.length > 0 ? 'зависит от полного кода' : out.code ? 'не платится — кода нет в перечне' : ''}</span>)}</td>
              <td className="c-num">{money(out?.product ?? '')}</td>
              <td className="c-num">{money(out?.packaging ?? '')}</td>
              <td className="c-num">{money(out?.total ?? '')}</td>
              <td className="c-num">{usd(out?.totalUsd ?? '')}</td>
              <td className={out?.error ? 'c-note bad' : 'c-note'} title={note}>{note}</td>
            </tr>
          })}
        </tbody>
        {result && <tfoot><tr>
          <td colSpan={8}>Итого утильсбор по {filled} {filled % 10 === 1 && filled % 100 !== 11 ? 'строке' : 'строкам'}</td>
          <td className="c-num"><b><CopyCode number code={formatRuNumber(result.total)} /> KZT</b></td>
          <td className="c-num"><b>{result.totalUsd && <><CopyCode number code={formatRuNumber(result.totalUsd)} /> USD</>}</b></td>
          <td className="c-note">{!usdRate && 'Курс НБРК не загрузился — доллары не посчитаны'}</td>
        </tr></tfoot>}
      </table>
    </div>
  </section>
}
