import { ChangeEvent, useState } from 'react'
import { GoodsParseRow, SheetInfo } from '@/shared/types/goods'
import { GoodsMapping } from '../../types'
import '../../goods.css'
import './ImportStep.css'
import { ExcelPreview } from './ExcelPreview'
import { ColumnFieldMenu, ColumnMenuAnchor } from './ColumnFieldMenu'

type MappingOption = { value: keyof GoodsMapping; label: string }

type Props = {
  loading: boolean
  sheets: SheetInfo[]
  sheet: string
  headerRow: number
  dataStartRow: number
  headers: string[]
  rawRows: GoodsParseRow[]
  mapping: GoodsMapping
  fileName: string
  selectedFileAvailable: boolean
  mappingFieldOptions: MappingOption[]
  upload: (event: ChangeEvent<HTMLInputElement>) => void
  addEmptyRow: () => void
  setSheet: (value: string) => void
  setHeaderRow: (value: number) => void
  setDataStartRow: (value: number) => void
  reparseSelectedSheet: () => void
  mappedFieldForHeader: (header: string, mapping: GoodsMapping) => keyof GoodsMapping | ''
  setMappingForHeader: (header: string, value: keyof GoodsMapping | '') => void
  setRawRowIncluded: (sourceRow: number, included: boolean) => void
  applyMapping: () => void
}

// Вид таблицы строк файла: обычная или пробный «Как в Excel» (Jspreadsheet CE). Выбор помнит браузер.
type View = 'table' | 'excel'
const viewKey = 'tradeflow.importView'
function storedView(): View {
  try { return localStorage.getItem(viewKey) === 'excel' ? 'excel' : 'table' } catch { return 'table' }
}

// Подписи полей — короткие, те же, что в заголовках таблицы на этапе проверки.
const shortLabel: Record<keyof GoodsMapping, string> = { name: 'Наименование', name2: 'Наименование 2', name3: 'Наименование 3', code: 'Код ТН ВЭД', pairs: 'Кол-во, шт', places: 'Места', pairsPerPlace: 'Шт. в месте', grossKg: 'Брутто, кг', netKg: 'Нетто, кг', pricePerPair: 'Цена, USD' }

export function ImportStep({ loading, sheets, sheet, headerRow, dataStartRow, headers, rawRows, mapping, fileName, selectedFileAvailable, mappingFieldOptions, upload, addEmptyRow, setSheet, setHeaderRow, setDataStartRow, reparseSelectedSheet, mappedFieldForHeader, setMappingForHeader, setRawRowIncluded, applyMapping }: Props) {
  const fileInput = <input type="file" accept=".xlsx" onChange={upload} disabled={loading} />
  const [view, setViewState] = useState<View>(storedView)
  const [menu, setMenu] = useState<ColumnMenuAnchor | null>(null)
  function setView(next: View) {
    setViewState(next)
    try { localStorage.setItem(viewKey, next) } catch { /* выбор просто не запомнится */ }
  }

  if (headers.length === 0 && sheets.length === 0) return <div className="goods-step-page im-step">
    <label className={loading ? 'im-drop busy' : 'im-drop'}>
      {fileInput}
      <strong>{loading ? 'Читаем файл…' : 'Перетащите XLSX сюда или нажмите, чтобы выбрать файл'}</strong>
      <span>Обычный XLSX до 100 МБ. Фото из файла сжимаются и сохраняются только в этой смете.</span>
    </label>
    <p className="im-alt">или <button type="button" className="button-link" onClick={addEmptyRow}>заполните строки вручную</button></p>
  </div>

  const includedCount = rawRows.filter((row) => row.included).length
  const mappedCount = mappingFieldOptions.filter((option) => mapping[option.value]).length
  const fieldByHeader = headers.map((header) => mappedFieldForHeader(header, mapping))
  const fields = mappingFieldOptions.map((option) => ({ value: option.value, label: shortLabel[option.value] }))
  const menuChosen = menu ? mappedFieldForHeader(menu.header, mapping) : ''
  const holder = (field: string) => headers.find((header) => header !== menu?.header && mappedFieldForHeader(header, mapping) === field)
  function toggleMenu(header: string, button: HTMLElement) {
    const rect = button.getBoundingClientRect()
    setMenu((open) => open?.header === header ? null : { header, left: rect.left, top: rect.bottom, width: rect.width })
  }
  return <div className="goods-step-page im-step">
    <section className="rv-card">
      <div className="im-toolbar">
        <label className="im-file-button">{loading ? 'Обработка…' : 'Заменить файл'}{fileInput}</label>
        {fileName && <span className="im-file-name" title={fileName}>{fileName}</span>}
        <div className="im-settings">
          <label className="im-field">Лист<select value={sheet} onChange={(event) => setSheet(event.target.value)}>{sheets.length ? sheets.map((value) => <option key={value.name} value={value.name}>{value.name} ({value.rows} строк)</option>) : <option value="">Первый лист</option>}</select></label>
          <label className="im-field" title="Номер строки Excel с названиями колонок">Заголовки в строке<input type="number" min={1} value={headerRow} onChange={(event) => setHeaderRow(Math.max(1, Number(event.target.value) || 1))} /></label>
          <label className="im-field" title="Номер строки Excel, с которой начинаются товары">Товары со строки<input type="number" min={2} value={dataStartRow} onChange={(event) => setDataStartRow(Math.max(2, Number(event.target.value) || 2))} /></label>
          <button type="button" className="rv-bar-button" onClick={reparseSelectedSheet} disabled={loading || !selectedFileAvailable} title="Перечитать файл с выбранным листом и строками">Перечитать</button>
        </div>
      </div>
      <div className="im-mapping">
        <span className="im-mapping-title">Колонки файла:</span>
        <div className="im-view" role="radiogroup" aria-label="Вид таблицы">
          <button type="button" role="radio" aria-checked={view === 'table'} className={view === 'table' ? 'active' : undefined} onClick={() => setView('table')}>Таблица</button>
          <button type="button" role="radio" aria-checked={view === 'excel'} className={view === 'excel' ? 'active' : undefined} onClick={() => setView('excel')} title="Пробный режим: выделение ячеек мышью и копирование в Excel">Как в Excel</button>
        </div>
        {mappingFieldOptions.map((option) => <span key={option.value} className={mapping[option.value] ? 'im-chip set' : 'im-chip'} title={option.value === 'code' ? 'Если указать колонку с кодом, коды сверятся с номенклатурой ТН ВЭД ЕАЭС и строки получат её ставки' : option.value === 'name2' || option.value === 'name3' ? 'Ещё одна колонка с наименованием (например, китайское и русское): подбор кода и поиск в базе решений используют все наименования' : option.label}>
          {shortLabel[option.value]}{mapping[option.value] ? <b>{mapping[option.value]}</b> : <i>не выбрана</i>}
        </span>)}
      </div>
      {view === 'excel' ? <>
        <p className="im-xp-hint">Нажмите на заголовок колонки, чтобы выбрать, что в ней. Выделяйте ячейки мышью (с Shift — диапазон), Ctrl+C — скопировать в Excel. Флажок в первой колонке — импортировать строку.</p>
        <ExcelPreview headers={headers} rows={rawRows} fields={fields} fieldForHeader={(header) => mappedFieldForHeader(header, mapping)} onFieldChange={(header, field) => setMappingForHeader(header, field as keyof GoodsMapping | '')} onIncludedChange={setRawRowIncluded} />
      </> : <div className="im-table-wrap">
        <table className="sheet im-table">
          <thead><tr>
            <th className="im-check"><input type="checkbox" checked={rawRows.length > 0 && includedCount === rawRows.length} onChange={(event) => rawRows.forEach((row) => setRawRowIncluded(row.sourceRow, event.target.checked))} aria-label="Импортировать все строки" title="Импортировать все строки" /></th>
            <th className="im-row" title="Номер строки в Excel">Стр.</th>
            {headers.map((header, index) => <th key={header} className={fieldByHeader[index] ? 'im-mapped' : ''}>
              <span className="im-col-name" title={header}>{header}</span>
              <button type="button" className="im-pick" aria-haspopup="listbox" aria-expanded={menu?.header === header} aria-label={`Что в колонке ${header}`} onClick={(event) => toggleMenu(header, event.currentTarget)}>
                <span>{fieldByHeader[index] ? shortLabel[fieldByHeader[index] as keyof GoodsMapping] : 'Не использовать'}</span><i aria-hidden="true">▾</i>
              </button>
            </th>)}
          </tr></thead>
          <tbody>{rawRows.map((row) => <tr key={row.sourceRow} className={row.included ? '' : 'im-excluded'}>
            <td className="im-check"><input type="checkbox" checked={row.included} onChange={(event) => setRawRowIncluded(row.sourceRow, event.target.checked)} aria-label={`Импортировать строку ${row.sourceRow}`} /></td>
            <td className={row.warnings?.length ? 'im-row im-warn' : 'im-row'} title={row.warnings?.join('\n') || undefined}>{row.sourceRow}</td>
            {headers.map((header, index) => <td key={header} className={fieldByHeader[index] ? 'im-mapped' : ''} title={row.values[header] || undefined}>{row.values[header]}</td>)}
          </tr>)}</tbody>
        </table>
        {menu && <ColumnFieldMenu anchor={menu} fields={fields} chosen={menuChosen} holder={holder} ignoreSelector=".im-table th .im-pick"
          onChoose={(field) => { setMappingForHeader(menu.header, field as keyof GoodsMapping | ''); setMenu(null) }} onClose={() => setMenu(null)} />}
      </div>}
      <div className="rv-actions">
        <span className="rv-actions-note">К импорту <b>{includedCount}</b> из {rawRows.length} строк · колонок выбрано <b>{mappedCount}</b> из {mappingFieldOptions.length}{!mapping.name && <em className="im-need"> · выберите колонку «Наименование»</em>}</span>
        <button type="button" className="rv-clear im-manual" onClick={addEmptyRow}>+ Строка вручную</button>
        <button type="button" className="btn-calculate" onClick={applyMapping} disabled={loading || includedCount === 0}>Применить и перейти к проверке →</button>
      </div>
    </section>
  </div>
}
