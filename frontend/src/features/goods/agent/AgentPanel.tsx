import { ChangeEvent, FormEvent, KeyboardEvent as ReactKeyboardEvent, useCallback, useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { PhotoLightbox } from '@/shared/components/PhotoLightbox/PhotoLightbox'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { LinkText } from '@/shared/components/LinkText/LinkText'
import { errorMessage } from '@/shared/lib/errorMessage'
import { resizePhoto } from '@/shared/lib/resizePhoto'
import { decimalText } from '@/shared/lib/rounding'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { Link } from '@/shared/router/Link'
import { paths } from '@/shared/router/router'
import { GoodsItemInput } from '@/shared/types/goods'
import { photosOf } from '@/shared/lib/itemPhotos'
import { decisionsApi } from '@/features/decisions/decisionsApi'
import { saveRowDecision } from '../hooks/useRowDecisions'
import { lookupNomenclature } from '../lib/nomenclatureRates'
import { AgentCandidate, AgentItem, AgentMessage, AgentResult, AgentSession, AgentStatus, SameProductMatch, SimilarMatch, agentApi, quoteSourceLabels } from './agentApi'
import { AgentEvidence } from './AgentEvidence'
import { DeclarationsLink } from './DeclarationsLink'
import { DocumentsAnswer, DocumentsButton, documentsRequested, scrollToDocuments } from './DocumentsAnswer'
import './agent.css'
import { CodeDecision, decisionStatusLabels } from '@/shared/tnved/decisions'
import { nameParts, formatCode } from '@/shared/tnved/codes'
import { TrademarkHits } from '@/shared/trademarks/TrademarkHits'

type Props = {
  estimateId: number
  item: GoodsItemInput
  rowNumber: number
  rowCount: number
  // Соседняя строка без закрытия панели (нет — строка крайняя).
  onPrev?: () => void
  onNext?: () => void
  onClose: () => void
  // Применить код к строке сметы (ставки — из номенклатуры) и наименование.
  onApplyCode: (code: string) => Promise<void> | void
  onApplyName: (name: string) => void
  // Фото к строке сметы (как «Добавить фото» в таблице).
  onAddPhoto: (event: ChangeEvent<HTMLInputElement>) => void
  // Подбор начался, закончился или изменился — обновить значки в таблице.
  onChanged: () => void
}

const pollInterval = 1500
// Какую кнопку ↑ ↓ нажали: панель соседней строки ставит на неё фокус.
let navFocus: 'prev' | 'next' | null = null

const toolIcons: Record<string, string> = {
  search_nomenclature: '🔍', get_codes: '✔️', list_heading: '📂', open_details: '📄', search_decisions: '🗂️', search_directory: '📚',
  open_goods_help: '📋', open_link: '🔗', search_in_document: '🔎', web_search: '🌐', web_page: '🌐', review: '🧐', photo: '🖼️', look_at_photo: '📷', search_declarations: '📑',
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

// Отступ панели сверху: под строкой сметы с названием и кнопками (этап проверки), иначе под шапкой
// сайта. Берётся место на странице, а не в окне, — при прокрутке панель не двигается (раньше
// догоняла уезжающую шапку и дёргалась).
function usePanelTop() {
  const [top, setTop] = useState(0)
  useLayoutEffect(() => {
    const header = document.querySelector<HTMLElement>('.app-header')
    const anchor = document.querySelector<HTMLElement>('.rv-titlebar') ?? header
    if (!anchor) return
    const update = () => setTop(anchor.getBoundingClientRect().bottom + window.scrollY)
    update()
    const observer = new ResizeObserver(update)
    observer.observe(anchor)
    if (header) observer.observe(header)
    return () => observer.disconnect()
  }, [])
  return top
}

// Подбор кода ТН ВЭД для одной строки сметы — боковая панель справа, не модальная: таблица остаётся
// доступной. При открытии программа только ищет товар в базе решений (тот же товар и похожие по фото);
// агента сотрудник запускает кнопкой. Подбор идёт на сервере — панель можно закрыть.
export function AgentPanel({ estimateId, item, rowNumber, rowCount, onPrev, onNext, onClose, onApplyCode, onApplyName, onAddPhoto, onChanged }: Props) {
  const itemId = item.id ?? ''
  const panelTop = usePanelTop()
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
  // Применённый код агента записывается и в базу решений (как раньше); флажок позволяет не записывать.
  const [saveToBase, setSaveToBase] = useState(true)
  const lastId = useRef(0)
  const bodyRef = useRef<HTMLDivElement>(null)
  const menuRef = useRef<HTMLDetailsElement>(null)
  const photoInput = useRef<HTMLInputElement>(null)
  const changedRef = useRef(onChanged)
  changedRef.current = onChanged

  const composeRef = useRef<HTMLTextAreaElement>(null)
  const navRef = useRef<HTMLDivElement>(null)

  // Переход ↑ ↓ открывает панель заново (ключ — строка): фокус возвращается на ту же кнопку.
  useEffect(() => {
    if (!navFocus) return
    // На крайней строке нажатая кнопка выключена — фокус на другую.
    const buttons = navRef.current
    ;(buttons?.querySelector<HTMLButtonElement>(`[data-nav="${navFocus}"]:not(:disabled)`) ?? buttons?.querySelector<HTMLButtonElement>('[data-nav]:not(:disabled)'))?.focus()
    navFocus = null
  }, [])
  const navigate = (direction: 'prev' | 'next', go?: () => void) => { if (!go) return; navFocus = direction; go() }

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

  // Esc: открытое меню «⋯» закрывается первым, потом панель.
  useEffect(() => {
    if (lightbox) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      const menu = menuRef.current
      if (menu?.open) { menu.open = false; menu.querySelector('summary')?.focus(); return }
      onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose, lightbox])

  // Меню «⋯» закрывается щелчком мимо.
  useEffect(() => {
    const onMouseDown = (event: MouseEvent) => { const menu = menuRef.current; if (menu?.open && !event.composedPath().includes(menu)) menu.open = false }
    document.addEventListener('mousedown', onMouseDown)
    return () => document.removeEventListener('mousedown', onMouseDown)
  }, [])

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
    // Пока сообщение уходило, поле было выключено и потеряло фокус.
    requestAnimationFrame(() => composeRef.current?.focus())
  }

  // Enter — отправить, Shift+Enter — новая строка.
  function onComposeKeyDown(event: ReactKeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) { event.preventDefault(); event.currentTarget.form?.requestSubmit() }
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
      await saveRowDecision(item, estimateId)
      setSavedRowCode(item.code)
      changedRef.current()
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

  // Пункт меню «⋯»: действие и закрыть меню.
  function menuAction(action: () => void) {
    if (menuRef.current) menuRef.current.open = false
    action()
  }

  // Фото подбора — снятые при его начале; подбора ещё нет — фото строки.
  const photos = session?.photos?.length ? session.photos : photosOf(item)
  const messages = session?.messages ?? []
  const input = messages.find((message) => message.kind === 'input')
  const match = messages.find((message): message is Extract<AgentMessage, { kind: 'match' }> => message.kind === 'match')
  const similar = messages.find((message): message is Extract<AgentMessage, { kind: 'similar' }> => message.kind === 'similar')
  const blocks = chatBlocks(messages)
  const stepMessages = blocks.flatMap((block) => block.type === 'steps' ? block.messages : [])
  const stream = blocks.flatMap((block) => block.type === 'message' ? [block.message] : [])
  const agentItem = input?.kind === 'input' ? input.content.item : agentItemFrom(item)
  const baseFound = (match?.content.matches.length ?? 0) + (similar?.content.matches.length ?? 0)
  // Проверенный код найден в базе, агента не запускали: «Всё равно подобрать заново».
  const skippedByBase = Boolean(match?.content.skipped) && session?.runs === 0 && session.status === 'done'
  const configured = status?.configured !== false
  const canWrite = Boolean(session) && !working && configured && session?.status !== 'ready' && !skippedByBase
  // Агента ещё не запускали: вместо шагов — кнопка «Подобрать код».
  const notStarted = !working && (!session || session.status === 'ready' || skippedByBase)
  const decisionActions: DecisionActions = {
    applied, busy, rowName: (item.name ?? '').trim(), onOpenPhotos: (id) => void openDecisionPhotos(id),
    onApply: (code, name) => void applyDecision(code, name),
  }
  const launch = () => {
    if (!session) void start()
    else void act(() => skippedByBase ? agentApi.rerun(session.id) : agentApi.begin(session.id))
  }
  const rowCodeSaved = savedRowCode === item.code

  return <aside className="ag-panel" style={{ top: panelTop + 16 }} role="complementary" aria-labelledby="ag-title">
    <header className="ag-head">
      <div className="ag-head-text">
        <h2 id="ag-title">Подбор кода</h2>
        <p>Строка {rowNumber} из {rowCount}</p>
      </div>
      <div className="ag-head-actions" ref={navRef}>
        <button type="button" className="ag-icon" data-nav="prev" onClick={() => navigate('prev', onPrev)} disabled={!onPrev} aria-label="Предыдущая строка" title="Предыдущая строка">
          <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M4 10l4-4 4 4" /></svg>
        </button>
        <button type="button" className="ag-icon" data-nav="next" onClick={() => navigate('next', onNext)} disabled={!onNext} aria-label="Следующая строка" title="Следующая строка">
          <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M4 6l4 4 4-4" /></svg>
        </button>
        <details className="ag-menu" ref={menuRef}>
          <summary className="ag-icon" aria-label="Ещё действия" title="Ещё действия">
            <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="currentColor"><circle cx="3.5" cy="8" r="1.3" /><circle cx="8" cy="8" r="1.3" /><circle cx="12.5" cy="8" r="1.3" /></svg>
          </summary>
          <div className="ag-menu-list" role="menu">
            <button type="button" role="menuitem" onClick={() => menuAction(() => void start())} disabled={!session || working || busy || !configured}>Начать заново</button>
            {running && <button type="button" role="menuitem" onClick={() => menuAction(() => session && void act(() => agentApi.cancel(session.id)))} disabled={busy}>Остановить подбор</button>}
            {/^\d{10}$/.test(item.code) && <button type="button" role="menuitem" onClick={() => menuAction(() => void saveRowCode())} disabled={busy || rowCodeSaved}
              title="Записать код, который сейчас стоит в строке, в базу решений (ждёт проверки администратором)">{rowCodeSaved ? '✓ Код строки в базе решений' : 'Записать код строки в базу решений'}</button>}
            {session && session.runs > 0 && <p className="ag-menu-note" title={`Модель ${session.model}, шагов: ${session.steps}`}>Стоимость подбора ${session.costUsd}</p>}
          </div>
        </details>
        <button type="button" className="ag-icon" onClick={onClose} aria-label="Закрыть" title="Закрыть (Esc)">
          <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"><path d="M4 4l8 8M12 4l-8 8" /></svg>
        </button>
      </div>
    </header>

    <div className="ag-body" ref={bodyRef}>
      {status && !status.configured && <p className="ag-error">Подбор кода не настроен: на сервере нет ключа (TNVED_AI_API_KEY или DEEPSEEK_API_KEY).</p>}
      <ItemCard item={agentItem} photos={photos} onOpenPhoto={(index) => setLightbox({ photos, index })} onAddPhoto={() => photoInput.current?.click()} />
      <input ref={photoInput} type="file" accept="image/jpeg,image/png" style={{ display: 'none' }} onChange={onAddPhoto} />
      {agentItem.code && <FileCode code={agentItem.code} />}

      {session && baseFound > 0 && <BaseCard match={match?.content.matches ?? []} similar={similar?.content} searching={searching} hasPhotos={photos.length > 0} actions={decisionActions} />}

      <section className="ag-section">
        <header className="ag-section-head">
          <h3>Что проверил агент</h3>
          {session && !notStarted && <small>{statusText(session)}</small>}
        </header>
        {loading && !session
          ? <p className="ag-wait"><span className="ag-spinner" aria-hidden="true" />Загрузка…</p>
          : notStarted
            ? <div className="ag-launch">
              <button type="button" className="ag-primary" onClick={launch} disabled={busy || !configured}>
                <svg width="15" height="15" viewBox="0 0 16 16" aria-hidden="true" fill="currentColor"><path d="M8 1.5l1.4 3.6 3.6 1.4-3.6 1.4L8 11.5 6.6 7.9 3 6.5l3.6-1.4zM12.5 10.5l.6 1.4 1.4.6-1.4.6-.6 1.4-.6-1.4-1.4-.6 1.4-.6z" /></svg>
                {skippedByBase ? 'Всё равно подобрать заново' : 'Подобрать код'}
              </button>
              <span>{skippedByBase ? 'Проверенный код этого товара уже есть в базе решений.' : 'обычно 1–2 минуты'}</span>
            </div>
            : <AgentChecks messages={stepMessages} searching={searching} running={running} baseFound={baseFound} />}
        {stepMessages.length > 0 && <StepsGroup messages={stepMessages} live={running} />}
        {running && <div className="ag-typing" aria-live="polite"><span /><span /><span /> {documentsRequested(messages) ? 'агент разбирает справку и перечни' : 'агент работает'} — панель можно закрыть</div>}
      </section>

      {stream.map((message) => <MessageView key={message.id} message={message} applied={applied} busy={busy} fileCode={agentItem.code}
        onApplyCode={(code, record) => void applyCode(code, record && saveToBase)} onApplyName={(name) => void applyName(name)}
        saveToBase={saveToBase} onSaveToBase={setSaveToBase}
        documentsDisabled={busy || working || !configured} onDocuments={(code) => void requestDocuments(code)} />)}
      {error && <p className="ag-error" role="alert">{error}</p>}
    </div>

    <footer className="ag-foot">
      <form className="ag-compose" onSubmit={(event) => void send(event)}>
        <textarea ref={composeRef} value={text} onChange={(event) => setText(event.target.value)} onKeyDown={onComposeKeyDown} maxLength={1000} rows={2} disabled={busy || !canWrite}
          placeholder={session?.status === 'waiting' ? 'Ответьте на вопрос агента…' : running ? 'Агент работает…' : canWrite ? 'Уточните для агента: материал, назначение, размер…' : 'Сначала запустите подбор кода'} aria-label="Сообщение агенту" />
        <button type="submit" className="ag-send" disabled={busy || !canWrite || !text.trim()} aria-label="Отправить" title="Отправить (Enter)">
          <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M8 13V3M3.5 7.5L8 3l4.5 4.5" /></svg>
        </button>
      </form>
      <div className="ag-foot-row">
        {status && status.dailyLimit > 0 && <span>Подборов сегодня: {status.usedToday} из {status.dailyLimit}</span>}
        <span className="ag-disclaimer">Окончательный код выбирает декларант</span>
      </div>
    </footer>
    {lightbox && <PhotoLightbox photos={lightbox.photos} startIndex={lightbox.index} onClose={() => setLightbox(null)} />}
  </aside>
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

// «Что проверил агент»: по источникам — база решений, номенклатура, страницы кодов и документы (и интернет / фото,
// если агент к ним обращался). Пока агент работает, крутится источник его последнего шага.
const checkGroups: { label: string; tools: string[]; always: boolean; summary: (count: number) => string }[] = [
  { label: 'Номенклатура ТН ВЭД', tools: ['search_nomenclature', 'get_codes', 'list_heading'], always: true, summary: (count) => `${count} ${plural(count, 'запрос', 'запроса', 'запросов')}` },
  { label: 'Страницы кодов и документы', tools: ['open_details', 'open_goods_help', 'search_in_document', 'open_link'], always: true, summary: (count) => `открыто: ${count}` },
  { label: 'Фото товара', tools: ['photo', 'look_at_photo'], always: false, summary: (count) => `${count} ${plural(count, 'просмотр', 'просмотра', 'просмотров')}` },
  { label: 'Интернет и декларации', tools: ['web_search', 'web_page', 'search_declarations'], always: false, summary: (count) => `${count} ${plural(count, 'запрос', 'запроса', 'запросов')}` },
]

type CheckState = 'done' | 'run' | 'fail' | 'idle'

function AgentChecks({ messages, searching, running, baseFound }: { messages: AgentMessage[]; searching: boolean; running: boolean; baseFound: number }) {
  const tools = messages.flatMap((message) => message.kind === 'tool' ? [message.content] : [])
  const lastTool = tools[tools.length - 1]?.tool
  const rows: { label: string; state: CheckState; summary: string }[] = [
    { label: 'База решений', state: searching ? 'run' : 'done', summary: searching ? 'ищу…' : baseFound ? `найдено: ${baseFound}` : 'не найдено' },
  ]
  for (const group of checkGroups) {
    const used = tools.filter((tool) => group.tools.includes(tool.tool))
    if (!used.length && !group.always) continue
    const state: CheckState = running && lastTool && group.tools.includes(lastTool) ? 'run'
      : !used.length ? 'idle' : used.every((tool) => tool.failed) ? 'fail' : 'done'
    rows.push({ label: group.label, state, summary: used.length ? group.summary(used.length) : running ? 'ожидает' : 'не понадобилось' })
  }
  return <ul className="ag-checks">
    {rows.map((row) => <li key={row.label} className={`ag-check-row ${row.state}`}>
      <CheckIcon state={row.state} />
      <span>{row.label}</span>
      <small>{row.summary}</small>
    </li>)}
  </ul>
}

function CheckIcon({ state }: { state: CheckState }) {
  if (state === 'run') return <span className="ag-spinner" aria-label="идёт" />
  if (state === 'fail') return <svg className="ag-check-icon" width="16" height="16" viewBox="0 0 16 16" aria-label="ошибка" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"><circle cx="8" cy="8" r="6.5" /><path d="M5.8 5.8l4.4 4.4M10.2 5.8l-4.4 4.4" /></svg>
  if (state === 'idle') return <svg className="ag-check-icon" width="16" height="16" viewBox="0 0 16 16" aria-label="не было" fill="none" stroke="currentColor" strokeWidth="1.4" strokeDasharray="2 2"><circle cx="8" cy="8" r="6.5" /></svg>
  return <svg className="ag-check-icon" width="16" height="16" viewBox="0 0 16 16" aria-label="готово" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><circle cx="8" cy="8" r="6.5" /><path d="M5.3 8.2l1.9 1.9 3.6-3.8" /></svg>
}

function plural(count: number, one: string, few: string, many: string) {
  const tens = count % 100, units = count % 10
  if (tens >= 11 && tens <= 14) return many
  return units === 1 ? one : units >= 2 && units <= 4 ? few : many
}

// Ход подбора: свёрнут после итога, пока агент работает — открыт.
function StepsGroup({ messages, live }: { messages: AgentMessage[]; live: boolean }) {
  const [open, setOpen] = useState(false)
  const steps = messages.filter((message) => message.kind === 'tool').length
  const expanded = open || live
  return <section className={expanded ? 'ag-steps open' : 'ag-steps'}>
    <button type="button" className="ag-steps-toggle" onClick={() => setOpen((value) => !value)} aria-expanded={expanded} disabled={live}>
      <span>Ход подбора</span><small>{steps} {plural(steps, 'шаг', 'шага', 'шагов')}</small>
      {!live && <i aria-hidden="true">{expanded ? '▴' : '▾'}</i>}
    </button>
    {expanded && <ol className="ag-steps-list">
      {messages.map((message) => {
        if (message.kind === 'text') return <li key={message.id} className="ag-thought">{message.content.text}</li>
        if (message.kind !== 'tool') return null
        const { tool, label, detail, url, failed } = message.content
        // Описание фото и ответы по фото — длинный текст: под подписью шага, с переносами.
        const block = tool === 'photo' || tool === 'look_at_photo'
        return <li key={message.id} className={'ag-step' + (failed ? ' failed' : '') + (block ? ' block' : '')}>
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
  // Код строки при начале подбора (из файла) — сравнить с предложением агента.
  fileCode: string
  onApplyCode: (code: string, record: boolean) => void
  onApplyName: (name: string) => void
  saveToBase: boolean
  onSaveToBase: (value: boolean) => void
  documentsDisabled: boolean
  onDocuments: (code: string) => void
}

function MessageView({ message, ...props }: MessageProps) {
  switch (message.kind) {
    case 'question': return <div className="ag-msg ag-question"><b>Вопрос агента</b>{message.content.text}</div>
    case 'user': return <div className="ag-msg ag-user">{message.content.text}</div>
    case 'documents': return <div className="ag-msg ag-user">Документы для ввоза · {formatCode(message.content.code)}</div>
    case 'notice': return <p className="ag-notice">{message.content.text}</p>
    case 'error': return <p className="ag-error">{message.content.message}</p>
    case 'rerun': return <p className="ag-divider"><span>подбор заново</span></p>
    case 'result': return message.content.documentsCode ? <DocumentsAnswer result={message.content} /> : <ResultCard result={message.content} {...props} />
    default: return null
  }
}

// Товар строки: название крупно, остальные названия (китайское, поставщика) — серым ниже, чипы количества и веса.
function ItemCard({ item, photos, onOpenPhoto, onAddPhoto }: { item: AgentItem; photos: string[]; onOpenPhoto: (index: number) => void; onAddPhoto: () => void }) {
  const unitNet = perUnit(item.netKg || item.grossKg, item.quantity)
  const title = russianPart(item.name) || item.supplierName || 'Без наименования'
  const rest = item.name.slice(russianPart(item.name).length).replace(/^\s*\/\s*/, '')
  const others = [...new Set([rest, item.supplierName, ...(item.otherNames ?? [])].map((name) => name?.trim()).filter((name): name is string => Boolean(name) && name !== title))]
  return <section className="ag-item">
    {photos.length > 0
      ? <div className="ag-item-photos">{photos.slice(0, 3).map((photo, index) => <button key={index} type="button" className="ag-photo" onClick={() => onOpenPhoto(index)} title="Открыть фото крупно" aria-label={`Открыть фото товара ${index + 1} крупно`}><img src={photo} alt="" /></button>)}</div>
      : <button type="button" className="ag-add-photo" onClick={onAddPhoto} title="Фото добавится к строке сметы; агент увидит его при подборе заново">
        <svg width="18" height="18" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"><path d="M8 4.5v7M4.5 8h7" /></svg>
        Добавить фото
      </button>}
    <div className="ag-item-text">
      <strong>{title}</strong>
      {others.map((other) => <span key={other}>{other}</span>)}
      <div className="ag-chips">
        {item.quantity && <span className="ag-chip">{formatRuNumber(item.quantity)} {item.unit}</span>}
        {unitNet && <span className="ag-chip">{unitNet} кг / {item.unit}</span>}
        {item.pricePerUnit && <span className="ag-chip">{formatRuNumber(item.pricePerUnit)} USD / {item.unit}</span>}
      </div>
    </div>
  </section>
}

// «Код из файла 8305 20 000 0 · пошлина 13%»: ставка — из номенклатуры.
function FileCode({ code }: { code: string }) {
  const [duty, setDuty] = useState<string | null>(null)
  useEffect(() => {
    let active = true
    void lookupNomenclature([code]).then((found) => {
      if (!active) return
      const official = found?.byCode.get(code)
      setDuty(official ? `пошлина ${official.tariffText || 'не указана'}` : found ? 'нет в номенклатуре' : '')
    }).catch(() => undefined)
    return () => { active = false }
  }, [code])
  return <p className="ag-filecode">Код из файла <b>{formatCode(code)}</b>{duty && <> · {duty}</>}</p>
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
export function russianPart(name: string) {
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

// Ставки кода чипами и антидемпинг ЕАЭС (реестр ЕЭК).
function CandidateRates({ candidate }: { candidate: AgentCandidate }) {
  return <>
    {candidate.nomenclature && <div className="ag-chips">
      <span className="ag-chip">Пошлина {candidate.nomenclature.tariffText || 'не указана'}</span>
      <span className="ag-chip">НДС {candidate.nomenclature.vatPercent || '16'}%</span>
    </div>}
    {candidate.nomenclature?.antiDumping?.map((measure) => <p key={measure.number} className={measure.exact && !measure.suspended ? 'ag-antidumping' : 'ag-antidumping maybe'}>
      {measure.suspended ? 'Антидемпинг временно не применяется' : measure.exact ? 'Антидемпинг' : 'Возможен антидемпинг'}: {measure.number} «{measure.shortName}» — {measure.countries.map((country) => `${measure.countryNames && measure.countries.length === 1 ? measure.countryNames : country} ${(measure.rates[country] ?? '').replace('.', ',')}%`).join(', ')}
      {!measure.exact && ' (мера на часть товаров позиции — сверьте описание)'}
    </p>)}
  </>
}

// «Почему»: цитата из источника, довод агента и что уточнить.
function CandidateWhy({ candidate }: { candidate: AgentCandidate }) {
  return <>
    {candidate.quote && <blockquote>{candidate.quote}{quoteSourceLabels[candidate.quoteSource] && <cite>{quoteSourceLabels[candidate.quoteSource]}</cite>}</blockquote>}
    {candidate.decisionQuote && <blockquote>{candidate.decisionQuote}<cite>{candidate.decisionSource} (TWS.BY)</cite></blockquote>}
    {candidate.reason && <p className="ag-reason"><LinkText text={candidate.reason} /></p>}
    {candidate.check && <p className="ag-check"><b>Уточнить:</b> <LinkText text={candidate.check} /></p>}
  </>
}

type CandidateProps = { candidate: AgentCandidate; applied: Applied; busy: boolean; onApplyCode: (code: string, record: boolean) => void; documentsDisabled: boolean; onDocuments: (code: string) => void }

// Другие варианты агента — компактно, под главным предложением.
function CandidateCard({ candidate, applied, busy, onApplyCode, documentsDisabled, onDocuments }: CandidateProps) {
  return <article className={`ag-cand ${candidate.verdict}`}>
    <div className="ag-row-top">
      <CodeLine code={candidate.code} name={candidate.nomenclature?.name} />
      <i className={`ag-pill ${candidate.verdict}`}>{verdictLabels[candidate.verdict]}</i>
    </div>
    <CandidateRates candidate={candidate} />
    <CandidateWhy candidate={candidate} />
    <div className="ag-actions">
      {candidate.verdict !== 'no' && (applied.code === candidate.code ? <span className="ag-done">✓ применён</span> : <button type="button" className="ag-apply ghost" onClick={() => onApplyCode(candidate.code, true)} disabled={busy}>Применить код</button>)}
      {candidate.verdict !== 'no' && <DocumentsButton code={candidate.code} disabled={documentsDisabled} onClick={onDocuments} />}
      {candidate.nomenclature?.detailsUrl && <a href={candidate.nomenclature.detailsUrl} target="_blank" rel="noreferrer">Подробнее ↗</a>}
      <DeclarationsLink candidate={candidate} />
    </div>
  </article>
}

// Главное предложение агента: код крупно, позиция, ставки, сравнение с кодом из файла, «Почему», применение.
function ProposalCard({ candidate, result, fileCode, applied, busy, onApplyCode, saveToBase, onSaveToBase, documentsDisabled, onDocuments }: CandidateProps & { result: AgentResult; fileCode: string; saveToBase: boolean; onSaveToBase: (value: boolean) => void }) {
  const checkboxId = useId()
  const parts = candidate.nomenclature ? nameParts(candidate.nomenclature.name) : []
  const same = Boolean(fileCode) && fileCode === candidate.code
  const differs = result.fileCode?.dutyDiffers ? ` · пошлина ${result.fileCode.duty} → ${result.fileCode.bestDuty}` : ''
  return <article className="ag-proposal">
    <div className="ag-proposal-top">
      <CopyCode code={candidate.code} className="ag-proposal-code" />
      <i className={`ag-pill ${candidate.verdict}`}>{verdictLabels[candidate.verdict]}</i>
    </div>
    {parts.length > 0 && <p className="ag-proposal-name" title={parts.join(' › ')}>{parts.slice(-2).join(' › ')}</p>}
    <CandidateRates candidate={candidate} />
    {fileCode && <span className={same ? 'ag-match same' : 'ag-match differs'}>{same ? 'Совпадает с кодом из файла' : `Отличается от кода из файла${differs}`}</span>}
    {(candidate.quote || candidate.reason || candidate.check) && <div className="ag-why"><b>Почему</b><CandidateWhy candidate={candidate} /></div>}
    <div className="ag-proposal-actions">
      {applied.code === candidate.code
        ? <span className="ag-done">✓ Код применён к строке</span>
        : <button type="button" className="ag-primary" onClick={() => onApplyCode(candidate.code, true)} disabled={busy}>{same ? 'Подтвердить код' : 'Применить код'}</button>}
      <label className="ag-save" htmlFor={checkboxId}>
        <input id={checkboxId} type="checkbox" checked={saveToBase} onChange={(event) => onSaveToBase(event.target.checked)} />
        Сохранить в базу решений
      </label>
    </div>
    <div className="ag-actions">
      <DocumentsButton code={candidate.code} disabled={documentsDisabled} onClick={onDocuments} />
      {candidate.nomenclature?.detailsUrl && <a href={candidate.nomenclature.detailsUrl} target="_blank" rel="noreferrer">Подробнее ↗</a>}
      <DeclarationsLink candidate={candidate} />
    </div>
  </article>
}

// Итог: главное предложение, другие подходящие коды, неподходящие — свёрнуты; наименование и замечания.
type ResultProps = Omit<MessageProps, 'message'> & { result: AgentResult }

function ResultCard({ result, fileCode, applied, busy, onApplyCode, onApplyName, saveToBase, onSaveToBase, documentsDisabled, onDocuments }: ResultProps) {
  const main = result.candidates.filter((candidate) => candidate.verdict !== 'no')
  const rejected = result.candidates.filter((candidate) => candidate.verdict === 'no')
  const [best, ...others] = main
  const shared = { applied, busy, onApplyCode, documentsDisabled, onDocuments }
  return <section className="ag-result">
    <header className="ag-section-head"><h3>Предложение агента</h3></header>
    {result.description && <p className="ag-desc"><LinkText text={result.description} /></p>}
    {best ? <ProposalCard candidate={best} result={result} fileCode={fileCode} saveToBase={saveToBase} onSaveToBase={onSaveToBase} {...shared} />
      : <p className="ag-notice">Подходящего кода агент не нашёл — уточните товар в сообщении ниже.</p>}
    <AgentEvidence result={result} />
    {others.length > 0 && <div className="ag-others">
      <h4>Другие варианты</h4>
      {others.map((candidate) => <CandidateCard key={candidate.code} candidate={candidate} {...shared} />)}
    </div>}
    {rejected.length > 0 && <details className="ag-more">
      <summary>Не подходят: {rejected.map((candidate) => formatCode(candidate.code)).join(', ')}</summary>
      {rejected.map((candidate) => <CandidateCard key={candidate.code} candidate={candidate} {...shared} />)}
    </details>}
    {result.name && <NameEditor suggested={result.name} applied={applied} busy={busy} onApply={onApplyName} />}
    {result.notes && <p className="ag-note"><LinkText text={result.notes} /></p>}
    {result.trademarks && <TrademarkHits hits={result.trademarks} />}
    {(result.droppedCodes.length > 0 || result.quotesRemoved > 0) && <p className="ag-notice">
      {result.droppedCodes.length > 0 && <>Программа убрала коды, которых нет в номенклатуре: {result.droppedCodes.map(formatCode).join(', ')}. </>}
      {result.quotesRemoved > 0 && <>Цитат, не найденных в источниках, убрано: {result.quotesRemoved}.</>}
    </p>}
  </section>
}
