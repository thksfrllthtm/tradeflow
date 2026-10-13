import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { useEffect, useRef, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatRuDate } from '@/shared/lib/dates'
import { NomenclatureImport } from '@/shared/tnved/nomenclature'
import { nameParts } from '@/shared/tnved/codes'
import { nomenclatureApi } from './nomenclatureApi'
import { checkNomenclature, CodeCheckRow, parsePastedCodes, PastedCodes } from './pastedCodes'
import './CodeCheck.css'

const statusText = { unknown: 'Нет в ТН ВЭД ЕАЭС', official: 'Есть в ТН ВЭД' }

// Вставить колонку кодов из Excel и сразу увидеть по всем кодам, есть ли они в номенклатуре ТН ВЭД
// ЕАЭС, — с наименованием, ставками и ссылкой. initialOpen — переход с главной по «Проверить коды».
// button — в шапку страницы, panel — под шапку.
export function useCodeCheck({ initialOpen = false, onOpen }: { initialOpen?: boolean; onOpen?: () => void }) {
  const [open, setOpen] = useState(initialOpen)
  const [text, setText] = useState('')
  const [pasted, setPasted] = useState<PastedCodes | null>(null)
  const [rows, setRows] = useState<CodeCheckRow[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [copied, setCopied] = useState(false)
  const [source, setSource] = useState<NomenclatureImport | null>(null)
  const input = useRef<HTMLTextAreaElement>(null)

  // Фокус после монтирования, а не autoFocus: иначе его забирает поиск на странице ниже.
  useEffect(() => { if (open) input.current?.focus() }, [open])

  useEffect(() => {
    if (!pasted) return
    let cancelled = false
    setBusy(true); setError('')
    nomenclatureApi.lookup(pasted.codes.map((item) => item.code)).then((found) => {
      if (cancelled) return
      setSource(found.import)
      if (!found.import) { setRows([]); setError('Номенклатура ещё не загружена — проверять не с чем.'); return }
      setRows(checkNomenclature(pasted.codes, found.items))
    }).catch((reason) => { if (!cancelled) setError(errorMessage(reason)) }).finally(() => { if (!cancelled) setBusy(false) })
    return () => { cancelled = true }
  }, [pasted])

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

  const unknown = rows.filter((row) => row.status === 'unknown')
  async function copyAbsent() {
    try { await navigator.clipboard.writeText(unknown.map((row) => row.code).join('\n')); setCopied(true) } catch { setError('Не удалось скопировать. Выделите коды вручную.') }
  }

  const button = <div className="dir-transfer">
    <button type="button" className={open ? 'dir-transfer-button active' : 'dir-transfer-button'} onClick={() => { if (!open) onOpen?.(); setOpen(!open) }} aria-expanded={open} title="Вставить колонку кодов из Excel и проверить по номенклатуре ТН ВЭД ЕАЭС">Проверить коды</button>
  </div>
  const panel = open && <section className="dir-transfer-panel code-check">
    <div className="dir-transfer-row">
      <div>
        <strong>Проверка кодов по номенклатуре ТН ВЭД ЕАЭС</strong>
        <p>Скопируйте колонку кодов в Excel и вставьте сюда. Повторы и заголовок колонки можно не убирать.</p>
      </div>
      <button type="button" className="dir-transfer-close" onClick={() => setOpen(false)} aria-label="Закрыть">×</button>
    </div>
    <textarea className="code-check-input" rows={5} value={text} onChange={(event) => setText(event.target.value)} placeholder={'4015190000\n6116102000\n6116990000'} spellCheck={false} ref={input}
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
        {unknown.length > 0 ? <span className="missing">Нет в ТН ВЭД ЕАЭС: {unknown.length}</span> : <span className="ok">Все коды есть в ТН ВЭД ЕАЭС</span>}
        {unknown.length > 0 && <span className="found">Есть в ТН ВЭД: {rows.length - unknown.length}</span>}
        {unknown.length > 0 && <button type="button" className="code-check-secondary" onClick={() => void copyAbsent()}>{copied ? 'Скопировано' : 'Копировать отсутствующие'}</button>}
      </div>
      <ul className="code-check-list">
        {rows.map((row) => {
          const item = row.nomenclature
          return <li key={row.code} className={row.status}>
            <CopyCode code={row.code} />
            <span className={`code-check-status ${row.status}`}>{statusText[row.status]}</span>
            <span className="code-check-name">
              {item ? <>{nameParts(item.name).slice(-1)[0]}<em> · ЕТТ {item.tariffText || 'не указана'}{item.kz && ` · РК ${item.kz.rateText}${item.kz.active ? '' : ` (${item.kz.status})`}`}{item.vatPercent && ` · НДС ${item.vatPercent}%`}</em></>
                : <em>Проверьте код: в номенклатуре{source?.actualDate ? ` на ${formatRuDate(source.actualDate)}` : ''} его нет</em>}
              {row.padded && <em> · в Excel потерян ведущий ноль</em>}
              {row.count > 1 && <em> · строк: {row.count}</em>}
            </span>
            <span className="code-check-row-actions">
              {item?.detailsUrl && <a className="code-check-link" href={item.detailsUrl} target="_blank" rel="noreferrer">Подробнее</a>}
            </span>
          </li>
        })}
      </ul>
    </>}
    {pasted && pasted.invalid.length > 0 && <p className="dir-transfer-error">Не похоже на код из 10 цифр: {pasted.invalid.join(', ')}</p>}
  </section>
  return { button, panel, close: () => setOpen(false) }
}
