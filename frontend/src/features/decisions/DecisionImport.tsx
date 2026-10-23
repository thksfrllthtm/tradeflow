import { ChangeEvent, useEffect, useMemo, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { plural } from '@/shared/lib/plural'
import { resizePhoto } from '@/shared/lib/resizePhoto'
import { goodsApi } from '@/shared/api/goodsApi'
import { GoodsParseResult } from '@/shared/types/goods'
import { DecisionInput, decisionsApi } from './decisionsApi'
import '@/shared/styles/drawer.css'

// Шаблон «База решений.xlsx» (его собирает сборщик в папке «_сборка базы решений»): лист «База»,
// заголовки в первой строке, фото в ячейке строки. Колонки узнаются по названию, порядок не важен.
const template = {
  name: 'Наименование из инвойса',
  code: 'Код ТН ВЭД',
  price: 'Цена за штуку, USD',
  certificates: 'СС / ДС',
  invoiceFile: 'Файл инвойса',
  photoFile: 'Файл с фото',
  note: 'Примечание',
} as const
type Column = keyof typeof template
const required: Column[] = ['name', 'code']
const templateSheet = 'База'

const tenDigits = /^\d{10}$/
const hasHan = (value: string) => /\p{Script=Han}/u.test(value)
const normalizeHeader = (value: string) => value.toLowerCase().replace(/ё/g, 'е').replace(/[^\p{L}\p{N}]+/gu, '')
const normalizeName = (value: string) => value.toLowerCase().replace(/ё/g, 'е').replace(/[^\p{L}\p{N}]+/gu, ' ').trim()
const normalizeCode = (value: string) => String(value ?? '').replace(/[\s.]/g, '').replace(/^'/, '').replace(/\.0+$/, '')
// «Лопата штыковая / 尖锹» — китайская часть после косой черты, если в ней иероглифы.
function chinesePart(value: string) {
  const index = value.lastIndexOf('/')
  const right = index >= 0 ? value.slice(index + 1).trim() : ''
  return right && hasHan(right) ? right : ''
}

// Колонки файла по заголовкам шаблона; отсутствующие — ''.
function matchColumns(headers: string[]) {
  const found = {} as Record<Column, string>
  for (const column of Object.keys(template) as Column[]) {
    const wanted = normalizeHeader(template[column])
    found[column] = headers.find((header) => normalizeHeader(header) === wanted) ?? ''
  }
  return found
}

const chunkSize = 25

type ImportRow = { sourceRow: number; invoiceName: string; supplierName: string; code: string; unitPriceUsd: string; note: string }

// Импорт базы решений из Excel по шаблону: строки сразу «Проверено», фото берутся из файла (тот же
// разбор Excel, что в сметах) и уменьшаются в браузере. С заменой — текущая база удаляется перед
// первой порцией, дальше порции идут как обычно.
export function DecisionImport({ onClose, onImported }: { onClose: () => void; onImported: (created: number, skipped: number, replaced: boolean) => void }) {
  const [file, setFile] = useState<File | null>(null)
  const [parsed, setParsed] = useState<GoodsParseResult | null>(null)
  const [columns, setColumns] = useState<Record<Column, string> | null>(null)
  const [replace, setReplace] = useState(false)
  const [busy, setBusy] = useState(false)
  const [progress, setProgress] = useState('')
  const [error, setError] = useState('')
  const [problems, setProblems] = useState<{ row: number; message: string }[]>([])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => { if (event.key === 'Escape' && !busy) onClose() }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose, busy])

  async function read(target: File) {
    setBusy(true); setError(''); setParsed(null); setColumns(null); setProgress('Читаем файл (большой файл с фото — до минуты)…')
    try {
      let result = await goodsApi.parseXlsx(target, { headerRow: 1, dataStartRow: 2, shareImages: true })
      // Лист «База» не первый — перечитываем его.
      if (result.sheets.length > 1 && result.sheets[0]?.name !== templateSheet && result.sheets.some((item) => item.name === templateSheet)) {
        result = await goodsApi.parseXlsx(target, { sheet: templateSheet, headerRow: 1, dataStartRow: 2, shareImages: true })
      }
      const found = matchColumns(result.headers)
      const missing = required.filter((column) => !found[column]).map((column) => `«${template[column]}»`)
      if (missing.length > 0) {
        setError(`Файл не по шаблону: в первой строке листа «${templateSheet}» нет ${missing.join(', ')}. Нужны колонки: ${Object.values(template).map((title) => `«${title}»`).join(', ')}, фото — в ячейке строки.`)
        return
      }
      setParsed(result); setColumns(found)
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
    void read(target)
  }

  // Строки к импорту: с 10-значным кодом и наименованием. При добавлении повторы внутри файла (то же
  // наименование, код и то же фото) склеиваются; при замене файл загружается как есть — повторы уже
  // свернул сборщик. Цена, СС/ДС, файлы и примечание — в примечание записи.
  const { rows, duplicates } = useMemo(() => {
    if (!parsed || !columns) return { rows: [] as ImportRow[], duplicates: 0 }
    const seen = new Set<string>()
    let repeated = 0
    const result: ImportRow[] = []
    for (const row of parsed.rows) {
      const value = (column: Column) => columns[column] ? String(row.values[columns[column]] ?? '').replace(/\s+/g, ' ').trim() : ''
      const invoiceName = value('name')
      const code = normalizeCode(value('code'))
      if (!tenDigits.test(code) || !invoiceName) continue
      if (!replace) {
        const photo = parsed.images.find((image) => image.sourceRow === row.sourceRow)?.data ?? ''
        const key = [code, normalizeName(invoiceName), photo].join('|')
        if (seen.has(key)) { repeated++; continue }
        seen.add(key)
      }
      const note = [
        value('certificates') && `СС / ДС: ${value('certificates')}`,
        value('invoiceFile') && `Файл инвойса: ${value('invoiceFile')}`,
        value('photoFile') && `Файл с фото: ${value('photoFile')}`,
        value('note'),
        `Импорт: ${file?.name ?? 'Excel'}, строка ${row.sourceRow}`,
      ].filter(Boolean).join('\n')
      result.push({ sourceRow: row.sourceRow, invoiceName, supplierName: chinesePart(invoiceName), code, unitPriceUsd: value('price').replace(',', '.'), note })
    }
    return { rows: result, duplicates: repeated }
  }, [parsed, columns, file, replace])
  const withPhotos = rows.filter((row) => parsed?.images.some((image) => image.sourceRow === row.sourceRow)).length

  async function runImport() {
    if (!parsed || rows.length === 0) return
    if (replace && !window.confirm('Удалить все записи базы решений и загрузить файл заново? Отменить это нельзя.')) return
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
          inputs.push({ invoiceName: row.invoiceName, supplierName: row.supplierName, code: row.code, unitPriceUsd: row.unitPriceUsd, photos, thumb, note: row.note })
        }
        const result = await decisionsApi.importRows(inputs, { replace: replace && start === 0, keepDuplicates: replace })
        created += result.created
        skipped += result.skipped
        found.push(...result.errors.map((item) => ({ row: chunk[item.row - 1]?.sourceRow ?? item.row, message: item.message })))
      }
      setProblems(found)
      onImported(created, skipped, replace)
      if (found.length === 0) onClose()
      else setProgress(`Добавлено ${created}; строки с ошибками — ниже`)
    } catch (reason) {
      setError(`${errorMessage(reason)}. Добавлено до ошибки: ${created}`)
      if (created || replace) onImported(created, skipped, replace)
    } finally {
      setBusy(false)
    }
  }

  return <div className="drawer-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget && !busy) onClose() }}>
    <aside className="drawer" role="dialog" aria-modal="true" aria-labelledby="dc-import-title">
      <header className="drawer-head">
        <div><h2 id="dc-import-title">Импорт базы решений</h2><p>Файл «База решений.xlsx» из сборщика: лист «{templateSheet}», колонки {Object.values(template).map((title) => `«${title}»`).join(', ')}, фото в ячейке строки. Строки попадут в базу сразу как «Проверено».</p></div>
        <button type="button" className="drawer-close" onClick={onClose} disabled={busy} aria-label="Закрыть">×</button>
      </header>
      <div className="drawer-body">
        <section className="drawer-section">
          <h3>Файл</h3>
          <label className="dc-file">{file ? file.name : 'Выбрать Excel (.xlsx)'}<input type="file" accept=".xlsx" onChange={pick} disabled={busy} /></label>
          {parsed && columns && <p className="drawer-hint">
            К импорту {rows.length} {plural(rows.length, 'строка', 'строки', 'строк')} с 10-значным кодом, с фото — {withPhotos}{duplicates ? `; повторов в файле склеено: ${duplicates}` : ''}.
            {' '}Строки без кода или без наименования пропускаются{replace ? '; остальное загружается как есть, без поиска повторов' : ', уже имеющиеся в базе — тоже'}.
            {(['price', 'certificates', 'invoiceFile', 'photoFile', 'note'] as Column[]).filter((column) => !columns[column]).map((column) => ` Колонки «${template[column]}» в файле нет.`).join('')}
          </p>}
        </section>
        {parsed && <section className="drawer-section">
          <h3>Режим</h3>
          <label className="drawer-field dc-check"><input type="checkbox" checked={replace} onChange={(event) => setReplace(event.target.checked)} disabled={busy} /><span>Заменить базу: удалить все текущие записи и загрузить файл заново</span></label>
          {replace && <p className="dc-error" role="alert">Все записи базы решений будут удалены, включая добавленные вручную и из подборов ИИ. Описания фото для поиска похожих ИИ составит заново (это платные запросы).</p>}
        </section>}
        {progress && <p className="drawer-hint">{progress}</p>}
        {error && <p className="dc-error" role="alert">{error}</p>}
        {problems.length > 0 && <ul className="dc-problems">{problems.map((item) => <li key={`${item.row}-${item.message}`}><b>Строка {item.row}</b> {item.message}</li>)}</ul>}
      </div>
      <footer className="drawer-foot">
        <button type="button" onClick={() => void runImport()} disabled={busy || rows.length === 0}>{busy ? 'Загружаем…' : replace ? `Заменить базу (${rows.length})` : `Импортировать ${rows.length || ''}`.trim()}</button>
        <button type="button" className="secondary" onClick={onClose} disabled={busy}>Отмена</button>
      </footer>
    </aside>
  </div>
}
