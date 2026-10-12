import { ChangeEvent, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatDisplayNumber } from '@/shared/lib/formatDisplayNumber'
import { plural } from '@/shared/lib/plural'
import { formatRuDate } from '@/shared/lib/dates'
import { DirectoryImportResult } from '../common/directoryTransfer'
import { KzImport, KzImportResult, KzListError, kzListApi } from './kzListApi'
import { readKzPdf } from './kzListPdf'
import './KzListPanel.css'

type Busy = { action: 'excel' | 'pdf' | 'import'; text: string } | null

// «Перечень РК» (администратор): 1) Excel из PDF ЕЭК — проверить и при нужде поправить;
// 2) загрузить этот Excel; или сразу загрузить PDF. button — в шапку, panel — под шапку.
export function useKzListPanel({ current, onImported, onOpen }: { current: KzImport | null; onImported: () => void; onOpen?: () => void }) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState<Busy>(null)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [rowErrors, setRowErrors] = useState<DirectoryImportResult['errors']>([])

  function start(action: NonNullable<Busy>['action'], text: string) { setBusy({ action, text }); setMessage(''); setError(''); setRowErrors([]) }
  function fail(reason: unknown) { setError(errorMessage(reason)); setRowErrors((reason as KzListError).result?.errors ?? []) }
  function takeFile(event: ChangeEvent<HTMLInputElement>) { const file = event.target.files?.[0]; event.target.value = ''; return file }
  const readPdf = (file: File) => readKzPdf(file, (page, total) => setBusy((value) => value && { ...value, text: `Читаем PDF: страница ${page} из ${total}` }))

  function imported(result: KzImportResult) {
    const { summary, import: info } = result
    const missing = summary.notInNomenclature.length ? ` Нет в номенклатуре: ${summary.notInNomenclature.slice(0, 5).join(', ')}${summary.notInNomenclature.length > 5 ? ` и ещё ${summary.notInNomenclature.length - 5}` : ''}.` : ''
    setMessage(`Загружено ${formatDisplayNumber(info.rowCount)} ${plural(info.rowCount, 'код', 'кода', 'кодов')}: действуют ${formatDisplayNumber(summary.active)}, ещё не начали ${summary.notYet}, уже не действуют ${summary.expired}.${missing}`)
    onImported()
  }

  async function excelFromPdf(event: ChangeEvent<HTMLInputElement>) {
    const file = takeFile(event)
    if (!file) return
    start('excel', 'Читаем PDF…')
    try {
      const extract = await readPdf(file)
      setBusy({ action: 'excel', text: 'Собираем Excel…' })
      await kzListApi.excel(extract)
      setMessage(`Excel скачан: ${formatDisplayNumber(extract.rows.length)} строк, ${Object.keys(extract.footnotes).length} сносок. Проверьте его и загрузите в шаге 2.`)
    } catch (reason) { fail(reason) } finally { setBusy(null) }
  }

  async function importPdf(event: ChangeEvent<HTMLInputElement>) {
    const file = takeFile(event)
    if (!file) return
    start('pdf', 'Читаем PDF…')
    try {
      const extract = await readPdf(file)
      setBusy({ action: 'pdf', text: 'Сохраняем…' })
      imported(await kzListApi.importPdf(extract))
    } catch (reason) { fail(reason) } finally { setBusy(null) }
  }

  async function importExcel(event: ChangeEvent<HTMLInputElement>) {
    const file = takeFile(event)
    if (!file) return
    start('import', 'Загружаем…')
    try { imported(await kzListApi.importExcel(file)) } catch (reason) { fail(reason) } finally { setBusy(null) }
  }

  const fileButton = (action: NonNullable<Busy>['action'], label: string, accept: string, onChange: (event: ChangeEvent<HTMLInputElement>) => void, primary = false) =>
    <label className={['kz-file', primary ? 'primary' : '', busy ? 'disabled' : ''].filter(Boolean).join(' ')}>
      {busy?.action === action ? busy.text : label}
      <input type="file" accept={accept} onChange={onChange} disabled={busy !== null} />
    </label>

  const button = <div className="dir-transfer">
    <button type="button" className={open ? 'dir-transfer-button active' : 'dir-transfer-button'} onClick={() => { if (!open) onOpen?.(); setOpen(!open) }} aria-expanded={open} title="Ставки ниже ЕТТ по обязательствам Казахстана в ВТО">Перечень РК</button>
  </div>

  const panel = open && <section className="dir-transfer-panel kz-panel">
    <div className="dir-transfer-row">
      <div>
        <strong>Перечень РК — ставки ниже ЕТТ</strong>
        <p>{current ? `Загружен ${formatRuDate(current.importedAt.slice(0, 10))}${current.importedByName ? `, ${current.importedByName}` : ''}: ${current.fileName || (current.source === 'pdf' ? 'PDF' : 'Excel')}, ${formatDisplayNumber(current.rowCount)} ${plural(current.rowCount, 'код', 'кода', 'кодов')}.` : 'Ещё не загружен.'} Актуальный PDF — на сайте ЕЭК, раздел «Применение ставок, отличных от ставок ЕТТ».</p>
      </div>
      <button type="button" className="dir-transfer-close" onClick={() => setOpen(false)} aria-label="Закрыть">×</button>
    </div>
    <div className="kz-steps">
      <div className="kz-step">
        <span className="kz-step-number">1</span>
        <div><b>Excel из PDF</b><p>Сайт прочитает PDF и скачает Excel: код, ставка, даты действия по сноскам, ставка ЕТТ для сравнения. Файл можно проверить и поправить.</p></div>
        {fileButton('excel', 'Выбрать PDF', '.pdf,application/pdf', (event) => void excelFromPdf(event))}
      </div>
      <div className="kz-step">
        <span className="kz-step-number">2</span>
        <div><b>Загрузить Excel</b><p>Нужны колонки «Код ТН ВЭД» и «Ставка РК»; «Действует с» и «Действует по» — ДД.ММ.ГГГГ. Список заменяется целиком.</p></div>
        {fileButton('import', 'Выбрать .xlsx', '.xlsx', (event) => void importExcel(event), true)}
      </div>
      <div className="kz-step kz-step-quick">
        <span className="kz-step-number">⚡</span>
        <div><b>Или загрузить PDF сразу</b><p>То же, что шаги 1 и 2, без промежуточного Excel.</p></div>
        {fileButton('pdf', 'Выбрать PDF', '.pdf,application/pdf', (event) => void importPdf(event))}
      </div>
    </div>
    {message && <p className="dir-transfer-ok" role="status">{message}</p>}
    {error && <p className="dir-transfer-error" role="alert">{error}</p>}
    {rowErrors.length > 0 && <ul className="dir-transfer-errors">{rowErrors.map((item) => <li key={`${item.row}-${item.message}`}><b>Строка {item.row}</b> {item.message}</li>)}</ul>}
  </section>

  return { button, panel, close: () => setOpen(false) }
}
