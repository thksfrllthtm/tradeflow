import { useEffect, useRef, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatRuDate } from '@/features/estimates/shared/lib/dates'
import { verificationStatusLabel } from '../directoryLabels'
import { KzImport } from '../kzList/kzListApi'
import { NomenclatureImport, nomenclatureApi } from '../nomenclature/nomenclatureApi'
import { directoryDraft, nameParts } from '../nomenclature/nomenclatureDraft'
import { checkCodes, checkNomenclature, CodeCheckRow, parsePastedCodes, PastedCodes } from './pastedCodes'
import { tnvedDirectoryApi } from './tnvedDirectoryApi'
import { TNVEDDirectoryEntry, TNVEDDirectoryInput } from './types'
import './TNVEDCodeCheck.css'

type Props = {
  // directory — сверка со справочником ТН ВЭД; nomenclature — с номенклатурой ТН ВЭД ЕАЭС.
  mode: 'directory' | 'nomenclature'
  // canAdd — администратор и инвойсер; canRestore (архивные коды) — только администратор.
  canAdd: boolean
  canRestore?: boolean
  // Меняется после сохранения записи — проверка повторяется, добавленный код уходит из «нет».
  refreshKey: number
  // Панель открыта сразу — переход с главной по «Проверить коды ТН ВЭД».
  initialOpen?: boolean
  // draft — код, а если код есть в номенклатуре, ещё описание и ставки.
  onAdd: (draft: Partial<TNVEDDirectoryInput>) => void
  onOpen?: (entry: TNVEDDirectoryEntry) => void
  onRestore?: (entry: TNVEDDirectoryEntry) => void
}

const statusText = { unknown: 'Нет в ТН ВЭД ЕАЭС', missing: 'Нет в справочнике', archived: 'Только в архиве', found: 'Есть', official: 'Есть в ТН ВЭД' }

// Вставить колонку кодов из Excel и сразу увидеть результат по всем кодам, а не искать каждый
// по одному. На странице справочника — есть ли код в справочнике; на странице номенклатуры —
// есть ли он в ТН ВЭД ЕАЭС, с наименованием, ставками и ссылкой. button — в шапку, panel — под шапку.
export function useTNVEDCodeCheck({ mode, canAdd, canRestore = false, refreshKey, initialOpen = false, onAdd, onOpen, onRestore }: Props) {
  const nomenclatureMode = mode === 'nomenclature'
  const [open, setOpen] = useState(initialOpen)
  const [text, setText] = useState('')
  const [pasted, setPasted] = useState<PastedCodes | null>(null)
  const [rows, setRows] = useState<CodeCheckRow[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [copied, setCopied] = useState(false)
  const [sources, setSources] = useState<{ nomenclature: NomenclatureImport | null; kz: KzImport | null }>({ nomenclature: null, kz: null })
  const input = useRef<HTMLTextAreaElement>(null)

  // Фокус после монтирования, а не autoFocus: иначе его забирает поиск на странице ниже.
  useEffect(() => { if (open) input.current?.focus() }, [open])

  useEffect(() => {
    if (!pasted) return
    let cancelled = false
    setBusy(true); setError('')
    const codes = pasted.codes.map((item) => item.code)
    const run = async () => {
      if (nomenclatureMode) {
        const found = await nomenclatureApi.lookup(codes)
        if (cancelled) return
        setSources({ nomenclature: found.import, kz: found.kzImport })
        if (!found.import) { setRows([]); setError('Номенклатура ещё не загружена — проверять не с чем.'); return }
        setRows(checkNomenclature(pasted.codes, found.items))
        return
      }
      // Номенклатура здесь не показывается — из неё только заполняется карточка при «Добавить».
      const [entries, found] = await Promise.all([tnvedDirectoryApi.list({ includeArchived: true }), nomenclatureApi.lookup(codes).catch(() => null)])
      if (cancelled) return
      setSources({ nomenclature: found?.import ?? null, kz: found?.kzImport ?? null })
      setRows(checkCodes(pasted.codes, entries, found?.import ? found.items : []))
    }
    run().catch((reason) => { if (!cancelled) setError(errorMessage(reason)) }).finally(() => { if (!cancelled) setBusy(false) })
    return () => { cancelled = true }
  }, [pasted, refreshKey, nomenclatureMode])

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

  const count = (status: CodeCheckRow['status']) => rows.filter((row) => row.status === status).length
  // Копируются коды, которых нет: в справочнике — недостающие, в номенклатуре — неизвестные.
  const absent = rows.filter((row) => row.status === (nomenclatureMode ? 'unknown' : 'missing'))
  async function copyAbsent() {
    try { await navigator.clipboard.writeText(absent.map((row) => row.code).join('\n')); setCopied(true) } catch { setError('Не удалось скопировать. Выделите коды вручную.') }
  }
  const add = (row: CodeCheckRow) => onAdd(row.nomenclature ? directoryDraft(row.nomenclature, sources.nomenclature, sources.kz) : { code: row.code })

  const summary = nomenclatureMode
    ? <>
      {count('unknown') > 0 ? <span className="missing">Нет в ТН ВЭД ЕАЭС: {count('unknown')}</span> : <span className="ok">Все коды есть в ТН ВЭД ЕАЭС</span>}
      {count('unknown') > 0 && <span className="found">Есть в ТН ВЭД: {count('official')}</span>}
      {absent.length > 0 && <button type="button" className="code-check-secondary" onClick={() => void copyAbsent()}>{copied ? 'Скопировано' : 'Копировать отсутствующие'}</button>}
    </>
    : <>
      {count('missing') > 0 ? <span className="missing">Нет в справочнике: {count('missing')}</span> : <span className="ok">Все коды есть в справочнике</span>}
      {count('archived') > 0 && <span className="archived">Только в архиве: {count('archived')}</span>}
      <span className="found">Есть: {count('found')}</span>
      {absent.length > 0 && <button type="button" className="code-check-secondary" onClick={() => void copyAbsent()}>{copied ? 'Скопировано' : 'Копировать недостающие'}</button>}
    </>

  const nomenclatureRow = (row: CodeCheckRow) => {
    const item = row.nomenclature
    return <>
      <span className="code-check-name">
        {item ? <>{nameParts(item.name).slice(-1)[0]}<em> · ЕТТ {item.tariffText || 'не указана'}{item.kz && ` · РК ${item.kz.rateText}${item.kz.active ? '' : ` (${item.kz.status})`}`}{item.vatPercent && ` · НДС ${item.vatPercent}%`}</em></>
          : <em>Проверьте код: в номенклатуре{sources.nomenclature?.actualDate ? ` на ${formatRuDate(sources.nomenclature.actualDate)}` : ''} его нет</em>}
        {row.padded && <em> · в Excel потерян ведущий ноль</em>}
        {row.count > 1 && <em> · строк: {row.count}</em>}
      </span>
      <span className="code-check-row-actions">
        {item?.detailsUrl && <a className="code-check-link" href={item.detailsUrl} target="_blank" rel="noreferrer">Подробнее</a>}
        {item && (item.inDirectory ? <span className="code-check-status found">В справочнике</span> : canAdd && <button type="button" className="code-check-primary" onClick={() => add(row)}>Добавить</button>)}
      </span>
    </>
  }

  const directoryRow = (row: CodeCheckRow) => <>
    <span className="code-check-name">
      {row.entries[0] ? <>{row.entries[0].name || 'Без описания'}{row.entries.length > 1 && <em> и ещё {row.entries.length - 1}</em>}{row.status === 'found' && row.entries.every((item) => item.verificationStatus === 'needs_review') && <em> · {verificationStatusLabel('needs_review').toLowerCase()}</em>}</> : null}
      {row.padded && <em> · в Excel потерян ведущий ноль</em>}
      {row.count > 1 && <em> · строк: {row.count}</em>}
    </span>
    <span className="code-check-row-actions">
      {row.status === 'missing' && canAdd && <button type="button" className="code-check-primary" onClick={() => add(row)}>Добавить</button>}
      {row.status === 'archived' && canRestore && onRestore && <button type="button" className="code-check-secondary" onClick={() => onRestore(row.entries[0])}>Восстановить</button>}
      {row.status === 'found' && onOpen && <button type="button" className="code-check-secondary" onClick={() => onOpen(row.entries[0])}>Открыть</button>}
    </span>
  </>

  const button = <div className="dir-transfer">
    <button type="button" className={open ? 'dir-transfer-button active' : 'dir-transfer-button'} onClick={() => setOpen(!open)} aria-expanded={open} title={nomenclatureMode ? 'Вставить колонку кодов из Excel и проверить по номенклатуре ТН ВЭД ЕАЭС' : 'Вставить колонку кодов из Excel и найти недостающие в справочнике'}>Проверить коды</button>
  </div>
  const panel = open && <section className="dir-transfer-panel code-check">
    <div className="dir-transfer-row">
      <div>
        <strong>{nomenclatureMode ? 'Проверка кодов по номенклатуре ТН ВЭД ЕАЭС' : 'Проверка кодов перед расчётом'}</strong>
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
      <div className="code-check-summary" role="status">{summary}</div>
      <ul className="code-check-list">
        {rows.map((row) => <li key={row.code} className={row.status}>
          <code>{row.code}</code>
          <span className={`code-check-status ${row.status}`}>{statusText[row.status]}</span>
          {nomenclatureMode ? nomenclatureRow(row) : directoryRow(row)}
        </li>)}
      </ul>
    </>}
    {pasted && pasted.invalid.length > 0 && <p className="dir-transfer-error">Не похоже на код из 10 цифр: {pasted.invalid.join(', ')}</p>}
    {pasted && !nomenclatureMode && count('missing') > 0 && !canAdd && <p className="code-check-note">Недостающие коды добавляют администратор и инвойсер: скопируйте список и отправьте его.</p>}
    {pasted && !nomenclatureMode && count('archived') > 0 && !canRestore && <p className="code-check-note">Коды из архива восстанавливает администратор.</p>}
  </section>
  return { button, panel }
}
