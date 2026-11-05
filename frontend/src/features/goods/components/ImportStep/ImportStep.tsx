import { ChangeEvent, ReactNode, useEffect, useRef, useState } from 'react'
import { GoodsParseRow, SheetInfo } from '@/shared/types/goods'
import { GoodsMapping } from '../../types'
import '@/shared/styles/list-page.css'
import '../../goods.css'
import '../ReviewStep/ReviewStep.css'
import './ImportStep.css'
import { ExcelPreview } from './ExcelPreview'

type MappingOption = { value: keyof GoodsMapping; label: string }

type Props = {
  // Название сметы и этапы — строка заголовка, как на этапах проверки и результата.
  header: ReactNode
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

// Подписи полей — короткие, те же, что в заголовках таблицы на этапе проверки.
const shortLabel: Record<keyof GoodsMapping, string> = { name: 'Наименование', name2: 'Наименование 2', name3: 'Наименование 3', code: 'Код ТН ВЭД', pairs: 'Кол-во, шт / пар', places: 'Места', pairsPerPlace: 'Шт. / пар в месте', grossKg: 'Брутто, кг', netKg: 'Нетто, кг', pricePerPair: 'Цена, USD' }

// Этап 1 «Файл и колонки» — в одном стиле с этапами проверки и результата (классы rv-*): строка заголовка,
// панель файла (лист, строки заголовков и товаров, «Применить») и лист «как в Excel» до низа окна. Колонке
// назначают поле щелчком по её заголовку — назначенные видны в заголовках («Наименование ← Колонка 3»).
export function ImportStep({ header, loading, sheets, sheet, headerRow, dataStartRow, headers, rawRows, mapping, fileName, selectedFileAvailable, mappingFieldOptions, upload, addEmptyRow, setSheet, setHeaderRow, setDataStartRow, reparseSelectedSheet, mappedFieldForHeader, setMappingForHeader, setRawRowIncluded, applyMapping }: Props) {
  const fileInput = <input type="file" accept=".xlsx,.xls" onChange={upload} disabled={loading} />
  const titlebar = <div className="estimate-bar rv-titlebar">{header}</div>

  // Файла ещё нет — строка заголовка (название сметы и этапы: видно, что это шаг 1 из 3) и зона загрузки.
  if (headers.length === 0 && sheets.length === 0) return <div className="goods-step-page rv-step im-step">
    {titlebar}
    <label className={loading ? 'im-drop busy' : 'im-drop'}>
      {fileInput}
      <svg width="28" height="28" viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M12 15V4M7.5 8.5 12 4l4.5 4.5M5 15v3.5A1.5 1.5 0 0 0 6.5 20h11a1.5 1.5 0 0 0 1.5-1.5V15" /></svg>
      <strong>{loading ? 'Читаем файл…' : 'Перетащите Excel (XLSX или XLS) сюда или нажмите, чтобы выбрать файл'}</strong>
      <span>Файл до 100 МБ. Фото из XLSX сжимаются и сохраняются только в этой смете; из старого XLS фото не читаются.</span>
    </label>
    <p className="im-alt">или <button type="button" className="button-link" onClick={addEmptyRow}>заполните строки вручную</button></p>
  </div>

  const includedCount = rawRows.filter((row) => row.included).length
  const fields = mappingFieldOptions.map((option) => ({ value: option.value, label: shortLabel[option.value] }))

  return <div className="goods-step-page rv-step im-step">
    {titlebar}

    {/* Панель файла: слева — откуда читаем, справа — сколько строк и «Применить». */}
    <div className="rv-toolbar im-toolbar">
      <SourceSettings loading={loading} fileName={fileName} fileInput={fileInput} sheets={sheets} sheet={sheet} headerRow={headerRow} dataStartRow={dataStartRow}
        selectedFileAvailable={selectedFileAvailable} setSheet={setSheet} setHeaderRow={setHeaderRow} setDataStartRow={setDataStartRow} reparse={reparseSelectedSheet} />
      <div className="rv-title-actions">
        <span className="rv-save-state im-count">К импорту <b>{includedCount}</b> из {rawRows.length} строк</span>
        {/* Без колонки «Наименование» строки не применить — подсказка рядом со счётчиком. */}
        {!mapping.name && <span className="rv-save-state unsaved" title="Нажмите на заголовок колонки с названиями товаров и выберите «Наименование»">Выберите колонку «Наименование»</span>}
        <label className="secondary im-replace">{loading ? 'Обработка…' : 'Заменить файл'}{fileInput}</label>
        <button type="button" className="secondary" onClick={addEmptyRow}>+ Строка вручную</button>
        <button type="button" onClick={applyMapping} disabled={loading || includedCount === 0}>Применить и перейти к проверке <span aria-hidden="true">→</span></button>
      </div>
    </div>

    <section className="rv-card" aria-label="Строки файла">
      <ExcelPreview headers={headers} rows={rawRows} fields={fields} fieldForHeader={(column) => mappedFieldForHeader(column, mapping)} onFieldChange={(column, field) => setMappingForHeader(column, field as keyof GoodsMapping | '')} onIncludedChange={setRawRowIncluded} />
    </section>
  </div>
}

type SourceProps = {
  loading: boolean
  fileName: string
  fileInput: ReactNode
  sheets: SheetInfo[]
  sheet: string
  headerRow: number
  // Без строки товаров (керамика: товары сразу под заголовками) — поля «Товары со строки» нет.
  dataStartRow?: number
  selectedFileAvailable: boolean
  setSheet: (value: string) => void
  setHeaderRow: (value: number) => void
  setDataStartRow?: (value: number) => void
  reparse: () => void
}

// Откуда читаются строки — как «Параметры расчёта» на этапе проверки: сводка (файл, лист, строки заголовков
// и товаров) и кнопка «Изменить», под ней — список: лист кнопками, номера строк и «Перечитать» (файла во
// вкладке нет — вместо неё «Выбрать файл»). Лист и строки применяются только перечитыванием файла; после перезагрузки страницы файла
// в браузере нет — тогда его нужно выбрать снова.
export function SourceSettings({ loading, fileName, fileInput, sheets, sheet, headerRow, dataStartRow, selectedFileAvailable, setSheet, setHeaderRow, setDataStartRow, reparse }: SourceProps) {
  const [open, setOpen] = useState(false)
  const box = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const outside = (event: MouseEvent) => { if (!box.current?.contains(event.target as Node)) setOpen(false) }
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') setOpen(false) }
    document.addEventListener('mousedown', outside)
    document.addEventListener('keydown', escape)
    return () => { document.removeEventListener('mousedown', outside); document.removeEventListener('keydown', escape) }
  }, [open])

  const sheetRows = sheets.find((value) => value.name === sheet)?.rows
  const shown = [
    { label: 'Файл', text: fileName || 'не выбран', empty: !fileName, file: true },
    { label: 'Лист', text: sheet || 'первый', empty: !sheet },
    { label: 'Заголовки', text: `строка ${headerRow}` },
    ...(dataStartRow === undefined ? [] : [{ label: 'Товары', text: `со строки ${dataStartRow}` }]),
  ]
  const number = (value: string, min: number) => Math.max(min, Math.round(Number(value)) || min)

  return <div className="rv-params im-source" ref={box}>
    <div className="rv-params-summary">
      {shown.map((item) => <span key={item.label} className={`rv-params-item${item.empty ? ' empty' : ''}${item.file ? ' im-file' : ''}`} title={item.file ? fileName : undefined}><span>{item.label}</span><b>{item.text}</b></span>)}
      <button type="button" className={open ? 'rv-params-edit open' : 'rv-params-edit'} onClick={() => setOpen(!open)} aria-expanded={open} aria-haspopup="true">Изменить
        <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><path d="M3 4.5l3 3 3-3" /></svg>
      </button>
    </div>

    {open && <div className="rv-params-form im-source-form" role="group" aria-label="Откуда читать строки">
      <div className="rv-param im-sheets">
        <span>Лист</span>
        {sheets.length > 1
          ? <div className="dir-seg" role="radiogroup" aria-label="Лист файла">
            {sheets.map((value) => <button key={value.name} type="button" role="radio" aria-checked={value.name === sheet} className={value.name === sheet ? 'active' : undefined}
              onClick={() => setSheet(value.name)} title={`${value.rows} строк`}>{value.name}</button>)}
          </div>
          : <b className="rv-param-static">{sheet || 'первый'}{sheetRows ? <small> · {sheetRows} строк</small> : null}</b>}
      </div>
      <label className="rv-param" title="Номер строки Excel с названиями колонок">
        <span>Заголовки в строке</span>
        <input type="number" min={1} value={headerRow} onChange={(event) => setHeaderRow(number(event.target.value, 1))} />
      </label>
      {dataStartRow !== undefined && setDataStartRow && <label className="rv-param" title="Номер строки Excel, с которой начинаются товары">
        <span>Товары со строки</span>
        <input type="number" min={2} value={dataStartRow} onChange={(event) => setDataStartRow(number(event.target.value, 2))} />
      </label>}
      {selectedFileAvailable
        ? <div className="im-source-actions">
          <span>Лист и строки применятся после перечитывания</span>
          <button type="button" className="sm" onClick={() => { setOpen(false); reparse() }} disabled={loading}
            title="Прочитать файл заново с выбранным листом и строками">Перечитать</button>
        </div>
        : <div className="im-source-missing" role="status">
          <span><b>Файл не загружен в этой вкладке.</b> Чтобы перечитать, выберите его снова.</span>
          <label className="secondary sm im-replace">{loading ? 'Обработка…' : 'Выбрать файл'}{fileInput}</label>
        </div>}
    </div>}
  </div>
}
