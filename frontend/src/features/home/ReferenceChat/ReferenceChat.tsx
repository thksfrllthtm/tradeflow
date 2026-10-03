import { ClipboardEvent, DragEvent, FormEvent, KeyboardEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { PhotoLightbox } from '@/shared/components/PhotoLightbox/PhotoLightbox'
import { errorMessage } from '@/shared/lib/errorMessage'
import { resizePhoto } from '@/shared/lib/resizePhoto'
import { formatCode, nameParts } from '@/shared/tnved/codes'
import { AgentCandidate, AgentMessage, AgentResult, AgentSession, AgentStatus, ReferenceSummary, agentApi } from '@/features/goods/agent/agentApi'
import './ReferenceChat.css'

const pollInterval = 1500
const maxPhotos = 3
const examples = ['Лопата штыковая стальная с черенком', 'Перчатки трикотажные с покрытием ПВХ', 'Чем отличаются 6116 10 200 0 и 6116 10 800 0?']
const verdictLabels: Record<AgentCandidate['verdict'], string> = { fits: 'подходит', doubtful: 'возможно', no: 'не подходит' }
const toolIcons: Record<string, string> = { search_nomenclature: '🔍', get_codes: '✔️', list_heading: '📂', open_details: '📄', search_decisions: '🗂️', search_directory: '📚' }

const isWorking = (session: AgentSession | null) => session?.status === 'running' || session?.status === 'searching'

// Чат-справочник ТН ВЭД на главной: фото и/или вопрос о товаре → коды и объяснение, почему они.
// Без наименования и записи в базу решений. Слева — прошлые вопросы сотрудника.
export function ReferenceChat() {
  const [status, setStatus] = useState<AgentStatus | null>(null)
  const [history, setHistory] = useState<ReferenceSummary[]>([])
  const [session, setSession] = useState<AgentSession | null>(null)
  const [text, setText] = useState('')
  const [photos, setPhotos] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [dragging, setDragging] = useState(false)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [lightbox, setLightbox] = useState<{ photos: string[]; index: number } | null>(null)
  const lastId = useRef(0)
  const threadRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  const loadHistory = useCallback(() => agentApi.history().then((list) => setHistory(list.items)).catch(() => undefined), [])
  useEffect(() => {
    agentApi.status().then(setStatus).catch(() => undefined)
    void loadHistory()
  }, [loadHistory])

  const accept = useCallback((next: AgentSession, replace: boolean) => {
    const newest = next.messages.length ? next.messages[next.messages.length - 1].id : 0
    lastId.current = replace ? newest : Math.max(lastId.current, newest)
    setSession((current) => {
      if (replace || !current || current.id !== next.id) return next
      const known = new Set(current.messages.map((message) => message.id))
      return { ...next, photos: next.photos ?? current.photos, messages: [...current.messages, ...next.messages.filter((message) => !known.has(message.id))] }
    })
  }, [])

  // Пока идёт поиск или подбор — забираем новые сообщения; закончился — обновляем историю.
  const sessionId = session?.id
  const working = isWorking(session)
  useEffect(() => {
    if (!sessionId || !working) return
    let active = true
    const timer = window.setInterval(() => {
      agentApi.get(sessionId, lastId.current).then((next) => {
        if (!active) return
        accept(next, false)
        if (!isWorking(next)) { void loadHistory(); agentApi.status().then(setStatus).catch(() => undefined) }
      }).catch(() => undefined)
    }, pollInterval)
    return () => { active = false; window.clearInterval(timer) }
  }, [sessionId, working, accept, loadHistory])

  const messageCount = session?.messages.length ?? 0
  useEffect(() => {
    threadRef.current?.scrollTo({ top: threadRef.current.scrollHeight, behavior: 'smooth' })
  }, [messageCount, session?.status])

  async function addFiles(files: FileList | File[]) {
    const images = [...files].filter((file) => file.type === 'image/jpeg' || file.type === 'image/png')
    if (images.length === 0) { setError('Фото — JPG или PNG'); return }
    setError('')
    const loaded = await Promise.all(images.slice(0, maxPhotos).map((file) => new Promise<string>((resolve) => {
      const reader = new FileReader()
      reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '')
      reader.onerror = () => resolve('')
      reader.readAsDataURL(file)
    })))
    const resized = await Promise.all(loaded.filter(Boolean).map((photo) => resizePhoto(photo)))
    setPhotos((current) => [...current, ...resized].slice(0, maxPhotos))
    inputRef.current?.focus()
  }

  function onPaste(event: ClipboardEvent<HTMLTextAreaElement>) {
    const files = [...event.clipboardData.files]
    if (files.length) { event.preventDefault(); void addFiles(files) }
  }

  function onDrop(event: DragEvent<HTMLElement>) {
    event.preventDefault()
    setDragging(false)
    if (event.dataTransfer.files.length) void addFiles(event.dataTransfer.files)
  }

  // Фото или нет активного вопроса — новый вопрос; иначе уточнение к текущему (агент помнит разговор).
  async function send(event?: FormEvent) {
    event?.preventDefault()
    const value = text.trim()
    if ((!value && photos.length === 0) || busy) return
    setBusy(true); setError('')
    try {
      if (!session || photos.length > 0) {
        accept(await agentApi.ask({ text: value, photos }), true)
        setPhotos([])
      } else {
        accept(await agentApi.message(session.id, value), false)
      }
      setText('')
      void loadHistory()
      agentApi.status().then(setStatus).catch(() => undefined)
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false)
    }
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); void send() }
  }

  async function open(id: number) {
    setHistoryOpen(false); setError('')
    try { accept(await agentApi.get(id), true) } catch (reason) { setError(errorMessage(reason)) }
  }

  async function remove(id: number) {
    if (!window.confirm('Удалить этот вопрос из истории?')) return
    try {
      await agentApi.removeQuestion(id)
      if (session?.id === id) setSession(null)
      void loadHistory()
    } catch (reason) { setError(errorMessage(reason)) }
  }

  function startNew() {
    setSession(null); setText(''); setPhotos([]); setError(''); setHistoryOpen(false)
    inputRef.current?.focus()
  }

  const configured = status?.configured !== false
  const placeholder = !session ? 'Опишите товар, вставьте фото (Ctrl+V) или перетащите его сюда…'
    : session.status === 'waiting' ? 'Ответьте на вопрос…' : photos.length ? 'Добавьте пояснение к фото — это будет новый вопрос' : 'Уточните: «а если покрытие латекс?» — или добавьте фото для нового вопроса'

  return <section className={dragging ? 'rc dragging' : 'rc'} aria-label="Справочник ТН ВЭД"
    onDragOver={(event) => { if (event.dataTransfer.types.includes('Files')) { event.preventDefault(); setDragging(true) } }}
    onDragLeave={(event) => { if (event.currentTarget === event.target) setDragging(false) }} onDrop={onDrop}>
    <header className="rc-head">
      <div className="rc-title">
        <span className="rc-logo" aria-hidden="true">
          <svg width="18" height="18" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"><path d="M4 5.5h12M4 10h8M4 14.5h5" /><circle cx="14.5" cy="13.5" r="2.6" /><path d="M16.4 15.4 18 17" /></svg>
        </span>
        <div>
          <h2>Справочник ТН ВЭД</h2>
          <p>Фото или описание товара — код и почему именно он</p>
        </div>
      </div>
      <div className="rc-head-actions">
        <button type="button" className="rc-ghost rc-history-toggle" onClick={() => setHistoryOpen((value) => !value)} aria-expanded={historyOpen}>История{history.length > 0 && ` · ${history.length}`}</button>
        {session && <button type="button" className="rc-ghost" onClick={startNew}>+ Новый вопрос</button>}
      </div>
    </header>

    <div className="rc-body">
      <aside className={historyOpen ? 'rc-history open' : 'rc-history'} aria-label="Прошлые вопросы">
        <p className="rc-history-title">Ваши вопросы</p>
        {history.length === 0 ? <p className="rc-history-empty">Здесь появятся ваши вопросы.</p>
          : <ul>{history.map((item) => <li key={item.id} className={session?.id === item.id ? 'active' : undefined}>
            <button type="button" onClick={() => void open(item.id)} title={item.title}>
              <span className="rc-history-name">{item.hasPhoto && <i aria-hidden="true">📷</i>}{item.title}</span>
              <small>{item.bestCode ? formatCode(item.bestCode) : item.status === 'running' || item.status === 'searching' ? 'ищет…' : item.status === 'waiting' ? 'ждёт ответа' : '—'} · {new Date(item.updatedAt).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })}</small>
            </button>
            <button type="button" className="rc-history-remove" onClick={() => void remove(item.id)} aria-label={`Удалить вопрос «${item.title}»`} title="Удалить">×</button>
          </li>)}</ul>}
      </aside>

      <div className="rc-main">
        <div className="rc-thread" ref={threadRef}>
          {!session ? <div className="rc-empty">
            <span className="rc-empty-icon" aria-hidden="true">
              <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="5" width="18" height="14" rx="3" /><circle cx="9" cy="11" r="2" /><path d="m21 16-4.5-4.5L7 19" /></svg>
            </span>
            <h3>Какой код у товара?</h3>
            <p>Перетащите фото, вставьте его через Ctrl+V или опишите товар. Сначала проверим базу решений компании, потом агент найдёт код в номенклатуре и объяснит выбор.</p>
            <div className="rc-examples">{examples.map((example) => <button key={example} type="button" onClick={() => { setText(example); inputRef.current?.focus() }}>{example}</button>)}</div>
          </div> : <Thread session={session} onPhoto={(list, index) => setLightbox({ photos: list, index })} />}
          {error && <p className="rc-error" role="alert">{error}</p>}
        </div>

        <form className="rc-composer" onSubmit={(event) => void send(event)}>
          {photos.length > 0 && <div className="rc-attachments">{photos.map((photo, index) => <span key={index} className="rc-attachment">
            <img src={photo} alt="" />
            <button type="button" onClick={() => setPhotos((current) => current.filter((_, at) => at !== index))} aria-label="Убрать фото">×</button>
          </span>)}</div>}
          <div className="rc-input-row">
            <label className="rc-attach" title="Добавить фото (JPG, PNG)">
              <svg width="18" height="18" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"><path d="m15.5 9.5-5.6 5.6a3.4 3.4 0 0 1-4.8-4.8l6-6a2.3 2.3 0 0 1 3.2 3.2l-6 6a1.1 1.1 0 0 1-1.6-1.6l5.3-5.3" /></svg>
              <input type="file" accept="image/jpeg,image/png" multiple onChange={(event) => { if (event.target.files) void addFiles(event.target.files); event.target.value = '' }} disabled={busy || photos.length >= maxPhotos} />
            </label>
            <textarea ref={inputRef} value={text} onChange={(event) => setText(event.target.value)} onKeyDown={onKeyDown} onPaste={onPaste} rows={1} maxLength={2000}
              placeholder={placeholder} aria-label="Вопрос о товаре" disabled={!configured || (working && photos.length === 0)} />
            <button type="submit" className="rc-send" disabled={busy || !configured || (!text.trim() && photos.length === 0) || (working && photos.length === 0)} aria-label="Отправить">
              <svg width="18" height="18" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round"><path d="M10 16V4M5 9l5-5 5 5" /></svg>
            </button>
          </div>
          <p className="rc-hint">
            {!configured ? 'ИИ не настроен на сервере.' : <>Enter — отправить, Shift+Enter — новая строка{status && status.dailyLimit > 0 && <> · сегодня {status.usedToday} из {status.dailyLimit}</>}</>}
            <span>Подсказка ИИ — код выбирает декларант</span>
          </p>
        </form>
      </div>
    </div>
    {dragging && <div className="rc-drop" aria-hidden="true"><span>Отпустите фото — оно добавится к вопросу</span></div>}
    {lightbox && <PhotoLightbox photos={lightbox.photos} startIndex={lightbox.index} onClose={() => setLightbox(null)} />}
  </section>
}

// Разговор: вопрос сотрудника, что нашлось в базе, ход поиска, вопросы агента, ответ.
function Thread({ session, onPhoto }: { session: AgentSession; onPhoto: (photos: string[], index: number) => void }) {
  const photos = session.photos ?? []
  const blocks = useMemo(() => threadBlocks(session.messages), [session.messages])
  const working = isWorking(session)
  return <>
    {blocks.map((block, index) => {
      if (block.type === 'steps') return <Steps key={`steps-${block.messages[0].id}`} messages={block.messages} live={working && index === blocks.length - 1} />
      const message = block.message
      switch (message.kind) {
        case 'input': return <div key={message.id} className="rc-user">
          {photos.length > 0 && <div className="rc-user-photos">{photos.map((photo, at) => <button key={at} type="button" onClick={() => onPhoto(photos, at)} aria-label="Открыть фото крупно"><img src={photo} alt="" /></button>)}</div>}
          {message.content.item.name && <p>{message.content.item.name}</p>}
        </div>
        case 'user': return <div key={message.id} className="rc-user"><p>{message.content.text}</p></div>
        case 'question': return <div key={message.id} className="rc-bot rc-question"><b>Уточните</b>{message.content.text}</div>
        case 'match': return message.content.matches.length > 0 ? <BaseHits key={message.id} items={message.content.matches.map((match) => ({ id: match.decision.id, code: match.decision.code, name: match.decision.finalName || match.decision.invoiceName, thumb: match.decision.thumb, note: match.byPhoto ? 'то же фото' : 'то же наименование', verified: match.decision.status === 'verified' }))} title="Этот товар уже оформляли" /> : null
        case 'similar': return message.content.matches.length > 0 ? <BaseHits key={message.id} items={message.content.matches.map((match) => ({ id: match.decision.id, code: match.decision.code, name: match.decision.finalName || match.decision.invoiceName, thumb: match.decision.thumb, note: match.verdict === 'same' ? `тот же товар · ${match.score}%` : `похожий · ${match.score}%`, verified: match.decision.status === 'verified' }))} title="Похожие товары в базе решений" /> : null
        case 'result': return <Answer key={message.id} result={message.content} previous={block.previous} />
        case 'notice': return <p key={message.id} className="rc-notice">{message.content.text}</p>
        case 'error': return <p key={message.id} className="rc-error">{message.content.message}</p>
        case 'rerun': return <p key={message.id} className="rc-divider"><span>заново</span></p>
        default: return null
      }
    })}
    {session.status === 'searching' && <p className="rc-typing"><span /><span /><span /> ищу в базе решений…</p>}
    {session.status === 'running' && <p className="rc-typing"><span /><span /><span /> агент ищет код…</p>}
  </>
}

// previous — прошлый ответ в разговоре: с ним сравниваются коды ответа на уточнение.
type Block = { type: 'steps'; messages: AgentMessage[] } | { type: 'message'; message: AgentMessage; previous?: AgentResult }

function threadBlocks(messages: AgentMessage[]): Block[] {
  const blocks: Block[] = []
  let previous: AgentResult | undefined
  for (const message of messages) {
    if (message.kind === 'result') {
      blocks.push({ type: 'message', message, previous })
      previous = message.content
      continue
    }
    if (message.kind === 'rerun') previous = undefined
    if (message.kind === 'start') continue
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

function Steps({ messages, live }: { messages: AgentMessage[]; live: boolean }) {
  const [open, setOpen] = useState(false)
  const count = messages.filter((message) => message.kind === 'tool').length
  const expanded = open || live
  return <div className={expanded ? 'rc-steps open' : 'rc-steps'}>
    <button type="button" onClick={() => setOpen((value) => !value)} disabled={live} aria-expanded={expanded}>
      Ход поиска · {count} {count === 1 ? 'шаг' : count > 1 && count < 5 ? 'шага' : 'шагов'}{!live && <i aria-hidden="true">{expanded ? '▴' : '▾'}</i>}
    </button>
    {expanded && <ol>{messages.map((message) => message.kind === 'tool'
      ? <li key={message.id} className={message.content.failed ? 'failed' : undefined}><span aria-hidden="true">{toolIcons[message.content.tool] ?? '•'}</span>{message.content.label}<small>{message.content.detail}</small>{message.content.url && <a href={message.content.url} target="_blank" rel="noreferrer">↗</a>}</li>
      : message.kind === 'text' ? <li key={message.id} className="thought">{message.content.text}</li> : null)}</ol>}
  </div>
}

type Hit = { id: number; code: string; name: string; thumb: string; note: string; verified: boolean }

function BaseHits({ title, items }: { title: string; items: Hit[] }) {
  return <div className="rc-base">
    <p className="rc-base-title">{title}</p>
    {items.map((item) => <div key={item.id} className="rc-base-row">
      {item.thumb ? <img src={item.thumb} alt="" /> : <span className="rc-base-nophoto" aria-hidden="true" />}
      <div>
        <CopyCode code={item.code} className="rc-code" />
        <span className="rc-base-name">{item.name}</span>
      </div>
      <small>{item.verified ? 'проверено' : 'не проверено'} · {item.note}</small>
    </div>)}
  </div>
}

// Подходящие коды ответа с вердиктами — по ним видно, поменялся ли ответ после уточнения.
function mainCodes(result: AgentResult): string {
  return result.candidates.filter((candidate) => candidate.verdict !== 'no').map((candidate) => `${candidate.code}:${candidate.verdict}`).join(',')
}

// Ответ агента: что за товар, подходящие коды с цитатой и ставками, затем почему не соседние.
// Ответ на уточнение с теми же кодами — только текст и строка с кодом, без повторных карточек.
function Answer({ result, previous }: { result: AgentResult; previous?: AgentResult }) {
  const main = result.candidates.filter((candidate) => candidate.verdict !== 'no')
  const rejected = result.candidates.filter((candidate) => candidate.verdict === 'no')
  if (previous && main.length > 0 && mainCodes(previous) === mainCodes(result)) {
    return <div className="rc-answer rc-answer-short">
      {result.description && <p className="rc-answer-text">{result.description}</p>}
      {result.notes && <p className="rc-note">{result.notes}</p>}
      <p className="rc-same-code">
        <span>{main.length > 1 ? 'Коды без изменений' : 'Код без изменений'}</span>
        {main.map((candidate) => <CopyCode key={candidate.code} code={candidate.code} className="rc-code small" />)}
      </p>
    </div>
  }
  return <div className="rc-answer">
    {result.description && <p className="rc-answer-text">{result.description}</p>}
    {main.map((candidate) => <Candidate key={candidate.code} candidate={candidate} />)}
    {rejected.length > 0 && <div className="rc-rejected">
      <p>Почему не эти коды</p>
      {rejected.map((candidate) => <div key={candidate.code} className="rc-rejected-row">
        <CopyCode code={candidate.code} className="rc-code small" />
        <span>{candidate.reason}</span>
      </div>)}
    </div>}
    {result.notes && <p className="rc-note">{result.notes}</p>}
  </div>
}

function Candidate({ candidate }: { candidate: AgentCandidate }) {
  const parts = candidate.nomenclature ? nameParts(candidate.nomenclature.name) : []
  const nomenclature = candidate.nomenclature
  return <article className={`rc-cand ${candidate.verdict}`}>
    <div className="rc-cand-top">
      <CopyCode code={candidate.code} className="rc-code big" />
      <i className={`rc-verdict ${candidate.verdict}`}>{verdictLabels[candidate.verdict]}</i>
    </div>
    {parts.length > 0 && <p className="rc-cand-name" title={parts.join(' › ')}>{parts.slice(-2).join(' › ')}</p>}
    {nomenclature && <p className="rc-rates">
      <span>Пошлина {nomenclature.tariffText || 'не указана'}</span>
      <span>НДС {nomenclature.vatPercent || '16'}%</span>
      {nomenclature.antiDumping?.map((measure) => <span key={measure.number} className={measure.exact && !measure.suspended ? 'ad' : 'ad maybe'} title={`${measure.number} «${measure.shortName}»`}>
        {measure.suspended ? 'антидемпинг не применяется' : `${measure.exact ? 'антидемпинг' : 'возможен антидемпинг'} ${measure.countries.map((country) => `${(measure.rates[country] ?? '').replace('.', ',')}%`).join(' / ')} (${measure.countryNames})`}
      </span>)}
    </p>}
    {candidate.quote && <blockquote>{candidate.quote}<cite>{candidate.quoteSource === 'page' ? 'пояснения к коду (TWS.BY)' : 'наименование кода в номенклатуре'}</cite></blockquote>}
    {candidate.reason && <p className="rc-reason">{candidate.reason}</p>}
    {candidate.check && <p className="rc-check"><b>Уточнить:</b> {candidate.check}</p>}
    {nomenclature?.detailsUrl && <a className="rc-more" href={nomenclature.detailsUrl} target="_blank" rel="noreferrer">Страница кода ↗</a>}
  </article>
}
