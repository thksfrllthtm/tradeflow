import { FormEvent, useCallback, useEffect, useId, useRef, useState } from 'react'
import { PhotoLightbox } from '@/shared/components/PhotoLightbox/PhotoLightbox'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { LinkText } from '@/shared/components/LinkText/LinkText'
import { errorMessage } from '@/shared/lib/errorMessage'
import { resizePhoto } from '@/shared/lib/resizePhoto'
import { decimalText } from '@/shared/lib/rounding'
import { Link } from '@/shared/router/Link'
import { paths } from '@/shared/router/router'
import { GoodsItemInput } from '@/shared/types/goods'
import { photosOf } from '@/shared/lib/itemPhotos'
import { decisionsApi } from '@/features/decisions/decisionsApi'
import { AgentCandidate, AgentItem, AgentMessage, AgentResult, AgentSession, AgentStatus, SameProductMatch, SimilarMatch, agentApi } from './agentApi'
import { DocumentsAnswer, DocumentsButton, documentsRequested, scrollToDocuments } from './DocumentsAnswer'
import '@/shared/styles/drawer.css'
import './agent.css'
import { CodeDecision, decisionStatusLabels } from '@/shared/tnved/decisions'
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
  open_goods_help: '📋', open_link: '🔗', search_in_document: '🔎',
}

const verdictLabels: Record<AgentCandidate['verdict'], string> = { fits: 'подходит', doubtful: 'возможно', no: 'не подходит' }

export function agentItemFrom(item: GoodsItemInput): AgentItem {
  return {
    name: item.name ?? '', supplierName: item.supplierName && item.supplierName !== item.name ? item.supplierName : '', otherNames: item.otherNames ?? [], code: item.code ?? '',
    quantity: item.pairs ?? '', places: item.places ?? '', grossKg: item.grossKg ?? '', netKg: item.netKg ?? '', pricePerUnit: item.pricePerPair ?? '', unit: 'шт',
  }
}

// Фото строки для ИИ: до трёх, уменьшенные до 1024 px.
export async function agentPhotosFrom(item: GoodsItemInput): Promise<string[]> {
  return Promise.all(photosOf(item).slice(0, 3).map((photo) => resizePhoto(photo)))
}

// Чат подбора кода ТН ВЭД для одной строки сметы. При открытии программа только ищет товар в
// базе решений (тот же товар и похожие по фото); агента сотрудник запускает кнопкой внизу.
// Подбор идёт на сервере — панель можно закрыть.
export function AgentPanel({ estimateId, item, rowNumber, onClose, onApplyCode, onApplyName, onChanged }: Props) {
  const itemId = item.id ?? ''
  const [session, setSession] = useState<AgentSession | null>(null)
  const [status, setStatus] = useState<AgentStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [text, setText] = useState('')
  // name — применённое «русское / 中文», nameRu — русская часть, с которой его перевели.
  const [applied, setApplied] = useState<Applied>({})
  const [lightbox, setLightbox] = useState<{ photos: string[]; index: number } | null>(null)
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

  // Новый подбор строки: сначала только поиск в базе решений.
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

  // Открытие: последний подбор строки; если его нет — поиск в базе.
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

  // Пока идёт поиск или подбор — забираем новые сообщения.
  const sessionId = session?.id
  const running = session?.status === 'running'
  const searching = session?.status === 'searching'
  const working = running || searching
  useEffect(() => {
    if (!sessionId || !working) return
    let active = true
    const timer = window.setInterval(() => {
      agentApi.get(sessionId, lastId.current).then((next) => {
        if (!active) return
        accept(next, false)
        if (next.status !== 'running' && next.status !== 'searching') changedRef.current()
      }).catch(() => undefined)
    }, pollInterval)
    return () => { active = false; window.clearInterval(timer) }
  }, [sessionId, working, accept])

  // Новые сообщения — прокрутка вниз.
  const messageCount = session?.messages.length ?? 0
  useEffect(() => {
    bodyRef.current?.scrollTo({ top: bodyRef.current.scrollHeight, behavior: 'smooth' })
  }, [messageCount, session?.status])

  useEffect(() => {
    if (lightbox) return
    const onKeyDown = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose, lightbox])

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

  // «Документы для ввоза»: ответ для этого кода уже есть — прокрутка к нему, иначе запуск агента.
  async function requestDocuments(code: string) {
    if (!session || busy || working) return
    if (scrollToDocuments(bodyRef.current, code)) return
    setBusy(true); setError('')
    try {
      accept(await agentApi.documents(session.id, code), false)
      changedRef.current()
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false)
    }
  }

  async function applyCode(code: string, record: boolean) {
    setBusy(true); setError('')
    try {
      await onApplyCode(code)
      setApplied((current) => ({ ...current, code }))
      if (record && session) {
        await agentApi.apply(session.id, code, applied.name ?? session.result?.name ?? '')
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
      await decisionsApi.create({ invoiceName: item.name ?? '', supplierName: item.supplierName !== item.name ? item.supplierName : (item.otherNames ?? []).join(' / '), finalName: item.standardName || '', code: item.code, photos, thumb, estimateId, itemId })
      setSavedRowCode(item.code)
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false)
    }
  }

  // Русская часть (возможно, исправленная) → перевод на китайский → «русское / 中文» в строку сметы.
  async function applyName(russian: string) {
    if (!session) return
    setBusy(true); setError('')
    try {
      const { name } = await agentApi.name(session.id, russian)
      onApplyName(name)
      setApplied((current) => ({ ...current, name, nameRu: russian }))
      accept(await agentApi.get(session.id, lastId.current), false)
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false)
    }
  }

  // Запись базы решений: код и (если передано) её наименование — оно уже «русское / 中文», без перевода.
  async function applyDecision(code: string, name: string) {
    if (code) await applyCode(code, false)
    if (name) {
      onApplyName(name)
      setApplied((current) => ({ ...current, name, nameRu: russianPart(name) }))
    }
  }

  async function openDecisionPhotos(id: number) {
    try {
      const decision = await decisionsApi.get(id)
      if (decision.photos.length) setLightbox({ photos: decision.photos, index: 0 })
    } catch (reason) {
      setError(errorMessage(reason))
    }
  }

  const photos = session?.photos ?? []
  const messages = session?.messages ?? []
  const input = messages.find((message) => message.kind === 'input')
  const match = messages.find((message): message is Extract<AgentMessage, { kind: 'match' }> => message.kind === 'match')
  const similar = messages.find((message): message is Extract<AgentMessage, { kind: 'similar' }> => message.kind === 'similar')
  const blocks = chatBlocks(messages)
  // Проверенный код найден в базе, агента не запускали: внизу — «Всё равно подобрать ИИ».
  const skippedByBase = Boolean(match?.content.skipped) && session?.runs === 0 && session.status === 'done'
  const configured = status?.configured !== false
  const canWrite = Boolean(session) && !working && configured && session?.status !== 'ready' && !skippedByBase
  const decisionActions: DecisionActions = {
    applied, busy, rowName: (item.name ?? '').trim(), onOpenPhotos: (id) => void openDecisionPhotos(id),
    onApply: (code, name) => void applyDecision(code, name),
  }

  return <div className="drawer-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <aside className="drawer ag-drawer" role="dialog" aria-modal="true" aria-labelledby="ag-title">
      <header className="drawer-head ag-head">
        <div>
          <h2 id="ag-title">Подбор кода · строка {rowNumber}</h2>
          <p>{session ? statusText(session) : status && !status.configured ? 'Подбор кода не настроен на сервере' : 'Загрузка…'}</p>
        </div>
        <button type="button" className="drawer-close" onClick={onClose} aria-label="Закрыть" title="Закрыть (Esc)">×</button>
      </header>

      <div className="drawer-body ag-body" ref={bodyRef}>
        {loading && !session && <p className="ag-muted">Загрузка…</p>}
        {status && !status.configured && <p className="ag-error">Подбор кода не настроен: на сервере нет ключа (TNVED_AI_API_KEY или DEEPSEEK_API_KEY).</p>}
        {input?.kind === 'input' && <ItemCard item={input.content.item} photos={photos} onOpenPhoto={(index) => setLightbox({ photos, index })} />}
        {session && <BaseCard match={match?.content.matches ?? []} similar={similar?.content} searching={searching} hasPhotos={photos.length > 0} actions={decisionActions} />}
        {blocks.map((block) => block.type === 'steps'
          ? <StepsGroup key={`steps-${block.messages[0].id}`} messages={block.messages} live={running && block === blocks[blocks.length - 1]} />
          : <MessageView key={block.message.id} message={block.message} applied={applied} busy={busy}
            onApplyCode={applyCode} onApplyName={(name) => void applyName(name)}
            documentsDisabled={busy || working || !configured} onDocuments={(code) => void requestDocuments(code)} />)}
        {running && <div className="ag-typing" aria-live="polite"><span /><span /><span /> {documentsRequested(messages) ? 'агент читает справку tnved.info и акты' : 'агент работает'} — панель можно закрыть</div>}
        {error && <p className="ag-error" role="alert">{error}</p>}
      </div>

      <footer className="drawer-foot ag-foot">
        {!session && !loading && configured && <button type="button" className="ag-primary" onClick={() => void start()} disabled={busy}>Найти в базе решений</button>}
        {searching && <p className="ag-wait"><span className="ag-spinner" aria-hidden="true" />Ищу этот товар в базе решений…</p>}
        {(session?.status === 'ready' || skippedByBase) && <div className="ag-launch">
          <button type="button" className="ag-primary" disabled={busy || !configured}
            onClick={() => session && void act(() => skippedByBase ? agentApi.rerun(session.id) : agentApi.begin(session.id))}>
            {skippedByBase ? 'Всё равно подобрать заново' : 'Начать подбор кода'}
          </button>
          <span>{skippedByBase ? 'Проверенный код этого товара уже есть в базе.' : 'Агент изучит номенклатуру и пояснения к кодам — обычно 1–2 минуты.'}
            {status && status.dailyLimit > 0 && <> Сегодня: {status.usedToday} из {status.dailyLimit}.</>}</span>
        </div>}
        {running && <div className="ag-launch"><button type="button" className="ag-ghost" onClick={() => session && void act(() => agentApi.cancel(session.id))} disabled={busy}>Остановить</button><span>Агент подбирает код…</span></div>}
        {canWrite && <form className="ag-form" onSubmit={(event) => void send(event)}>
          <input value={text} onChange={(event) => setText(event.target.value)} maxLength={1000} disabled={busy}
            placeholder={session?.status === 'waiting' ? 'Ответьте на вопрос агента…' : 'Уточнить: «это латекс», «посмотри 3926»…'} aria-label="Сообщение агенту" />
          <button type="submit" disabled={busy || !text.trim()}>Отправить</button>
        </form>}
        <div className="ag-foot-row">
          {session && !working && <button type="button" className="ag-link" onClick={() => void start()} disabled={busy || !configured}>Начать заново</button>}
          {session && session.runs > 0 && <span className="ag-cost" title={`Модель ${session.model}, шагов: ${session.steps}`}>стоимость ${session.costUsd}</span>}
          {/^\d{10}$/.test(item.code) && (savedRowCode === item.code
            ? <span className="ag-done">✓ код строки в базе решений</span>
            : <button type="button" className="ag-link" onClick={() => void saveRowCode()} disabled={busy} title="Записать код, который сейчас стоит в строке, в базу решений (ждёт проверки администратором)">Код строки → в базу</button>)}
          <span className="ag-disclaimer">Код выбирает декларант</span>
        </div>
      </footer>
    </aside>
    {lightbox && <PhotoLightbox photos={lightbox.photos} startIndex={lightbox.index} onClose={() => setLightbox(null)} />}
  </div>
}

function statusText(session: AgentSession) {
  switch (session.status) {
    case 'searching': return 'Поиск в базе решений…'
    case 'ready': return 'Поиск в базе закончен'
    case 'running': return 'Агент подбирает код'
    case 'waiting': return 'Агент ждёт ответа на вопрос'
    case 'done': return session.result ? 'Подбор готов — проверьте и примените код' : 'Найдено в базе решений'
    case 'error': return 'Подбор прервался с ошибкой'
    case 'cancelled': return 'Подбор остановлен'
  }
}

type Applied = { code?: string; name?: string; nameRu?: string }

// Сообщения чата после поиска в базе: шаги агента (инструменты и его пояснения) — одной
// сворачиваемой группой, остальное — по одному.
type ChatBlock = { type: 'steps'; messages: AgentMessage[] } | { type: 'message'; message: AgentMessage }

function chatBlocks(messages: AgentMessage[]): ChatBlock[] {
  const blocks: ChatBlock[] = []
  for (const message of messages) {
    if (message.kind === 'input' || message.kind === 'match' || message.kind === 'similar' || message.kind === 'start') continue
    if (message.kind === 'tool' || message.kind === 'text') {
      const last = blocks[blocks.length - 1]
      if (last?.type === 'steps') last.messages.push(message)
      else blocks.push({ type: 'steps', messages: [message] })
      continue
    }
    blocks.push({ type: 'message', message })
  }
  return blocks
}

// Ход подбора: свёрнут после итога, пока агент работает — открыт.
function StepsGroup({ messages, live }: { messages: AgentMessage[]; live: boolean }) {
  const [open, setOpen] = useState(false)
  const steps = messages.filter((message) => message.kind === 'tool').length
  const expanded = open || live
  return <section className={expanded ? 'ag-steps open' : 'ag-steps'}>
    <button type="button" className="ag-steps-toggle" onClick={() => setOpen((value) => !value)} aria-expanded={expanded} disabled={live}>
      <span>Ход подбора</span><small>{steps} {steps === 1 ? 'шаг' : steps < 5 && steps > 0 ? 'шага' : 'шагов'}</small>
      {!live && <i aria-hidden="true">{expanded ? '▴' : '▾'}</i>}
    </button>
    {expanded && <ol className="ag-steps-list">
      {messages.map((message) => {
        if (message.kind === 'text') return <li key={message.id} className="ag-thought">{message.content.text}</li>
        if (message.kind !== 'tool') return null
        const { tool, label, detail, url, failed } = message.content
        return <li key={message.id} className={failed ? 'ag-step failed' : 'ag-step'}>
          <span className="ag-step-icon" aria-hidden="true">{toolIcons[tool] ?? '•'}</span>
          <span className="ag-step-label">{label}</span>
          <span className="ag-step-detail">{detail}</span>
          {url && <a href={url} target="_blank" rel="noreferrer" title="Открыть страницу кода">↗</a>}
        </li>
      })}
    </ol>}
  </section>
}

type MessageProps = {
  message: AgentMessage
  applied: Applied
  busy: boolean
  onApplyCode: (code: string, record: boolean) => void
  onApplyName: (name: string) => void
  documentsDisabled: boolean
  onDocuments: (code: string) => void
}

function MessageView({ message, applied, busy, onApplyCode, onApplyName, documentsDisabled, onDocuments }: MessageProps) {
  switch (message.kind) {
    case 'question': return <div className="ag-msg ag-question"><b>Вопрос агента</b>{message.content.text}</div>
    case 'user': return <div className="ag-msg ag-user">{message.content.text}</div>
    case 'documents': return <div className="ag-msg ag-user">Документы для ввоза · {formatCode(message.content.code)}</div>
    case 'notice': return <p className="ag-notice">{message.content.text}</p>
    case 'error': return <p className="ag-error">{message.content.message}</p>
    case 'rerun': return <p className="ag-divider"><span>подбор заново</span></p>
    case 'result': return message.content.documentsCode ? <DocumentsAnswer result={message.content} />
      : <ResultCard result={message.content} applied={applied} busy={busy} onApplyCode={onApplyCode} onApplyName={onApplyName} documentsDisabled={documentsDisabled} onDocuments={onDocuments} />
    default: return null
  }
}

function ItemCard({ item, photos, onOpenPhoto }: { item: AgentItem; photos: string[]; onOpenPhoto: (index: number) => void }) {
  const unitNet = perUnit(item.netKg || item.grossKg, item.quantity)
  const meta = [
    item.code && `код из файла ${formatCode(item.code)}`,
    item.quantity && `${item.quantity} ${item.unit}`,
    unitNet && `1 ${item.unit}: ${unitNet} кг`,
    item.pricePerUnit && `${item.pricePerUnit} $`,
    photos.length === 0 && 'без фото',
  ].filter(Boolean).join(' · ')
  return <section className="ag-item">
    {photos.length > 0 && <div className="ag-item-photos">{photos.map((photo, index) => <button key={index} type="button" className="ag-photo" onClick={() => onOpenPhoto(index)} title="Открыть фото крупно" aria-label={`Открыть фото товара ${index + 1} крупно`}><img src={photo} alt="" /></button>)}</div>}
    <div className="ag-item-text">
      <strong>{item.name || item.supplierName || 'Без наименования'}</strong>
      {item.supplierName && item.supplierName !== item.name && <span>{item.supplierName}</span>}
      {item.otherNames?.map((other) => <span key={other}>{other}</span>)}
      {meta && <small>{meta}</small>}
    </div>
  </section>
}

function perUnit(total: string, quantity: string) {
  const value = Number(String(total).replace(',', '.'))
  const count = Number(String(quantity).replace(',', '.'))
  if (!(value > 0) || !(count > 0)) return ''
  return decimalText(value / count, 4).replace('.', ',')
}

function CodeLine({ code, name }: { code: string; name?: string }) {
  const parts = name ? nameParts(name) : []
  return <div className="ag-code-line">
    <CopyCode code={code} className="ag-code" />
    {parts.length > 0 && <span title={parts.join(' › ')}>{parts.slice(-2).join(' › ')}</span>}
  </div>
}

// onApply(code, name): пустая строка — это не применять (только код или только наименование).
type DecisionActions = { applied: Applied; busy: boolean; rowName: string; onApply: (code: string, name: string) => void; onOpenPhotos: (id: number) => void }

const similarVerdicts: Record<SimilarMatch['verdict'], string> = { same: 'тот же товар', similar: 'похожий товар', '': '' }

// Что нашлось в базе решений: тот же товар (по фото или наименованию) и похожие по фото.
function BaseCard({ match, similar, searching, hasPhotos, actions }: { match: SameProductMatch[]; similar?: { matches: SimilarMatch[]; description: string }; searching: boolean; hasPhotos: boolean; actions: DecisionActions }) {
  const similarMatches = similar?.matches ?? []
  const found = match.length + similarMatches.length
  return <section className="ag-card ag-base">
    <header className="ag-card-head">
      <h3>В базе решений</h3>
      <small>{searching ? 'ищу…' : found ? `найдено: ${found}` : 'ничего не найдено'}</small>
    </header>
    {match.map(({ decision, byPhoto, byName, photoDiffers }) => <DecisionRow key={`same-${decision.id}`} decision={decision} actions={actions}
      badge={photoDiffers ? <i className="ag-pill doubtful" title="Наименование то же, но фото другое — фото главнее">то же наименование</i> : <i className="ag-pill fits">тот же товар</i>}
      note={photoDiffers ? 'Наименование совпало, но на фото, похоже, другой товар — сверьте фото, прежде чем применять код.' : undefined}
      meta={`узнан ${[byPhoto && 'по фото', byName && 'по наименованию'].filter(Boolean).join(' и ')} · в инвойсе: «${decision.invoiceName || decision.supplierName}»`} />)}
    {similarMatches.map((item) => <DecisionRow key={`similar-${item.decision.id}`} decision={item.decision} actions={actions}
      badge={<>{item.verdict && <i className={`ag-pill ${item.verdict === 'same' ? 'fits' : 'doubtful'}`}>{similarVerdicts[item.verdict]}</i>}<i className="ag-pill plain" title="Похожесть по фото">{item.score}%</i></>}
      meta={[item.byPhoto && 'снимок похож', item.byDescription && item.description && `в базе: ${item.description}`].filter(Boolean).join(' · ')}
      note={item.why} />)}
    {!searching && found === 0 && <p className="ag-empty-line">{hasPhotos ? 'Такого товара и похожих по фото в базе нет.' : 'Такого товара в базе нет. Похожие ищутся по фото — у строки фото нет.'}</p>}
    {similar?.description && <p className="ag-seen">На фото: {similar.description}</p>}
  </section>
}

function DecisionRow({ decision, actions, badge, meta, note }: { decision: CodeDecision; actions: DecisionActions; badge: React.ReactNode; meta: string; note?: string }) {
  const { applied, busy, rowName, onApply, onOpenPhotos } = actions
  const name = decision.finalName.trim()
  const codeDone = applied.code === decision.code
  const nameDone = !name || applied.name === name || rowName === name
  const noCode = !decision.nomenclature
  return <article className="ag-row">
    {decision.thumb
      ? <button type="button" className="ag-photo ag-thumb" onClick={() => onOpenPhotos(decision.id)} title="Открыть фото из базы крупно" aria-label="Открыть фото записи базы крупно"><img src={decision.thumb} alt="" /></button>
      : <span className="ag-thumb" aria-hidden="true" />}
    <div className="ag-row-body">
      <div className="ag-row-top">
        <CodeLine code={decision.code} name={decision.nomenclature?.name} />
        <span className="ag-pills">{badge}</span>
      </div>
      {decision.finalName && <p className="ag-row-name">{decision.finalName}</p>}
      <p className="ag-row-meta">{decisionStatusLabels[decision.status]}{decision.verifiedByName && ` · проверил ${decision.verifiedByName}`}{meta && ` · ${meta}`}</p>
      {note && <p className="ag-row-note">{note}</p>}
      {!decision.nomenclature && <p className="ag-warn">Кода нет в текущей номенклатуре — проверьте.</p>}
      <div className="ag-actions">
        {codeDone && nameDone && <span className="ag-done">{name ? '✓ код и наименование применены' : '✓ код применён'}</span>}
        {!codeDone && <button type="button" className="ag-apply" onClick={() => onApply(decision.code, nameDone ? '' : name)} disabled={busy || noCode}>{nameDone ? 'Применить код' : 'Применить код и наименование'}</button>}
        {!codeDone && !nameDone && <button type="button" className="ag-text" onClick={() => onApply(decision.code, '')} disabled={busy || noCode}>только код</button>}
        {codeDone && !nameDone && <button type="button" className="ag-apply ghost" onClick={() => onApply('', name)} disabled={busy}>Применить наименование</button>}
        {decision.nomenclature?.detailsUrl && <a href={decision.nomenclature.detailsUrl} target="_blank" rel="noreferrer">Подробнее ↗</a>}
        <Link href={`${paths.decisions}?id=${decision.id}`}>В базе решений</Link>
      </div>
    </div>
  </article>
}

// Русская часть до « / » с китайским текстом (у старых подборов агент писал и перевод).
function russianPart(name: string) {
  const index = name.lastIndexOf('/')
  return index > 0 && /\p{Script=Han}/u.test(name.slice(index)) ? name.slice(0, index).trim() : name.trim()
}

// Наименование агента можно исправить; перевод на китайский запрашивается только по кнопке.
function NameEditor({ suggested, applied, busy, onApply }: { suggested: string; applied: Applied; busy: boolean; onApply: (name: string) => void }) {
  const inputId = useId()
  const [draft, setDraft] = useState(() => russianPart(suggested))
  const value = draft.trim()
  const done = Boolean(applied.name) && applied.nameRu === value
  return <div className="ag-name">
    <label htmlFor={inputId}>Наименование для декларации</label>
    <textarea id={inputId} value={draft} onChange={(event) => setDraft(event.target.value)} rows={2} maxLength={200} disabled={busy} />
    {done && <p className="ag-name-final">{applied.name}</p>}
    <div className="ag-actions">
      {done
        ? <span className="ag-done">✓ применено с переводом на китайский</span>
        : <button type="button" className="ag-apply ghost" onClick={() => onApply(value)} disabled={busy || !value}>{busy ? 'Перевожу…' : 'Применить наименование'}</button>}
      {!done && <span className="ag-muted">перевод на китайский добавится при применении</span>}
    </div>
  </div>
}

type CandidateProps = { candidate: AgentCandidate; applied: Applied; busy: boolean; onApplyCode: (code: string, record: boolean) => void; documentsDisabled: boolean; onDocuments: (code: string) => void }

function CandidateCard({ candidate, applied, busy, onApplyCode, documentsDisabled, onDocuments }: CandidateProps) {
  return <article className={`ag-cand ${candidate.verdict}`}>
    <div className="ag-row-top">
      <CodeLine code={candidate.code} name={candidate.nomenclature?.name} />
      <i className={`ag-pill ${candidate.verdict}`}>{verdictLabels[candidate.verdict]}</i>
    </div>
    {candidate.nomenclature && <p className="ag-rates">Пошлина {candidate.nomenclature.tariffText || 'не указана'} · НДС {candidate.nomenclature.vatPercent || '16'}%</p>}
    {candidate.nomenclature?.antiDumping?.map((measure) => <p key={measure.number} className={measure.exact && !measure.suspended ? 'ag-antidumping' : 'ag-antidumping maybe'}>
      {measure.suspended ? 'Антидемпинг временно не применяется' : measure.exact ? 'Антидемпинг' : 'Возможен антидемпинг'}: {measure.number} «{measure.shortName}» — {measure.countries.map((country) => `${measure.countryNames && measure.countries.length === 1 ? measure.countryNames : country} ${(measure.rates[country] ?? '').replace('.', ',')}%`).join(', ')}
      {!measure.exact && ' (мера на часть товаров позиции — сверьте описание)'}
    </p>)}
    {candidate.quote && <blockquote>{candidate.quote}<cite>{candidate.quoteSource === 'page' ? 'со страницы кода (TWS.BY)' : candidate.quoteSource === 'help' ? 'справка по товару (tnved.info)' : 'из наименования кода в номенклатуре'}</cite></blockquote>}
    {candidate.reason && <p className="ag-reason"><LinkText text={candidate.reason} /></p>}
    {candidate.check && <p className="ag-check"><b>Уточнить:</b> <LinkText text={candidate.check} /></p>}
    <div className="ag-actions">
      {candidate.verdict !== 'no' && (applied.code === candidate.code ? <span className="ag-done">✓ применён</span> : <button type="button" className="ag-apply" onClick={() => onApplyCode(candidate.code, true)} disabled={busy}>Применить код</button>)}
      {candidate.verdict !== 'no' && <DocumentsButton code={candidate.code} disabled={documentsDisabled} onClick={onDocuments} />}
      {candidate.nomenclature?.detailsUrl && <a href={candidate.nomenclature.detailsUrl} target="_blank" rel="noreferrer">Подробнее ↗</a>}
    </div>
  </article>
}

// Итог: подходящие коды — сразу, неподходящие — свёрнуты; наименование и замечания.
type ResultProps = { result: AgentResult; applied: Applied; busy: boolean; onApplyCode: (code: string, record: boolean) => void; onApplyName: (name: string) => void; documentsDisabled: boolean; onDocuments: (code: string) => void }

function ResultCard({ result, applied, busy, onApplyCode, onApplyName, documentsDisabled, onDocuments }: ResultProps) {
  const main = result.candidates.filter((candidate) => candidate.verdict !== 'no')
  const rejected = result.candidates.filter((candidate) => candidate.verdict === 'no')
  return <section className="ag-card ag-result">
    <header className="ag-card-head"><h3>Итог подбора</h3></header>
    {result.description && <p className="ag-desc"><LinkText text={result.description} /></p>}
    {main.map((candidate) => <CandidateCard key={candidate.code} candidate={candidate} applied={applied} busy={busy} onApplyCode={onApplyCode} documentsDisabled={documentsDisabled} onDocuments={onDocuments} />)}
    {rejected.length > 0 && <details className="ag-more">
      <summary>Не подходят: {rejected.map((candidate) => formatCode(candidate.code)).join(', ')}</summary>
      {rejected.map((candidate) => <CandidateCard key={candidate.code} candidate={candidate} applied={applied} busy={busy} onApplyCode={onApplyCode} documentsDisabled={documentsDisabled} onDocuments={onDocuments} />)}
    </details>}
    {result.name && <NameEditor suggested={result.name} applied={applied} busy={busy} onApply={onApplyName} />}
    {result.notes && <p className="ag-note"><LinkText text={result.notes} /></p>}
    {(result.droppedCodes.length > 0 || result.quotesRemoved > 0) && <p className="ag-notice">
      {result.droppedCodes.length > 0 && <>Программа убрала коды, которых нет в номенклатуре: {result.droppedCodes.map(formatCode).join(', ')}. </>}
      {result.quotesRemoved > 0 && <>Цитат, не найденных в источниках, убрано: {result.quotesRemoved}.</>}
    </p>}
  </section>
}
