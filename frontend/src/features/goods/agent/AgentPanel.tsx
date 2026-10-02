import { FormEvent, useCallback, useEffect, useRef, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { resizePhoto } from '@/shared/lib/resizePhoto'
import { Link } from '@/shared/router/Link'
import { paths } from '@/shared/router/router'
import { GoodsItemInput } from '@/shared/types/goods'
import { photosOf } from '@/shared/lib/itemPhotos'
import { decisionsApi } from '@/features/decisions/decisionsApi'
import { AgentCandidate, AgentItem, AgentMessage, AgentResult, AgentSession, AgentStatus, SameProductMatch, agentApi } from './agentApi'
import '@/shared/styles/drawer.css'
import './agent.css'
import { decisionStatusLabels } from '@/shared/tnved/decisions'
import { nameParts, formatCode } from '@/shared/tnved/codes'

type Props = {
  estimateId: number
  item: GoodsItemInput
  rowNumber: number
  onClose: () => void
  // Применить код к строке сметы (ставки — из номенклатуры) и наименование.
  onApplyCode: (code: string) => Promise<void> | void
  onApplyName: (name: string) => void
  // Подбор начался, закончился или изменился — обновить значки в таблице.
  onChanged: () => void
}

const pollInterval = 1500

const toolIcons: Record<string, string> = {
  search_nomenclature: '🔍', get_codes: '✔️', list_heading: '📂', open_details: '📄', search_decisions: '🗂️', search_directory: '📚',
}

const verdictLabels: Record<AgentCandidate['verdict'], string> = { fits: 'подходит', doubtful: 'возможно', no: 'не подходит' }

export function agentItemFrom(item: GoodsItemInput): AgentItem {
  return {
    name: item.name ?? '', supplierName: item.supplierName && item.supplierName !== item.name ? item.supplierName : '', code: item.code ?? '',
    quantity: item.pairs ?? '', places: item.places ?? '', grossKg: item.grossKg ?? '', netKg: item.netKg ?? '', pricePerUnit: item.pricePerPair ?? '', unit: 'шт',
  }
}

// Фото строки для ИИ: до трёх, уменьшенные до 1024 px.
export async function agentPhotosFrom(item: GoodsItemInput): Promise<string[]> {
  return Promise.all(photosOf(item).slice(0, 3).map((photo) => resizePhoto(photo)))
}

// Чат подбора кода ТН ВЭД для одной строки сметы: товар, шаги агента, вопросы, итог с кодами
// и цитатами, кнопки «Применить». Подбор идёт на сервере — панель можно закрыть.
export function AgentPanel({ estimateId, item, rowNumber, onClose, onApplyCode, onApplyName, onChanged }: Props) {
  const itemId = item.id ?? ''
  const [session, setSession] = useState<AgentSession | null>(null)
  const [status, setStatus] = useState<AgentStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [text, setText] = useState('')
  const [applied, setApplied] = useState<{ code?: string; name?: string }>({})
  const [savedRowCode, setSavedRowCode] = useState('')
  const lastId = useRef(0)
  const bodyRef = useRef<HTMLDivElement>(null)
  const changedRef = useRef(onChanged)
  changedRef.current = onChanged

  // Ответ сервера: целиком (replace) или только новые сообщения после lastId. Слияние — по id,
  // без побочных эффектов внутри setState (StrictMode вызывает его дважды).
  const accept = useCallback((next: AgentSession, replace: boolean) => {
    const newest = next.messages.length ? next.messages[next.messages.length - 1].id : 0
    lastId.current = replace ? newest : Math.max(lastId.current, newest)
    setSession((current) => {
      if (replace || !current || current.id !== next.id) return next
      const known = new Set(current.messages.map((message) => message.id))
      return { ...next, photos: next.photos ?? current.photos, messages: [...current.messages, ...next.messages.filter((message) => !known.has(message.id))] }
    })
  }, [])

  const start = useCallback(async () => {
    setBusy(true); setError('')
    try {
      const next = await agentApi.start({ estimateId, itemId, item: agentItemFrom(item), photos: await agentPhotosFrom(item) })
      setApplied({})
      accept(next, true)
      changedRef.current()
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false)
    }
  }, [estimateId, itemId, item, accept])

  // Открытие: последний подбор строки; если его нет — сразу новый.
  useEffect(() => {
    let active = true
    setLoading(true)
    void Promise.all([agentApi.status().catch(() => null), agentApi.rows(estimateId)]).then(async ([agentStatus, rows]) => {
      if (!active) return
      setStatus(agentStatus)
      const row = rows.items.find((entry) => entry.itemId === itemId)
      if (row) accept(await agentApi.get(row.sessionId), true)
      else if (agentStatus?.configured) await start()
    }).catch((reason) => { if (active) setError(errorMessage(reason)) }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
    // Только при открытии панели для строки.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [estimateId, itemId])

  // Пока подбор идёт — забираем новые сообщения.
  const sessionId = session?.id
  const running = session?.status === 'running'
  useEffect(() => {
    if (!sessionId || !running) return
    let active = true
    const timer = window.setInterval(() => {
      agentApi.get(sessionId, lastId.current).then((next) => {
        if (!active) return
        accept(next, false)
        if (next.status !== 'running') changedRef.current()
      }).catch(() => undefined)
    }, pollInterval)
    return () => { active = false; window.clearInterval(timer) }
  }, [sessionId, running, accept])

  // Новые сообщения — прокрутка вниз.
  const messageCount = session?.messages.length ?? 0
  useEffect(() => {
    bodyRef.current?.scrollTo({ top: bodyRef.current.scrollHeight, behavior: 'smooth' })
  }, [messageCount, running])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose])

  async function act(action: () => Promise<AgentSession>) {
    setBusy(true); setError('')
    try {
      accept(await action(), true)
      changedRef.current()
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false)
    }
  }

  async function send(event: FormEvent) {
    event.preventDefault()
    const value = text.trim()
    if (!value || !session) return
    setText('')
    await act(() => agentApi.message(session.id, value))
  }

  async function applyCode(code: string, record: boolean) {
    setBusy(true); setError('')
    try {
      await onApplyCode(code)
      setApplied((current) => ({ ...current, code }))
      if (record && session) {
        await agentApi.apply(session.id, code, session.result?.name ?? '')
        accept(await agentApi.get(session.id, lastId.current), false)
      }
      changedRef.current()
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false)
    }
  }

  // Код, уже стоящий в строке (выбран вручную), — в базу решений как «Не проверено».
  async function saveRowCode() {
    setBusy(true); setError('')
    try {
      const photos = await agentPhotosFrom(item)
      const thumb = photosOf(item)[0] ? await resizePhoto(photosOf(item)[0], 160, 0.7) : ''
      await decisionsApi.create({ invoiceName: item.name ?? '', supplierName: item.supplierName !== item.name ? item.supplierName : '', finalName: item.standardName || '', code: item.code, photos, thumb, estimateId, itemId })
      setSavedRowCode(item.code)
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false)
    }
  }

  function applyName(name: string) {
    onApplyName(name)
    setApplied((current) => ({ ...current, name }))
  }

  const canWrite = Boolean(session) && !running && status?.configured !== false
  const photos = session?.photos ?? []
  return <div className="drawer-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <aside className="drawer ag-drawer" role="dialog" aria-modal="true" aria-labelledby="ag-title">
      <header className="drawer-head">
        <div>
          <h2 id="ag-title">Подбор кода ИИ · строка {rowNumber}</h2>
          <p>{running ? 'Агент работает — панель можно закрыть, подбор продолжится' : session ? statusText(session) : status && !status.configured ? 'ИИ не настроен на сервере' : 'Загрузка…'}</p>
        </div>
        <button type="button" className="drawer-close" onClick={onClose} aria-label="Закрыть" title="Закрыть (Esc)">×</button>
      </header>
      <div className="drawer-body ag-body" ref={bodyRef}>
        {loading && !session && <p className="ag-muted">Загрузка…</p>}
        {status && !status.configured && <p className="ag-error">ИИ для подбора кодов не настроен: на сервере нет ключа (TNVED_AI_API_KEY или DEEPSEEK_API_KEY).</p>}
        {!loading && !session && status?.configured && <div className="ag-empty"><p>Подбора для этой строки ещё не было.</p><button type="button" onClick={() => void start()} disabled={busy}>Подобрать код</button></div>}
        {session?.messages.map((message) => <MessageView key={message.id} message={message} photos={photos} applied={applied} busy={busy}
          onApplyCode={applyCode} onApplyName={applyName} onRerun={() => void act(() => agentApi.rerun(session.id))} />)}
        {running && <div className="ag-typing" aria-live="polite"><span /><span /><span /> агент думает…</div>}
        {error && <p className="ag-error" role="alert">{error}</p>}
      </div>
      <footer className="drawer-foot ag-foot">
        <form className="ag-form" onSubmit={(event) => void send(event)}>
          <input value={text} onChange={(event) => setText(event.target.value)} maxLength={1000} disabled={!canWrite || busy}
            placeholder={session?.status === 'waiting' ? 'Ответьте на вопрос агента…' : 'Уточнить: «это латекс», «посмотри 3926»…'} aria-label="Сообщение агенту" />
          <button type="submit" disabled={!canWrite || busy || !text.trim()}>Отправить</button>
        </form>
        <div className="ag-foot-row">
          {running
            ? <button type="button" className="ag-link" onClick={() => session && void act(() => agentApi.cancel(session.id))} disabled={busy}>Остановить</button>
            : <button type="button" className="ag-link" onClick={() => void start()} disabled={busy || status?.configured === false}>Начать заново</button>}
          {session && session.runs > 0 && <span className="ag-cost" title={`Модель ${session.model}`}>шагов {session.steps} · ${session.costUsd}</span>}
          {/^d{10}$/.test(item.code) && (savedRowCode === item.code
            ? <span className="ag-done">✓ код строки в базе решений</span>
            : <button type="button" className="ag-link" onClick={() => void saveRowCode()} disabled={busy} title="Записать код, который сейчас стоит в строке, в базу решений (ждёт проверки администратором)">Код строки → в базу решений</button>)}
          <span className="ag-disclaimer">Подсказка ИИ — код выбирает декларант</span>
        </div>
      </footer>
    </aside>
  </div>
}

function statusText(session: AgentSession) {
  switch (session.status) {
    case 'waiting': return 'Агент ждёт ответа на вопрос'
    case 'done': return session.result ? 'Подбор готов — проверьте и примените код' : 'Подбор закончен'
    case 'error': return 'Подбор прервался с ошибкой'
    case 'cancelled': return 'Подбор остановлен'
    default: return ''
  }
}

type MessageProps = {
  message: AgentMessage
  photos: string[]
  applied: { code?: string; name?: string }
  busy: boolean
  onApplyCode: (code: string, record: boolean) => void
  onApplyName: (name: string) => void
  onRerun: () => void
}

function MessageView({ message, photos, applied, busy, onApplyCode, onApplyName, onRerun }: MessageProps) {
  switch (message.kind) {
    case 'input': return <InputCard item={message.content.item} photos={photos} />
    case 'text': return <div className="ag-msg ag-bot">{message.content.text}</div>
    case 'question': return <div className="ag-msg ag-question"><b>Вопрос агента</b>{message.content.text}</div>
    case 'user': return <div className="ag-msg ag-user">{message.content.text}</div>
    case 'notice': return <p className="ag-notice">{message.content.text}</p>
    case 'error': return <p className="ag-error">{message.content.message}</p>
    case 'rerun': return <p className="ag-divider"><span>подбор заново</span></p>
    case 'tool': {
      const { tool, label, detail, url, failed } = message.content
      return <div className={failed ? 'ag-step failed' : 'ag-step'}>
        <span className="ag-step-icon" aria-hidden="true">{toolIcons[tool] ?? '•'}</span>
        <span className="ag-step-label">{label}</span>
        <span className="ag-step-detail">{detail}</span>
        {url && <a href={url} target="_blank" rel="noreferrer" title="Открыть страницу кода">↗</a>}
      </div>
    }
    case 'match': return <MatchCard matches={message.content.matches} skipped={message.content.skipped} applied={applied} busy={busy} onApplyCode={onApplyCode} onRerun={onRerun} />
    case 'result': return <ResultCard result={message.content} applied={applied} busy={busy} onApplyCode={onApplyCode} onApplyName={onApplyName} />
  }
}

function InputCard({ item, photos }: { item: AgentItem; photos: string[] }) {
  const unitNet = perUnit(item.netKg || item.grossKg, item.quantity)
  return <div className="ag-input">
    {photos.length > 0 && <div className="ag-photos">{photos.map((photo, index) => <img key={index} src={photo} alt={`Фото товара ${index + 1}`} />)}</div>}
    <div className="ag-input-text">
      <small>Товар</small>
      <strong>{item.name || item.supplierName || 'Без наименования'}</strong>
      {item.supplierName && item.supplierName !== item.name && <span>{item.supplierName}</span>}
      <em>
        {item.code && <>код из файла {formatCode(item.code)} · </>}
        {item.quantity && <>{item.quantity} {item.unit}</>}
        {unitNet && <> · 1 {item.unit}: {unitNet} кг</>}
        {item.pricePerUnit && <> · {item.pricePerUnit} $</>}
        {photos.length === 0 && <> · без фото</>}
      </em>
    </div>
  </div>
}

function perUnit(total: string, quantity: string) {
  const value = Number(String(total).replace(',', '.'))
  const count = Number(String(quantity).replace(',', '.'))
  if (!(value > 0) || !(count > 0)) return ''
  return String(Math.round((value / count) * 10000) / 10000).replace('.', ',')
}

function CodeLine({ code, name }: { code: string; name?: string }) {
  const parts = name ? nameParts(name) : []
  return <div className="ag-code-line">
    <b className="ag-code">{formatCode(code)}</b>
    {parts.length > 0 && <span title={parts.join(' › ')}>{parts.slice(-2).join(' › ')}</span>}
  </div>
}

function MatchCard({ matches, skipped, applied, busy, onApplyCode, onRerun }: { matches: SameProductMatch[]; skipped: boolean; applied: { code?: string }; busy: boolean; onApplyCode: (code: string, record: boolean) => void; onRerun: () => void }) {
  return <div className="ag-result ag-match">
    <h3>{skipped ? 'Этот товар уже есть в базе решений' : 'В базе решений есть похожие записи'}</h3>
    {matches.map(({ decision, byPhoto, byName }) => <div key={decision.id} className={`ag-cand ${decision.status === 'verified' ? 'fits' : 'doubtful'}`}>
      <div className="ag-cand-top">
        <CodeLine code={decision.code} name={decision.nomenclature?.name} />
        <i className={`ag-verdict ${decision.status === 'verified' ? 'fits' : 'doubtful'}`}>{decisionStatusLabels[decision.status]}</i>
      </div>
      {decision.finalName && <p className="ag-cand-name">{decision.finalName}</p>}
      <p className="ag-cand-meta">Узнан {[byPhoto && 'по фото', byName && 'по наименованию'].filter(Boolean).join(' и ')} · в инвойсе: «{decision.invoiceName || decision.supplierName}»{decision.verifiedByName && <> · проверил {decision.verifiedByName}</>}</p>
      {decision.quote && <blockquote>{decision.quote}</blockquote>}
      {!decision.nomenclature && <p className="ag-warn">Кода нет в текущей номенклатуре — проверьте.</p>}
      <div className="ag-cand-actions">
        {applied.code === decision.code ? <span className="ag-done">✓ применён</span> : <button type="button" onClick={() => onApplyCode(decision.code, false)} disabled={busy || !decision.nomenclature}>Применить код</button>}
        {decision.nomenclature?.detailsUrl && <a href={decision.nomenclature.detailsUrl} target="_blank" rel="noreferrer">Подробнее ↗</a>}
        <Link href={`${paths.decisions}?id=${decision.id}`}>В базе решений</Link>
      </div>
    </div>)}
    {skipped && <button type="button" className="ag-secondary" onClick={onRerun} disabled={busy}>Всё равно подобрать ИИ</button>}
  </div>
}

function ResultCard({ result, applied, busy, onApplyCode, onApplyName }: { result: AgentResult; applied: { code?: string; name?: string }; busy: boolean; onApplyCode: (code: string, record: boolean) => void; onApplyName: (name: string) => void }) {
  return <div className="ag-result">
    <h3>Итог подбора</h3>
    {result.description && <p className="ag-desc">{result.description}</p>}
    {result.candidates.map((candidate) => <div key={candidate.code} className={`ag-cand ${candidate.verdict}`}>
      <div className="ag-cand-top">
        <CodeLine code={candidate.code} name={candidate.nomenclature?.name} />
        <i className={`ag-verdict ${candidate.verdict}`}>{verdictLabels[candidate.verdict]}</i>
      </div>
      {candidate.nomenclature && <p className="ag-rates">Пошлина ЕТТ {candidate.nomenclature.tariffText || 'не указана'} · НДС {candidate.nomenclature.vatPercent || '16'}%</p>}
      {candidate.quote && <blockquote>{candidate.quote}<cite>{candidate.quoteSource === 'page' ? 'со страницы кода (TWS.BY)' : 'из наименования кода в номенклатуре'}</cite></blockquote>}
      {candidate.reason && <p className="ag-reason">{candidate.reason}</p>}
      {candidate.check && <p className="ag-check"><b>Уточнить:</b> {candidate.check}</p>}
      <div className="ag-cand-actions">
        {candidate.verdict !== 'no' && (applied.code === candidate.code ? <span className="ag-done">✓ применён</span> : <button type="button" onClick={() => onApplyCode(candidate.code, true)} disabled={busy}>Применить код</button>)}
        {candidate.nomenclature?.detailsUrl && <a href={candidate.nomenclature.detailsUrl} target="_blank" rel="noreferrer">Подробнее ↗</a>}
      </div>
    </div>)}
    {result.name && <div className="ag-name">
      <small>Наименование</small>
      <p>{result.name}</p>
      {applied.name === result.name ? <span className="ag-done">✓ применено</span> : <button type="button" className="ag-secondary" onClick={() => onApplyName(result.name)} disabled={busy}>Применить наименование</button>}
    </div>}
    {result.notes && <p className="ag-check">{result.notes}</p>}
    {(result.droppedCodes.length > 0 || result.quotesRemoved > 0) && <p className="ag-notice">
      {result.droppedCodes.length > 0 && <>Программа убрала коды, которых нет в номенклатуре: {result.droppedCodes.map(formatCode).join(', ')}. </>}
      {result.quotesRemoved > 0 && <>Цитат, не найденных в источниках, убрано: {result.quotesRemoved}.</>}
    </p>}
  </div>
}
