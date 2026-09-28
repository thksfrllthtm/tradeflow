import { useEffect, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { verificationStatusLabel } from '../directoryLabels'
import { checkCodes, CodeCheckRow, parsePastedCodes, PastedCodes } from './pastedCodes'
import { tnvedDirectoryApi } from './tnvedDirectoryApi'
import { TNVEDDirectoryEntry } from './types'
import './TNVEDCodeCheck.css'

type Props = {
  // canAdd — администратор и инвойсер; canRestore (архивные коды) — только администратор.
  canAdd: boolean
  canRestore: boolean
  // Меняется после сохранения записи — проверка повторяется, добавленный код уходит из «нет».
  refreshKey: number
  onAdd: (code: string) => void
  onOpen: (entry: TNVEDDirectoryEntry) => void
  onRestore: (entry: TNVEDDirectoryEntry) => void
}

const statusText = { missing: 'Нет в справочнике', archived: 'Только в архиве', found: 'Есть' }

// Вставить колонку кодов из Excel и сразу увидеть, каких нет в справочнике, — вместо поиска
// каждого кода по одному. button — в шапку страницы, panel — под шапку.
export function useTNVEDCodeCheck({ canAdd, canRestore, refreshKey, onAdd, onOpen, onRestore }: Props) {
  const [open, setOpen] = useState(false)
  const [text, setText] = useState('')
  const [pasted, setPasted] = useState<PastedCodes | null>(null)
  const [rows, setRows] = useState<CodeCheckRow[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!pasted) return
    let cancelled = false
    setBusy(true); setError('')
    tnvedDirectoryApi.list({ includeArchived: true })
      .then((entries) => { if (!cancelled) setRows(checkCodes(pasted.codes, entries)) })
      .catch((reason) => { if (!cancelled) setError(errorMessage(reason)) })
      .finally(() => { if (!cancelled) setBusy(false) })
    return () => { cancelled = true }
  }, [pasted, refreshKey])

  function check() {
    const result = parsePastedCodes(text)
    setCopied(false); setRows([])
    if (result.codes.length === 0 && result.invalid.length === 0) {
      setPasted(null); setError('Вставьте колонку с кодами ТН ВЭД.')
      return
    }
    setPasted(result)
  }

  function reset() { setText(''); setPasted(null); setRows([]); setError(''); setCopied(false) }

  const missing = rows.filter((row) => row.status === 'missing')
  async function copyMissing() {
    try { await navigator.clipboard.writeText(missing.map((row) => row.code).join('\n')); setCopied(true) } catch { setError('Не удалось скопировать. Выделите коды вручную.') }
  }

  const count = (status: CodeCheckRow['status']) => rows.filter((row) => row.status === status).length
  const button = <div className="dir-transfer">
    <button type="button" className={open ? 'dir-transfer-button active' : 'dir-transfer-button'} onClick={() => setOpen(!open)} aria-expanded={open} title="Вставить колонку кодов из Excel и найти недостающие">Проверить коды</button>
  </div>
  const panel = open && <section className="dir-transfer-panel code-check">
    <div className="dir-transfer-row">
      <div>
        <strong>Проверка кодов перед расчётом</strong>
        <p>Скопируйте колонку кодов в Excel и вставьте сюда. Повторы и заголовок колонки можно не убирать.</p>
      </div>
      <button type="button" className="dir-transfer-close" onClick={() => setOpen(false)} aria-label="Закрыть">×</button>
    </div>
    <textarea className="code-check-input" rows={5} value={text} onChange={(event) => setText(event.target.value)} placeholder={'4015190000\n6116102000\n6116990000'} spellCheck={false} autoFocus
      onKeyDown={(event) => { if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) { event.preventDefault(); check() } }} />
    <div className="dir-transfer-row">
      <span className="code-check-note">{pasted ? `Строк с кодами: ${pasted.rows}, разных кодов: ${pasted.codes.length}` : 'Ctrl+Enter — проверить'}</span>
      <div className="code-check-actions">
        {(text || pasted) && <button type="button" className="code-check-secondary" onClick={reset}>Очистить</button>}
        <button type="button" className="code-check-primary" onClick={check} disabled={busy || text.trim() === ''}>{busy ? 'Проверяем…' : 'Проверить'}</button>
      </div>
    </div>
    {error && <p className="dir-transfer-error" role="alert">{error}</p>}
    {pasted && rows.length > 0 && <>
      <div className="code-check-summary" role="status">
        {count('missing') > 0 ? <span className="missing">Нет в справочнике: {count('missing')}</span> : <span className="ok">Все коды есть в справочнике</span>}
        {count('archived') > 0 && <span className="archived">Только в архиве: {count('archived')}</span>}
        <span className="found">Есть: {count('found')}</span>
        {missing.length > 0 && <button type="button" className="code-check-secondary" onClick={() => void copyMissing()}>{copied ? 'Скопировано' : 'Копировать недостающие'}</button>}
      </div>
      <ul className="code-check-list">
        {rows.map((row) => <li key={row.code} className={row.status}>
          <code>{row.code}</code>
          <span className={`code-check-status ${row.status}`}>{statusText[row.status]}</span>
          <span className="code-check-name">
            {row.entries[0] ? <>{row.entries[0].name || 'Без описания'}{row.entries.length > 1 && <em> и ещё {row.entries.length - 1}</em>}{row.status === 'found' && row.entries.every((item) => item.verificationStatus === 'needs_review') && <em> · {verificationStatusLabel('needs_review').toLowerCase()}</em>}</> : null}
            {row.padded && <em> · в Excel потерян ведущий ноль</em>}
            {row.count > 1 && <em> · строк: {row.count}</em>}
          </span>
          <span className="code-check-row-actions">
            {row.status === 'missing' && canAdd && <button type="button" className="code-check-primary" onClick={() => onAdd(row.code)}>Добавить</button>}
            {row.status === 'archived' && canRestore && <button type="button" className="code-check-secondary" onClick={() => onRestore(row.entries[0])}>Восстановить</button>}
            {row.status === 'found' && <button type="button" className="code-check-secondary" onClick={() => onOpen(row.entries[0])}>Открыть</button>}
          </span>
        </li>)}
      </ul>
    </>}
    {pasted && pasted.invalid.length > 0 && <p className="dir-transfer-error">Не похоже на код из 10 цифр: {pasted.invalid.join(', ')}</p>}
    {pasted && count('missing') > 0 && !canAdd && <p className="code-check-note">Недостающие коды добавляют администратор и инвойсер: скопируйте список и отправьте его.</p>}
    {pasted && count('archived') > 0 && !canRestore && <p className="code-check-note">Коды из архива восстанавливает администратор.</p>}
  </section>
  return { button, panel }
}
