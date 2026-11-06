import { ChangeEvent, FormEvent, KeyboardEvent as ReactKeyboardEvent, useCallback, useEffect, useRef, useState } from 'react'
import { PhotoLightbox } from '@/shared/components/PhotoLightbox/PhotoLightbox'
import { errorMessage } from '@/shared/lib/errorMessage'
import { GoodsItemInput } from '@/shared/types/goods'
import { photosOf } from '@/shared/lib/itemPhotos'
import { decisionsApi } from '@/features/decisions/decisionsApi'
import { saveRowDecision } from '../hooks/useRowDecisions'
import { AgentMessage, AgentSession, AgentStatus, agentApi } from './agentApi'
import { documentsRequested, scrollToDocuments } from './DocumentsAnswer'
import { agentItemFrom, agentPhotosFrom, isEmptyItem } from './agentItem'
import { clampWidth, maxWidth, minWidth, usePanelTop, usePanelWidth } from './usePanelLayout'
import { AgentChecks, chatBlocks, StepsGroup } from './AgentSteps'
import { FileCode, ItemCard, russianPart } from './ItemCard'
import { BaseCard, DecisionActions } from './BaseCard'
import { Applied, MessageView } from './ResultCard'
import '../goods.css'
import './agent.css'

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

// Подбор кода ТН ВЭД для одной строки сметы — боковая панель справа, не модальная: таблица остаётся
// доступной. При открытии программа только ищет товар в базе решений (тот же товар и похожие по фото);
// агента сотрудник запускает кнопкой. Подбор идёт на сервере — панель можно закрыть.
export function AgentPanel({ estimateId, item, rowNumber, rowCount, onPrev, onNext, onClose, onApplyCode, onApplyName, onAddPhoto, onChanged }: Props) {
  const itemId = item.id ?? ''
  const panelTop = usePanelTop()
  const [width, setWidth] = usePanelWidth()
  const empty = isEmptyItem(item)
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
      else if (agentStatus?.configured && !empty) await start()
    }).catch((reason) => { if (active) setError(errorMessage(reason)) }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
    // При открытии панели для строки и когда у пустой строки появилось наименование или фото.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [estimateId, itemId, empty])

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

  // Новые сообщения — прокрутка вниз. Без сообщений — нет: иначе при открытии уезжает вверх карточка товара.
  const messageCount = session?.messages.length ?? 0
  useEffect(() => {
    if (messageCount === 0) return
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
  const noInput = empty && !session
  // Поле сообщения — только когда агенту можно писать (или он работает); до подбора его нет.
  const showCompose = canWrite || running
  const resize = (next: number) => setWidth(clampWidth(next))

  return <aside className="ag-panel" style={{ top: panelTop + 16, maxHeight: `calc(100dvh - ${panelTop + 32}px)` }} role="complementary" aria-labelledby="ag-title">
    <div className="ag-resize" role="separator" aria-orientation="vertical" aria-label="Ширина панели" aria-valuemin={minWidth} aria-valuemax={maxWidth} aria-valuenow={width} tabIndex={0}
      title="Потяните, чтобы изменить ширину (← →)"
      onPointerDown={(event) => { event.preventDefault(); event.currentTarget.setPointerCapture(event.pointerId) }}
      onPointerMove={(event) => {
        const panel = event.currentTarget.parentElement
        if (panel && event.currentTarget.hasPointerCapture(event.pointerId)) resize(panel.getBoundingClientRect().right - event.clientX)
      }}
      onKeyDown={(event) => {
        if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
        event.preventDefault()
        const step = event.key === 'ArrowLeft' ? 16 : -16
        setWidth((current) => clampWidth(current + step))
      }} />
    <header className="ag-head">
      <div className="ag-head-text">
        <h2 id="ag-title">Подбор кода</h2>
        <p>Строка {rowNumber} из {rowCount}</p>
      </div>
      <div className="ag-head-actions" ref={navRef}>
        <button type="button" className="icon-button ghost" data-nav="prev" onClick={() => navigate('prev', onPrev)} disabled={!onPrev} aria-label="Предыдущая строка" title="Предыдущая строка">
          <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M4 10l4-4 4 4" /></svg>
        </button>
        <button type="button" className="icon-button ghost" data-nav="next" onClick={() => navigate('next', onNext)} disabled={!onNext} aria-label="Следующая строка" title="Следующая строка">
          <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M4 6l4 4 4-4" /></svg>
        </button>
        <details className="ag-menu" ref={menuRef}>
          <summary className="icon-button ghost" aria-label="Ещё действия" title="Ещё действия">
            <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="currentColor"><circle cx="3.5" cy="8" r="1.3" /><circle cx="8" cy="8" r="1.3" /><circle cx="12.5" cy="8" r="1.3" /></svg>
          </summary>
          <div className="ag-menu-list" role="menu">
            <button type="button" role="menuitem" onClick={() => menuAction(() => void start())} disabled={!session || working || busy || !configured || empty}>Начать заново</button>
            {running && <button type="button" role="menuitem" onClick={() => menuAction(() => session && void act(() => agentApi.cancel(session.id)))} disabled={busy}>Остановить подбор</button>}
            {/^\d{10}$/.test(item.code) && <button type="button" role="menuitem" onClick={() => menuAction(() => void saveRowCode())} disabled={busy || rowCodeSaved}
              title="Записать код, который сейчас стоит в строке, в базу решений (ждёт проверки администратором)">{rowCodeSaved ? '✓ Код строки в базе решений' : 'Записать код строки в базу решений'}</button>}
            {session && session.runs > 0 && <p className="ag-menu-note" title={`Модель ${session.model}, шагов: ${session.steps}`}>Стоимость подбора ${session.costUsd}</p>}
          </div>
        </details>
        <button type="button" className="icon-button ghost" onClick={onClose} aria-label="Закрыть" title="Закрыть (Esc)">
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
          ? <p className="ag-wait"><span className="goods-spinner" aria-hidden="true" />Загрузка…</p>
          : noInput
            ? <div className="ag-empty">
              <p>Нет наименования и фото — агенту не по чему искать. Впишите наименование товара в таблице или добавьте фото.</p>
              <div className="ag-launch">
                <button type="button" onClick={() => photoInput.current?.click()}>Добавить фото</button>
                <button type="button" className="secondary" disabled title="Нет наименования и фото — подбирать не по чему">
                  <svg width="15" height="15" viewBox="0 0 16 16" aria-hidden="true" fill="currentColor"><path d="M8 1.5l1.4 3.6 3.6 1.4-3.6 1.4L8 11.5 6.6 7.9 3 6.5l3.6-1.4zM12.5 10.5l.6 1.4 1.4.6-1.4.6-.6 1.4-.6-1.4-1.4-.6 1.4-.6z" /></svg>
                  Подобрать код
                </button>
              </div>
            </div>
          : notStarted
            ? <div className="ag-launch">
              <button type="button" className={skippedByBase ? 'secondary' : undefined} onClick={launch} disabled={busy || !configured}>
                <svg width="15" height="15" viewBox="0 0 16 16" aria-hidden="true" fill="currentColor"><path d="M8 1.5l1.4 3.6 3.6 1.4-3.6 1.4L8 11.5 6.6 7.9 3 6.5l3.6-1.4zM12.5 10.5l.6 1.4 1.4.6-1.4.6-.6 1.4-.6-1.4-1.4-.6 1.4-.6z" /></svg>
                {skippedByBase ? 'Всё равно подобрать заново' : 'Подобрать код'}
              </button>
              <span>{skippedByBase ? 'Проверенный код этого товара уже есть в базе решений.' : 'обычно 1–2 минуты'}</span>
            </div>
            : <AgentChecks messages={stepMessages} searching={searching} running={running} baseFound={baseFound} />}
        {stepMessages.length > 0 && <StepsGroup messages={stepMessages} live={running} />}
        {running && <div className="ag-typing" aria-live="polite"><span /><span /><span /> {documentsRequested(messages) ? 'агенты разбирают справку и перечни параллельно' : 'агент работает'} — панель можно закрыть</div>}
      </section>

      {stream.map((message) => <MessageView key={message.id} message={message} applied={applied} busy={busy} fileCode={agentItem.code}
        onApplyCode={(code, record) => void applyCode(code, record && saveToBase)} onApplyName={(name) => void applyName(name)}
        saveToBase={saveToBase} onSaveToBase={setSaveToBase}
        documentsDisabled={busy || working || !configured} onDocuments={(code) => void requestDocuments(code)} />)}
      {error && <p className="ag-error" role="alert">{error}</p>}
    </div>

    {!noInput && <footer className="ag-foot">
      {showCompose && <form className="ag-compose" onSubmit={(event) => void send(event)}>
        <textarea ref={composeRef} value={text} onChange={(event) => setText(event.target.value)} onKeyDown={onComposeKeyDown} maxLength={1000} rows={2} disabled={busy || !canWrite}
          placeholder={session?.status === 'waiting' ? 'Ответьте на вопрос агента…' : running ? 'Агент работает…' : 'Уточните для агента: материал, назначение, размер…'} aria-label="Сообщение агенту" />
        <button type="submit" className="ag-send" disabled={busy || !canWrite || !text.trim()} aria-label="Отправить" title="Отправить (Enter)">
          <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M8 13V3M3.5 7.5L8 3l4.5 4.5" /></svg>
        </button>
      </form>}
      <div className="ag-foot-row">
        {status && status.dailyLimit > 0 && <span>Подборов сегодня: {status.usedToday} из {status.dailyLimit}</span>}
        <span className="ag-disclaimer">Окончательный код выбирает декларант</span>
      </div>
    </footer>}
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
