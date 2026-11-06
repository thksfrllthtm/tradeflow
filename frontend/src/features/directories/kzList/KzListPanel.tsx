import { ChangeEvent, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatDisplayNumber } from '@/shared/lib/formatDisplayNumber'
import { plural } from '@/shared/lib/plural'
import { DirectoryImportResult } from '../common/directoryTransfer'
import { KzImportResult, KzListError, kzListApi } from './kzListApi'
import { readKzPdf } from './kzListPdf'
import './KzListPanel.css'

type Busy = { action: 'excel' | 'import'; text: string } | null

// «Перечень РК» (администратор, страница «Обновление данных»): загрузить PDF ЕЭК или Excel — список заменяется целиком.
// «Excel для проверки» — сайт читает PDF и скачивает Excel, его можно поправить и загрузить той же кнопкой.
// Что загрузилось — текстом; страница хранит последний результат и показывает его по ссылке «Что изменилось».
export function kzImportText({ summary, import: info }: KzImportResult) {
  const missing = summary.notInNomenclature.length ? ` Нет в номенклатуре: ${summary.notInNomenclature.slice(0, 5).join(', ')}${summary.notInNomenclature.length > 5 ? ` и ещё ${summary.notInNomenclature.length - 5}` : ''}.` : ''
  return `Загружено ${formatDisplayNumber(info.rowCount)} ${plural(info.rowCount, 'код', 'кода', 'кодов')}: действуют ${formatDisplayNumber(summary.active)}, ещё не начали ${summary.notYet}, уже не действуют ${summary.expired}.${missing}`
}

// action — кнопки в строку, notes — ошибки и «Excel скачан» под ней; onImported — результат загрузки.
export function useKzListUpload({ onImported }: { onImported: (result: KzImportResult) => void }) {
  const [busy, setBusy] = useState<Busy>(null)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [rowErrors, setRowErrors] = useState<DirectoryImportResult['errors']>([])

  function start(action: NonNullable<Busy>['action'], text: string) { setBusy({ action, text }); setMessage(''); setError(''); setRowErrors([]) }
  function fail(reason: unknown) { setError(errorMessage(reason)); setRowErrors((reason as KzListError).result?.errors ?? []) }
  function takeFile(event: ChangeEvent<HTMLInputElement>) { const file = event.target.files?.[0]; event.target.value = ''; return file }
  const readPdf = (file: File) => readKzPdf(file, (page, total) => setBusy((value) => value && { ...value, text: `PDF: стр. ${page} из ${total}` }))

  async function excelFromPdf(event: ChangeEvent<HTMLInputElement>) {
    const file = takeFile(event)
    if (!file) return
    start('excel', 'Читаем PDF…')
    try {
      const extract = await readPdf(file)
      setBusy({ action: 'excel', text: 'Собираем Excel…' })
      await kzListApi.excel(extract)
      setMessage(`Excel скачан: ${formatDisplayNumber(extract.rows.length)} строк, ${Object.keys(extract.footnotes).length} сносок. Проверьте его и загрузите кнопкой «Загрузить».`)
    } catch (reason) { fail(reason) } finally { setBusy(null) }
  }

  // PDF ЕЭК читается в браузере; Excel — с колонками «Код ТН ВЭД», «Ставка РК», «Действует с/по» (ДД.ММ.ГГГГ).
  async function upload(event: ChangeEvent<HTMLInputElement>) {
    const file = takeFile(event)
    if (!file) return
    const pdf = /\.pdf$/i.test(file.name)
    start('import', pdf ? 'Читаем PDF…' : 'Загружаем…')
    try {
      if (!pdf) { onImported(await kzListApi.importExcel(file)); return }
      const extract = await readPdf(file)
      setBusy({ action: 'import', text: 'Сохраняем…' })
      onImported(await kzListApi.importPdf(extract))
    } catch (reason) { fail(reason) } finally { setBusy(null) }
  }

  const action = <>
    <label className={busy ? 'du-file-link disabled' : 'du-file-link'} title="Сайт прочитает PDF и скачает Excel: код, ставка, даты действия по сноскам, ставка ЕТТ для сравнения">
      {busy?.action === 'excel' ? busy.text : 'Excel для проверки'}
      <input type="file" accept=".pdf,application/pdf" onChange={(event) => void excelFromPdf(event)} disabled={busy !== null} />
    </label>
    <label className={busy ? 'kz-file disabled' : 'kz-file'} title="PDF ЕЭК или Excel с колонками «Код ТН ВЭД», «Ставка РК», «Действует с», «Действует по». Список заменяется целиком">
      {busy?.action === 'import' ? busy.text : 'Загрузить PDF или Excel'}
      <input type="file" accept=".pdf,application/pdf,.xlsx" onChange={(event) => void upload(event)} disabled={busy !== null} />
    </label>
  </>

  const notes = <>
    {message && <p className="dir-transfer-ok" role="status">{message}</p>}
    {error && <p className="dir-transfer-error" role="alert">{error}</p>}
    {rowErrors.length > 0 && <ul className="dir-transfer-errors">{rowErrors.map((item) => <li key={`${item.row}-${item.message}`}><b>Строка {item.row}</b> {item.message}</li>)}</ul>}
  </>

  return { action, notes: message || error || rowErrors.length > 0 ? notes : null }
}
