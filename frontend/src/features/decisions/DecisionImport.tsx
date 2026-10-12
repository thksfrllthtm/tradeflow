import { ChangeEvent, useEffect, useMemo, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { plural } from '@/shared/lib/plural'
import { resizePhoto } from '@/shared/lib/resizePhoto'
import { goodsApi } from '@/shared/api/goodsApi'
import { GoodsParseResult } from '@/shared/types/goods'
import { DecisionInput, decisionsApi } from './decisionsApi'
import '@/shared/styles/drawer.css'

type Field = 'invoiceName' | 'supplierName' | 'code' | 'finalName'
const fields: { value: Field; label: string; required?: boolean }[] = [
  { value: 'code', label: 'Код ТН ВЭД (правильный)', required: true },
  { value: 'finalName', label: 'Итоговое наименование для декларации («рус / кит»)' },
  { value: 'invoiceName', label: 'Наименование из инвойса (как писал поставщик)' },
  { value: 'supplierName', label: 'Наименование поставщика (кит.)' },
]
const tenDigits = /^\d{10}$/
const hasHan = (value: string) => /\p{Script=Han}/u.test(value)
const normalizeName = (value: string) => value.toLowerCase().replace(/ё/g, 'е').replace(/[^\p{L}\p{N}]+/gu, ' ').trim()
// «Лопата штыковая / 尖锹» — китайская часть после косой черты, если в ней иероглифы.
function chinesePart(value: string) {
  const index = value.lastIndexOf('/')
  const right = index >= 0 ? value.slice(index + 1).trim() : ''
  return right && hasHan(right) ? right : ''
}

// Колонки угадываются по заголовку: администратор только проверяет.
const guesses: Record<Field, RegExp> = {
  invoiceName: /наимен|товар|описан|name|品名/i,
  code: /тн\s*вэд|код|hs|商品代码|编码/i,
  supplierName: /кит|chinese|中文|供应/i,
  finalName: /итог|деклар|станд/i,
}

const chunkSize = 25
const normalizeCode = (value: string) => String(value ?? '').replace(/[\s.]/g, '').replace(/^'/, '').replace(/\.0+$/, '')

// Импорт старых инвойсов с правильными кодами в базу решений: строки сразу «Проверено».
// Фото строк берутся из файла (тот же разбор Excel, что в сметах) и уменьшаются в браузере.
export function DecisionImport({ onClose, onImported }: { onClose: () => void; onImported: (created: number, skipped: number) => void }) {
  const [file, setFile] = useState<File | null>(null)
  const [parsed, setParsed] = useState<GoodsParseResult | null>(null)
  const [sheet, setSheet] = useState('')
  const [headerRow, setHeaderRow] = useState(1)
  // В файле нет строки заголовков: первая строка — уже товар (так бывает в выгрузках наименований).
  const [noHeader, setNoHeader] = useState(false)
  const [mapping, setMapping] = useState<Record<Field, string>>({ invoiceName: '', supplierName: '', code: '', finalName: '' })
  const [busy, setBusy] = useState(false)
  const [progress, setProgress] = useState('')
  const [error, setError] = useState('')
  const [problems, setProblems] = useState<{ row: number; message: string }[]>([])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => { if (event.key === 'Escape' && !busy) onClose() }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose, busy])

  async function read(target: File, options: { sheet?: string; headerRow?: number } = {}) {
    setBusy(true); setError(''); setProgress('Читаем файл…')
    try {
      const result = await goodsApi.parseXlsx(target, { sheet: options.sheet, headerRow: options.headerRow, dataStartRow: (options.headerRow ?? 1) + 1, shareImages: true })
      setParsed(result)
      setSheet(options.sheet || result.sheets[0]?.name || '')
      // Заголовок, похожий на код (10 цифр), значит строки заголовков нет.
      const headerless = result.headers.some((header) => tenDigits.test(normalizeCode(header)))
      setNoHeader(headerless)
      const next = { invoiceName: '', supplierName: '', code: '', finalName: '' } as Record<Field, string>
      if (headerless) {
        next.code = result.headers.find((header) => tenDigits.test(normalizeCode(header))) ?? ''
        next.finalName = result.headers.find((header) => header !== next.code && /\p{L}/u.test(header)) ?? ''
      } else {
        for (const field of fields) next[field.value] = result.headers.find((header) => guesses[field.value].test(header) && !Object.values(next).includes(header)) ?? ''
      }
      setMapping(next)
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false); setProgress('')
    }
  }

  function pick(event: ChangeEvent<HTMLInputElement>) {
    const target = event.target.files?.[0]
    event.target.value = ''
    if (!target) return
    setFile(target); setProblems([])
    void read(target, { headerRow })
  }

  // Строки к импорту: с 10-значным кодом и наименованием; повторы внутри файла склеиваются.
  // Повтор — то же наименование, код и то же фото: у 16 «Отвертка / 螺丝刀» с разными фото — 16 записей.
  const { rows, duplicates } = useMemo(() => {
    if (!parsed || !mapping.code || !(mapping.invoiceName || mapping.finalName)) return { rows: [], duplicates: 0 }
    const source = noHeader ? [{ sourceRow: headerRow, values: Object.fromEntries(parsed.headers.map((header) => [header, header])) as Record<string, string>, included: true }, ...parsed.rows] : parsed.rows
    const seen = new Set<string>()
    let repeated = 0
    const result: { sourceRow: number; invoiceName: string; finalName: string; supplierName: string; code: string }[] = []
    for (const row of source) {
      const value = (field: Field) => mapping[field] ? String(row.values[mapping[field]] ?? '').replace(/\s+/g, ' ').trim() : ''
      const finalName = value('finalName')
      const item = { sourceRow: row.sourceRow, invoiceName: value('invoiceName'), finalName, supplierName: value('supplierName') || chinesePart(finalName), code: normalizeCode(String(row.values[mapping.code] ?? '')) }
      if (!tenDigits.test(item.code) || !(item.invoiceName || item.finalName || item.supplierName)) continue
      const photo = parsed.images.find((image) => image.sourceRow === row.sourceRow)?.data ?? ''
      const key = [item.code, normalizeName(item.finalName), normalizeName(item.invoiceName), normalizeName(item.supplierName), photo].join('|')
      if (seen.has(key)) { repeated++; continue }
      seen.add(key)
      result.push(item)
    }
    return { rows: result, duplicates: repeated }
  }, [parsed, mapping, noHeader, headerRow])
  const withPhotos = rows.filter((row) => parsed?.images.some((image) => image.sourceRow === row.sourceRow)).length

  async function runImport() {
    if (!parsed || rows.length === 0) return
    setBusy(true); setError(''); setProblems([])
    let created = 0
    let skipped = 0
    const found: { row: number; message: string }[] = []
    try {
      for (let start = 0; start < rows.length; start += chunkSize) {
        setProgress(`Загружаем ${Math.min(start + chunkSize, rows.length)} из ${rows.length}…`)
        const chunk = rows.slice(start, start + chunkSize)
        const inputs: DecisionInput[] = []
        for (const row of chunk) {
          const images = parsed.images.filter((image) => image.sourceRow === row.sourceRow && image.data).slice(0, 3)
          const sources = images.map((image) => `data:${image.mimeType};base64,${image.data}`)
          const photos = await Promise.all(sources.map((source) => resizePhoto(source)))
          const thumb = sources[0] ? await resizePhoto(sources[0], 160, 0.7) : ''
          inputs.push({ invoiceName: row.invoiceName, supplierName: row.supplierName, finalName: row.finalName, code: row.code, photos, thumb, note: `Импорт: ${file?.name ?? 'Excel'}, строка ${row.sourceRow}` })
        }
        const result = await decisionsApi.importRows(inputs)
        created += result.created
        skipped += result.skipped
        found.push(...result.errors.map((item) => ({ row: chunk[item.row - 1]?.sourceRow ?? item.row, message: item.message })))
      }
      setProblems(found)
      onImported(created, skipped)
      if (found.length === 0) onClose()
      else setProgress(`Добавлено ${created}; строки с ошибками — ниже`)
    } catch (reason) {
      setError(`${errorMessage(reason)}. Добавлено до ошибки: ${created}`)
      if (created) onImported(created, skipped)
    } finally {
      setBusy(false)
    }
  }

  return <div className="drawer-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget && !busy) onClose() }}>
    <aside className="drawer" role="dialog" aria-modal="true" aria-labelledby="dc-import-title">
      <header className="drawer-head">
        <div><h2 id="dc-import-title">Импорт старого инвойса</h2><p>Строки с правильными кодами попадут в базу сразу как «Проверено». Фото берутся из файла.</p></div>
        <button type="button" className="drawer-close" onClick={onClose} disabled={busy} aria-label="Закрыть">×</button>
      </header>
      <div className="drawer-body">
        <section className="drawer-section">
          <h3>Файл</h3>
          <label className="dc-file">{file ? file.name : 'Выбрать Excel (.xlsx)'}<input type="file" accept=".xlsx" onChange={pick} disabled={busy} /></label>
          {parsed && <div className="drawer-row">
            <label className="drawer-field"><span>Лист</span><select value={sheet} onChange={(event) => { setSheet(event.target.value); if (file) void read(file, { sheet: event.target.value, headerRow }) }} disabled={busy}>{parsed.sheets.map((item) => <option key={item.name} value={item.name}>{item.name} ({item.rows} строк)</option>)}</select></label>
            <label className="drawer-field dc-check"><input type="checkbox" checked={noHeader} onChange={(event) => setNoHeader(event.target.checked)} disabled={busy} /><span>Заголовков нет — первая строка уже товар</span></label>
            <label className="drawer-field"><span>{noHeader ? 'Первая строка' : 'Заголовки в строке'}</span><input type="number" min={1} value={headerRow} onChange={(event) => setHeaderRow(Math.max(1, Number(event.target.value) || 1))} onBlur={() => { if (file) void read(file, { sheet, headerRow }) }} disabled={busy} /></label>
          </div>}
        </section>
        {parsed && <section className="drawer-section">
          <h3>Колонки</h3>
          {fields.map((field) => <label key={field.value} className="drawer-field">
            <span>{field.label}{field.required && <b className="req">*</b>}</span>
            <select value={mapping[field.value]} onChange={(event) => setMapping((current) => ({ ...current, [field.value]: event.target.value }))} disabled={busy}>
              <option value="">— не выбрана —</option>
              {parsed.headers.map((header, index) => <option key={header} value={header}>{noHeader ? `Колонка ${String.fromCharCode(65 + index)}: ${header.slice(0, 50)}` : header}</option>)}
            </select>
          </label>)}
          <p className="drawer-hint">{mapping.code && (mapping.invoiceName || mapping.finalName)
            ? `К импорту ${rows.length} ${plural(rows.length, 'строка', 'строки', 'строк')} с 10-значным кодом, с фото — ${withPhotos}${duplicates ? `; повторов в файле склеено: ${duplicates}` : ''}. Строки без кода или без наименования пропускаются, уже имеющиеся в базе — тоже.`
            : 'Выберите колонку кода и колонку наименования (итогового или из инвойса).'}
            {mapping.finalName && !mapping.supplierName && ' Китайская часть после «/» станет наименованием поставщика.'}</p>
        </section>}
        {progress && <p className="drawer-hint">{progress}</p>}
        {error && <p className="dc-error" role="alert">{error}</p>}
        {problems.length > 0 && <ul className="dc-problems">{problems.map((item) => <li key={`${item.row}-${item.message}`}><b>Строка {item.row}</b> {item.message}</li>)}</ul>}
      </div>
      <footer className="drawer-foot">
        <button type="button" onClick={() => void runImport()} disabled={busy || rows.length === 0}>{busy ? 'Загружаем…' : `Импортировать ${rows.length || ''}`.trim()}</button>
        <button type="button" className="secondary" onClick={onClose} disabled={busy}>Отмена</button>
      </footer>
    </aside>
  </div>
}
