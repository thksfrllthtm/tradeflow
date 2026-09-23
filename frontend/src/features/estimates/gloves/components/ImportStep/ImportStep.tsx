import { ChangeEvent } from 'react'
import { GlovesParseRow, SheetInfo } from '../../../shared/types'
import { GlovesMapping } from '../../types'
import '../../../shared/styles/calculatorCommon.css'
import '../../styles/calculator.css'
import './ImportStep.css'

type MappingOption = { value: keyof GlovesMapping; label: string }

type Props = {
  loading: boolean
  sheets: SheetInfo[]
  sheet: string
  headerRow: number
  dataStartRow: number
  headers: string[]
  rawRows: GlovesParseRow[]
  mapping: GlovesMapping
  selectedFileAvailable: boolean
  mappingFieldOptions: MappingOption[]
  upload: (event: ChangeEvent<HTMLInputElement>) => void
  addEmptyRow: () => void
  setSheet: (value: string) => void
  setHeaderRow: (value: number) => void
  setDataStartRow: (value: number) => void
  reparseSelectedSheet: () => void
  mappedFieldForHeader: (header: string, mapping: GlovesMapping) => keyof GlovesMapping | ''
  setMappingForHeader: (header: string, value: keyof GlovesMapping | '') => void
  setRawRowIncluded: (sourceRow: number, included: boolean) => void
  applyMapping: () => void
}

export function ImportStep({ loading, sheets, sheet, headerRow, dataStartRow, headers, rawRows, mapping, selectedFileAvailable, mappingFieldOptions, upload, addEmptyRow, setSheet, setHeaderRow, setDataStartRow, reparseSelectedSheet, mappedFieldForHeader, setMappingForHeader, setRawRowIncluded, applyMapping }: Props) {
  return <div className="gloves-step-page">
    <section className="gloves-upload-bar"><label>{loading ? 'Обработка…' : 'Загрузить XLSX'}<input type="file" accept=".xlsx" onChange={upload} disabled={loading} /></label><button type="button" onClick={addEmptyRow}>Добавить строку</button><span className="upload-info">Поддерживается обычный XLSX до 50 МБ. Фото из файла используются только в этом черновике.</span></section>
    <section className="gloves-source-settings"><label>Лист<select value={sheet} onChange={(event) => setSheet(event.target.value)}>{sheets.length ? sheets.map((value) => <option key={value.name} value={value.name}>{value.name} ({value.rows} строк)</option>) : <option value="">Первый лист после загрузки</option>}</select></label><label>Строка заголовков<input type="number" min={1} value={headerRow} onChange={(event) => setHeaderRow(Math.max(1, Number(event.target.value) || 1))} /></label><label>Первая строка товаров<input type="number" min={2} value={dataStartRow} onChange={(event) => setDataStartRow(Math.max(2, Number(event.target.value) || 2))} /></label><button type="button" onClick={reparseSelectedSheet} disabled={loading || !selectedFileAvailable}>Перечитать лист</button><span className="muted">После загрузки проверьте сопоставление в таблице и выберите карточку перчаток для каждой строки.</span></section>
    {headers.length > 0 && <section className="gloves-mapping gloves-excel-preview"><div><strong>Первые строки файла</strong><span className="muted"> В каждой колонке сразу выберите назначение. Строки, которые не нужны, снимите в колонке «Импорт».</span></div><div className="gloves-preview-scroll"><table className="gloves-preview-table"><thead><tr><th>Импорт</th>{headers.map((header) => <th key={header}><strong title={header}>{header}</strong><select value={mappedFieldForHeader(header, mapping)} onChange={(event) => setMappingForHeader(header, event.target.value as keyof GlovesMapping | '')}><option value="">Не использовать</option>{mappingFieldOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></th>)}</tr></thead><tbody>{rawRows.map((row) => <tr key={row.sourceRow} className={row.included ? '' : 'row-excluded'}><td><input type="checkbox" checked={row.included} onChange={(event) => setRawRowIncluded(row.sourceRow, event.target.checked)} aria-label={`Импортировать строку ${row.sourceRow}`} /></td>{headers.map((header) => <td key={`${row.sourceRow}-${header}`} title={row.values[header] || ''}>{row.values[header] || '—'}</td>)}</tr>)}</tbody></table></div><small className="muted">Показываются все строки выбранного диапазона. Итоги и пустые строки уже сняты автоматически, но их можно включить вручную.</small></section>}
    <div className="gloves-step-actions"><span className="muted">Выберите только нужные колонки. После применения откроется проверка строк.</span><button type="button" className="btn-calculate" onClick={applyMapping} disabled={loading || rawRows.length === 0}>Применить и перейти к проверке →</button></div>
  </div>
}
