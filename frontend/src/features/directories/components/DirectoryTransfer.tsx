import { ChangeEvent, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { DirectoryImportResult, uploadDirectory } from '../common/directoryTransfer'
import { downloadFile } from '@/shared/api/download'
import './DirectoryTransfer.css'

type Props = {
  // /api/product-directory
  apiPath: string
  // «Справочник товаров» — для имени файла и подписей.
  title: string
  // Подсказка о колонках файла.
  hint: string
  onImported: () => void
}

// Выгрузить справочник в Excel и загрузить обратно: так справочник переносится между
// серверами или заполняется в Excel, а не по одной карточке. buttons — в шапку страницы,
// panel — под шапку.
export function useDirectoryTransfer({ apiPath, title, hint, onImported }: Props) {
  const [open, setOpen] = useState(false)
  const [update, setUpdate] = useState(true)
  const [busy, setBusy] = useState<'export' | 'import' | null>(null)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [rowErrors, setRowErrors] = useState<DirectoryImportResult['errors']>([])

  async function exportFile() {
    setBusy('export'); setError('')
    try {
      await downloadFile(`${apiPath}/export`, `${title}.xlsx`, 'Не удалось выгрузить справочник')
    } catch (reason) {
      setError(errorMessage(reason))
      setOpen(true)
    } finally {
      setBusy(null)
    }
  }

  async function importFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    setBusy('import'); setError(''); setMessage(''); setRowErrors([])
    try {
      const result = await uploadDirectory(`${apiPath}/import`, file, update)
      setMessage(`Загружено: новых ${result.created}, обновлено ${result.updated}${result.skipped ? `, пропущено существующих ${result.skipped}` : ''}.`)
      onImported()
    } catch (reason) {
      setError(errorMessage(reason))
      setRowErrors((reason as { result?: DirectoryImportResult }).result?.errors ?? [])
    } finally {
      setBusy(null)
    }
  }

  const buttons = <div className="dir-transfer">
      <button type="button" className="secondary" onClick={() => void exportFile()} disabled={busy !== null} title="Все действующие записи — в Excel">
        {busy === 'export' ? 'Выгружаем…' : 'Выгрузить'}
      </button>
      <button type="button" className={open ? 'secondary active' : 'secondary'} onClick={() => { setOpen(!open); setMessage(''); setError(''); setRowErrors([]) }} aria-expanded={open}>Загрузить</button>
  </div>
  const panel = open && <section className="dir-transfer-panel">
      <div className="dir-transfer-row">
        <div>
          <strong>Загрузка из Excel</strong>
          <p>{hint} Проще всего — выгрузить справочник, поправить файл и загрузить обратно.</p>
        </div>
        <button type="button" className="icon-button ghost dir-transfer-close" onClick={() => setOpen(false)} aria-label="Закрыть">×</button>
      </div>
      <div className="dir-transfer-row">
        <label className="dir-transfer-check"><input type="checkbox" checked={update} onChange={(event) => setUpdate(event.target.checked)} />Обновлять записи, которые уже есть</label>
        <label className={busy === 'import' ? 'dir-transfer-file busy' : 'dir-transfer-file'}>
          {busy === 'import' ? 'Загружаем…' : 'Выбрать .xlsx'}
          <input type="file" accept=".xlsx" onChange={(event) => void importFile(event)} disabled={busy !== null} />
        </label>
      </div>
      {message && <p className="dir-transfer-ok" role="status">{message}</p>}
      {error && <p className="dir-transfer-error" role="alert">{error}</p>}
      {rowErrors.length > 0 && <ul className="dir-transfer-errors">
        {rowErrors.map((item) => <li key={`${item.row}-${item.message}`}><b>Строка {item.row}</b> {item.message}</li>)}
      </ul>}
    </section>
  return { buttons, panel }
}
