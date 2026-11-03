import { ClipboardEvent, DragEvent, FormEvent, KeyboardEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { LinkText } from '@/shared/components/LinkText/LinkText'
import { CopyIcon } from '@/shared/components/CopyCode/CopyIcon'
import { PhotoLightbox } from '@/shared/components/PhotoLightbox/PhotoLightbox'
import { errorMessage } from '@/shared/lib/errorMessage'
import { clipboardImages, clipboardText } from '@/shared/lib/clipboardImages'
import { resizePhoto } from '@/shared/lib/resizePhoto'
import { sameAnswer } from './answerText'
import { Command, documentsCode, menuCommands, parseCommand, splitDocumentsArg } from './commands'
import { formatCode, nameParts } from '@/shared/tnved/codes'
import { AgentCandidate, AgentMessage, AgentResult, AgentSession, AgentStatus, BrandCheck, ReferenceSummary, agentApi, quoteSourceLabels } from '@/features/goods/agent/agentApi'
import { AgentEvidence } from '@/features/goods/agent/AgentEvidence'
import { DeclarationsLink } from '@/features/goods/agent/DeclarationsLink'
import { DocumentsAnswer, DocumentsButton, documentsRequested, scrollToDocuments } from '@/features/goods/agent/DocumentsAnswer'
import { TrademarkHits } from '@/shared/trademarks/TrademarkHits'
import { plural } from '@/shared/lib/plural'
import { RoadGame } from './RoadGame'
import './ReferenceChat.css'

const pollInterval = 1500
// Список истории, пока в нём есть вопрос, который ещё ищет.
const historyPollInterval = 5000
const maxPhotos = 3
const verdictLabels: Record<AgentCandidate['verdict'], string> = { fits: 'подходит', doubtful: 'возможно', no: 'не подходит' }
const toolIcons: Record<string, string> = { search_nomenclature: '🔍', get_codes: '✔️', list_heading: '📂', open_details: '📄', search_decisions: '🗂️', search_directory: '📚', open_goods_help: '📋', open_link: '🔗', search_in_document: '🔎', web_search: '🌐', web_page: '🌐', review: '🧐', photo: '🖼️', look_at_photo: '📷', search_declarations: '📑', check_trademarks: '®️', general: '🗃️', act: '📖' }

const isWorking = (session: AgentSession | null) => session?.status === 'running' || session?.status === 'searching'
const isImage = (file: File) => file.type === 'image/jpeg' || file.type === 'image/png'
// Вставку и «/» обрабатываем только вне полей ввода и открытых окон.
const editable = (target: EventTarget | null) => target instanceof HTMLElement && Boolean(target.closest('input, textarea, select, [contenteditable=""], [contenteditable="true"]'))
const dialogOpen = () => document.querySelector('[role="dialog"][aria-modal="true"]') !== null

// Строка под вопросом в истории: что с ним сейчас, затем код.
// Открытый вопрос и прокрутка ленты — в sessionStorage вкладки (может быть недоступен — тогда просто не помним).
const openQuestionKey = 'rc-open-question'
type OpenQuestion = { id: number; scroll: number }

function readOpenQuestion(): OpenQuestion | null {
  try {
    const value = JSON.parse(sessionStorage.getItem(openQuestionKey) ?? 'null') as Partial<OpenQuestion> | null
    return value && Number.isInteger(value.id) && value.id! > 0 ? { id: value.id!, scroll: Number(value.scroll) || 0 } : null
  } catch { return null }
}

function saveOpenQuestion(value: OpenQuestion | null) {
  try {
    if (value) sessionStorage.setItem(openQuestionKey, JSON.stringify(value))
    else sessionStorage.removeItem(openQuestionKey)
  } catch { /* хранилище недоступно */ }
}

function historyState(item: ReferenceSummary) {
  if (item.status === 'running' || item.status === 'searching') return <b className="rc-state busy">{item.kind === 'trademarks' ? 'проверяет…' : 'ищет…'}</b>
  if (item.status === 'waiting') return <b className="rc-state busy">ждёт ответа</b>
  if (item.status === 'error' || item.status === 'cancelled') return <b className="rc-state bad">не завершён</b>
  if (item.kind === 'trademarks') return item.trademarkHits ? `ТРОИС: найдено ${item.trademarkHits}` : 'ТРОИС: нет в реестре'
  return item.bestCode ? <code>{formatCode(item.bestCode)}</code> : 'код не найден'
}

// Вкладки истории: «Общий» — все вопросы; «Чат» — подбор кода (и документы после него); «ТРОИС» —
// проверки бренда; «Документы» — только «Документы для ввоза».
type HistoryTab = 'all' | ReferenceSummary['kind']
const historyTabs: { tab: HistoryTab; label: string }[] = [
  { tab: 'all', label: 'Общий' }, { tab: 'chat', label: 'Чат' }, { tab: 'trademarks', label: 'ТРОИС' }, { tab: 'documents', label: 'Документы' },
]

const kindLabels: Record<ReferenceSummary['kind'], string> = { chat: 'Чат', trademarks: 'Проверка ТРОИС', documents: 'Документы для ввоза' }

// Значки вида вопроса: общий — слои, чат — облачко, ТРОИС — щит (защита знака), документы — лист.
const kindPaths: Record<HistoryTab, string[]> = {
  all: ['M8 2 2 5l6 3 6-3-6-3Z', 'm2 8 6 3 6-3', 'm2 11 6 3 6-3'],
  chat: ['M3 3h10a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H7.2L4.5 13.4V11H3a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1Z'],
  trademarks: ['M8 1.8 3 3.7v3.9c0 3 2.1 5.3 5 6.6 2.9-1.3 5-3.6 5-6.6V3.7L8 1.8Z', 'm5.9 8 1.5 1.5 2.8-3'],
  documents: ['M4 1.8h5.4L12.4 5v9.2H4Z', 'M9.4 1.8V5h3', 'M6 8.2h4.4M6 10.8h4.4'],
}

function KindIcon({ kind, label }: { kind: HistoryTab; label?: string }) {
  return <svg className={`rc-kind ${kind}`} width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round"
    role={label ? 'img' : undefined} aria-label={label} aria-hidden={label ? undefined : true}>
    {kindPaths[kind].map((path) => <path key={path} d={path} />)}
  </svg>
}

// Что озвучить экранному диктору при смене состояния вопроса.
function liveText(session: AgentSession | null) {
  if (!session) return ''
  if (session.status === 'searching') return 'Ищу в базе решений'
  if (session.status === 'running') return session.trademarkCheck ? 'Проверяю бренд по ТРОИС' : 'Агент ищет код'
  if (session.status === 'waiting') return 'Агент задал уточняющий вопрос'
  if (session.status === 'error') return 'Подбор прервался'
  if (session.status === 'done') return 'Ответ готов'
  return ''
}

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
  // Игра, пока агент работает: только по кнопке «Сыграть», закрывается крестиком.
  const [gameOpen, setGameOpen] = useState(false)
  const [historyOpen, setHistoryOpen] = useState(false)
  const [historyTab, setHistoryTab] = useState<HistoryTab>('all')
  const [lightbox, setLightbox] = useState<{ photos: string[]; index: number } | null>(null)
  // История — одна остановка Tab: по вопросам — стрелками (roving tabindex).
  const [historyFocus, setHistoryFocus] = useState(0)
  const [menuIndex, setMenuIndex] = useState(0)
  const menu = menuCommands(text)
  const menuActive = menu[Math.min(menuIndex, menu.length - 1)]
  const historyRef = useRef<HTMLUListElement>(null)
  const lastId = useRef(0)
  const threadRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  const loadHistory = useCallback(() => agentApi.history().then((list) => setHistory(list.items)).catch(() => undefined), [])
  useEffect(() => {
    agentApi.status().then(setStatus).catch(() => undefined)
    void loadHistory()
  }, [loadHistory])

  // Вопрос в истории ещё ищет (в том числе не открытый сейчас) — обновляем список, пока не закончит.
  const historyWorking = history.some((item) => item.status === 'running' || item.status === 'searching')
  useEffect(() => {
    if (!historyWorking) return
    const timer = window.setInterval(() => void loadHistory(), historyPollInterval)
    return () => window.clearInterval(timer)
  }, [historyWorking, loadHistory])

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

  // Обновили страницу — тот же вопрос и то же место в ленте (только в этой вкладке: новая открывается пустой).
  const restoreScroll = useRef<number | null>(null)
  useEffect(() => {
    const saved = readOpenQuestion()
    if (!saved) return
    restoreScroll.current = saved.scroll
    agentApi.get(saved.id).then((next) => accept(next, true)).catch(() => { restoreScroll.current = null; saveOpenQuestion(null) })
  }, [accept])
  useEffect(() => {
    const save = () => saveOpenQuestion(sessionId ? { id: sessionId, scroll: threadRef.current?.scrollTop ?? 0 } : null)
    if (sessionId || restoreScroll.current === null) save() // пока вопрос восстанавливается — запись не стираем
    window.addEventListener('pagehide', save)
    return () => window.removeEventListener('pagehide', save)
  }, [sessionId])

  const messageCount = session?.messages.length ?? 0
  useEffect(() => {
    const thread = threadRef.current
    if (!thread) return
    if (restoreScroll.current !== null && messageCount > 0) {
      thread.scrollTop = restoreScroll.current
      restoreScroll.current = null
      return
    }
    thread.scrollTo({ top: thread.scrollHeight, behavior: 'smooth' })
  }, [messageCount, session?.status])

  async function addFiles(files: FileList | File[]) {
    const images = [...files].filter(isImage)
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

  // Фото из буфера, вставленное вне полей ввода, становится вложением вопроса (не отправляется само), а
  // текст (в том числе ячейки Excel) — дописывается в вопрос. «/» вне полей — перейти к вопросу. В полях,
  // окнах и с модификаторами ничего не перехватываем.
  const addFilesRef = useRef(addFiles)
  addFilesRef.current = addFiles
  useEffect(() => {
    const onWindowPaste = (event: globalThis.ClipboardEvent) => {
      if (event.defaultPrevented || editable(event.target) || dialogOpen()) return
      const files = clipboardImages(event.clipboardData).filter(isImage)
      const pasted = clipboardText(event.clipboardData)
      if (files.length === 0 && !pasted) return
      event.preventDefault()
      if (files.length > 0) {
        void addFilesRef.current(files)
        return
      }
      setText((current) => (current.trim() ? current.trimEnd() + ' ' + pasted : pasted))
      inputRef.current?.focus()
    }
    const onWindowKey = (event: globalThis.KeyboardEvent) => {
      if (event.key !== '/' || event.ctrlKey || event.metaKey || event.altKey || event.defaultPrevented || editable(event.target) || dialogOpen()) return
      event.preventDefault()
      // Пустое поле — сразу меню команд.
      setText((current) => current || '/')
      inputRef.current?.focus()
    }
    window.addEventListener('paste', onWindowPaste)
    window.addEventListener('keydown', onWindowKey)
    return () => { window.removeEventListener('paste', onWindowPaste); window.removeEventListener('keydown', onWindowKey) }
  }, [])

  // Открыли главную, курсор нигде, а поле вопроса видно — сразу в него (без прокрутки страницы).
  useEffect(() => {
    const input = inputRef.current
    const visible = input ? input.getBoundingClientRect().bottom <= window.innerHeight : false
    if (visible && (document.activeElement === document.body || document.activeElement === null)) input?.focus({ preventScroll: true })
  }, [])

  // Ячейки Excel вставляются текстом, а не их картинкой-отрисовкой.
  function onPaste(event: ClipboardEvent<HTMLTextAreaElement>) {
    const files = clipboardImages(event.clipboardData)
    if (files.length) { event.preventDefault(); void addFiles(files) }
  }

  function onDrop(event: DragEvent<HTMLElement>) {
    event.preventDefault()
    setDragging(false)
    if (event.dataTransfer.files.length) void addFiles(event.dataTransfer.files)
  }

  // Команда «/…» — её действие; иначе обычный вопрос.
  async function send(event?: FormEvent) {
    event?.preventDefault()
    const value = text.trim()
    if ((!value && photos.length === 0) || busy) return
    const parsed = parseCommand(value)
    if (parsed === 'unknown') { setError('Нет такой команды — наберите «/», чтобы увидеть список'); return }
    if (!parsed) return submit(value, false)
    const { command, arg } = parsed
    if (command.name === 'новый') return startNew()
    if (command.name === 'троис') {
      if (!arg && photos.length === 0) { setError('Напишите бренд или добавьте фото: /троис SONV'); return }
      return checkBrand(arg)
    }
    if (command.name === 'документы') {
      // Нет чата или указаны и код, и товар (текст/фото) — новый чат сразу с документами, без подбора.
      const typed = splitDocumentsArg(arg)
      if (!session || (typed.code && (typed.product || photos.length > 0))) {
        const example = ': /документы Наждачный круг 6804 22 500 0'
        if (typed.code.length !== 10) { setError((typed.code ? 'Код — 10 цифр' : 'Укажите код') + example); return }
        if (!typed.product && photos.length === 0) { setError('Опишите товар или добавьте фото' + example); return }
        return submit(typed.product, true, typed.code)
      }
      const code = documentsCode(arg, session.messages)
      if (code.length !== 10) { setError(code ? 'Код — 10 цифр' : 'В ответе нет подходящего кода — укажите его: /документы 8205 59 100 0'); return }
      setText('')
      return requestDocuments(code)
    }
    const question = command.name === 'сравнить' ? arg && `Чем отличаются коды ТН ВЭД ${arg} и когда какой применять?` : arg
    if (!question && photos.length === 0) { setError(command.name === 'сравнить' ? 'Укажите коды: /сравнить 6116 10 200 0 и 6116 10 800 0' : 'Опишите товар после /подбор или добавьте фото'); return }
    return submit(question, true)
  }

  // «/троис»: новый вопрос в истории — проверка бренда по ТРОИС идёт в фоне, ход и ответ — в ленте.
  async function checkBrand(value: string) {
    setBusy(true); setError('')
    try {
      accept(await agentApi.askTrademarks({ text: value, photos }), true)
      setPhotos([]); setText('')
      void loadHistory()
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false)
    }
  }

  // Новый вопрос (fresh, фото, нет активного или открыта проверка ТРОИС) — или уточнение к текущему
  // (агент помнит разговор).
  async function submit(value: string, fresh: boolean, documentsCode?: string) {
    setBusy(true); setError('')
    try {
      if (fresh || !session || photos.length > 0 || session.trademarkCheck) {
        accept(await agentApi.ask({ text: value, photos, documentsCode }), true)
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

  // «Документы для ввоза»: ответ для этого кода уже есть — прокрутка к нему, иначе запуск агента.
  async function requestDocuments(code: string) {
    if (!session || busy || isWorking(session)) return
    if (scrollToDocuments(threadRef.current, code)) return
    setBusy(true); setError('')
    try {
      accept(await agentApi.documents(session.id, code), false)
      agentApi.status().then(setStatus).catch(() => undefined)
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false)
    }
  }

  // Остановить агента: на сервере запуск прерывается, в чате — «Подбор остановлен.».
  async function stop() {
    if (!session || busy) return
    setBusy(true); setError('')
    try {
      accept(await agentApi.cancel(session.id), false)
      void loadHistory()
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false)
    }
  }

  // Выбор в меню: команда с аргументом — в поле, дописать; без аргумента — сразу.
  function pick(command: Command) {
    if (command.name === 'новый') { startNew(); return }
    setText(`/${command.name} `)
    inputRef.current?.focus()
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (menuActive) {
      const step = event.key === 'ArrowDown' ? 1 : event.key === 'ArrowUp' ? -1 : 0
      if (step) { event.preventDefault(); setMenuIndex((menu.indexOf(menuActive) + step + menu.length) % menu.length); return }
      if ((event.key === 'Enter' && !event.shiftKey) || event.key === 'Tab') { event.preventDefault(); pick(menuActive); return }
      if (event.key === 'Escape') { event.preventDefault(); setText(''); return }
    }
    if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); void send() }
  }

  async function open(id: number) {
    setHistoryOpen(false); setError('')
    try { accept(await agentApi.get(id), true); void loadHistory() } catch (reason) { setError(errorMessage(reason)) }
  }

  async function remove(id: number) {
    if (!window.confirm('Удалить этот вопрос из истории?')) return
    try {
      await agentApi.removeQuestion(id)
      if (session?.id === id) setSession(null)
      void loadHistory()
    } catch (reason) { setError(errorMessage(reason)) }
  }

  function onHistoryKey(event: KeyboardEvent<HTMLButtonElement>, index: number, id: number) {
    const last = shown.length - 1
    const next = event.key === 'ArrowDown' ? Math.min(last, index + 1) : event.key === 'ArrowUp' ? Math.max(0, index - 1) : event.key === 'Home' ? 0 : event.key === 'End' ? last : -1
    if (next >= 0) {
      event.preventDefault()
      setHistoryFocus(next)
      historyRef.current?.querySelectorAll<HTMLButtonElement>('.rc-history-open')[next]?.focus()
    } else if (event.key === 'Delete') {
      event.preventDefault()
      void remove(id)
    }
  }

  function startNew() {
    setSession(null); setText(''); setPhotos([]); setError(''); setHistoryOpen(false)
    inputRef.current?.focus()
  }

  const configured = status?.configured !== false
  // Лимит вопросов на сегодня: от 80% — подсветка (лимит не меняется, только видно, что он близко).
  const nearLimit = status !== null && status.dailyLimit > 0 && status.usedToday >= status.dailyLimit * .8
  const shown = historyTab === 'all' ? history : history.filter((item) => item.kind === historyTab)
  const rovingIndex = Math.min(historyFocus, Math.max(0, shown.length - 1))
  const placeholder = !session ? 'Опишите товар, вставьте фото (Ctrl+V) или перетащите его сюда…'
    : session.trademarkCheck ? 'Новый вопрос о товаре или /троис для другого бренда…'
    : session.status === 'waiting' ? 'Ответьте на вопрос…' : photos.length ? 'Добавьте пояснение к фото — это будет новый вопрос' : 'Уточните: «а если покрытие латекс?» — или добавьте фото для нового вопроса'

  return <section className={dragging ? 'rc dragging' : 'rc'} aria-label="Подбор кода ТН ВЭД"
    onDragOver={(event) => { if (event.dataTransfer.types.includes('Files')) { event.preventDefault(); setDragging(true) } }}
    // Ушёл за пределы чата (с любого вложенного элемента, из окна, Esc) — рамка снимается.
    onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false) }} onDrop={onDrop}>
    <header className="rc-head">
      <div className="rc-title">
        <span className="rc-logo" aria-hidden="true">
          <svg width="18" height="18" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"><path d="M4 5.5h12M4 10h8M4 14.5h5" /><circle cx="14.5" cy="13.5" r="2.6" /><path d="M16.4 15.4 18 17" /></svg>
        </span>
        <div>
          <h2>Подбор кода ТН ВЭД</h2>
          <p>Фото или описание товара — код и почему именно он</p>
        </div>
      </div>
      <div className="rc-head-actions">
        <button type="button" className="rc-ghost rc-history-toggle" onClick={() => setHistoryOpen((value) => !value)} aria-expanded={historyOpen}>История{history.length > 0 && ` · ${history.length}`}</button>
        {/* Всегда на месте: с пустого экрана видно, где начать заново (без разговора — неактивна). */}
        <button type="button" className="rc-ghost" onClick={startNew} disabled={!session} title={session ? 'Начать новый вопрос' : 'Вы уже в новом вопросе'}>+ Новый вопрос</button>
      </div>
    </header>

    <div className="rc-body">
      <aside className={historyOpen ? 'rc-history open' : 'rc-history'} aria-label="Прошлые вопросы">
        <p className="rc-history-title">История вопросов</p>
        {history.length > 0 && <div className="rc-history-tabs" role="group" aria-label="Вид вопросов">
          {historyTabs.map(({ tab, label }) => {
            const count = tab === 'all' ? history.length : history.filter((item) => item.kind === tab).length
            return <button key={tab} type="button" className={historyTab === tab ? 'active' : undefined} aria-pressed={historyTab === tab}
              onClick={() => { setHistoryTab(tab); setHistoryFocus(0) }}><span className="rc-tab-top"><KindIcon kind={tab} /><small>{count}</small></span><b>{label}</b></button>
          })}
        </div>}
        {history.length === 0 ? <p className="rc-history-empty">Здесь появятся ваши вопросы.</p>
          : shown.length === 0 ? <p className="rc-history-empty">{historyTab === 'trademarks' ? 'Проверок по ТРОИС ещё не было: отправьте фото или бренд с командой /троис.' : 'Здесь пока пусто.'}</p>
          : <>
            <p id="rc-history-help" className="rc-sr">Стрелки вверх и вниз — по вопросам, Enter — открыть, Delete — удалить</p>
            <ul ref={historyRef}>{shown.map((item, index) => <li key={item.id} className={session?.id === item.id ? 'active' : undefined}>
              <button type="button" className="rc-history-open" tabIndex={index === rovingIndex ? 0 : -1} onFocus={() => setHistoryFocus(index)} onKeyDown={(event) => onHistoryKey(event, index, item.id)} onClick={() => void open(item.id)} title={item.title} aria-describedby="rc-history-help" aria-current={session?.id === item.id ? 'true' : undefined} aria-keyshortcuts="Delete">
                <span className="rc-history-name"><KindIcon kind={item.kind} label={kindLabels[item.kind]} />{item.hasPhoto && <svg className="rc-history-photo" width="12" height="12" viewBox="0 0 12 12" aria-label="с фото" role="img" fill="none" stroke="currentColor" strokeWidth="1.3"><rect x="1.5" y="3" width="9" height="7" rx="1.5" /><circle cx="6" cy="6.5" r="1.6" /><path d="M4.2 3 5 1.8h2L7.8 3" /></svg>}{item.title}</span>
                <small>{historyState(item)} · {new Date(item.updatedAt).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })}</small>
              </button>
              <button type="button" className="rc-history-remove" tabIndex={-1} onClick={() => void remove(item.id)} aria-label={`Удалить вопрос «${item.title}»`} title="Удалить (Delete)">×</button>
            </li>)}</ul>
          </>}
      </aside>

      <div className="rc-main">
        <p className="rc-sr" role="status" aria-live="polite">{liveText(session)}</p>
        <div className="rc-thread" ref={threadRef}>
          {!session ? <div className="rc-empty">
            <span className="rc-empty-icon" aria-hidden="true">
              <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="5" width="18" height="14" rx="3" /><circle cx="9" cy="11" r="2" /><path d="m21 16-4.5-4.5L7 19" /></svg>
            </span>
            <h3>Какой код у товара?</h3>
            <p>Перетащите фото, вставьте его через Ctrl+V или опишите товар. Сначала проверим базу решений компании, потом агент сверит примеры декларирования и пояснения к ТН ВЭД, найдёт код в номенклатуре и объяснит выбор.</p>
          </div> : <Thread session={session} onPhoto={(list, index) => setLightbox({ photos: list, index })}
            documentsDisabled={busy || working || !configured} onDocuments={(code) => void requestDocuments(code)} />}
        </div>

        {/* Игра — не в ленте, а над полем ввода: новые шаги агента прокручивают ленту выше и игру не сдвигают.
            Только пока агент ищет код; ответ готов — игра уходит. Сервер не нагружает. */}
        {session?.status === 'running' && !session.trademarkCheck && <div className="rc-game-dock">
          {gameOpen ? <RoadGame onClose={() => setGameOpen(false)} />
            : <button type="button" className="rc-game-open" onClick={() => setGameOpen(true)}>Сыграть, пока ИИ ищет код</button>}
        </div>}

        <form className="rc-composer" onSubmit={(event) => void send(event)}>
          {/* Ошибка — в поле ввода, а не в ленте: фото увеличивает поле, и конец ленты уходит из вида. */}
          {error && <p className="rc-error" role="alert">{error}</p>}
          {photos.length > 0 && <div className="rc-attachments">{photos.map((photo, index) => <span key={index} className="rc-attachment">
            <img src={photo} alt="" />
            <button type="button" onClick={() => setPhotos((current) => current.filter((_, at) => at !== index))} aria-label="Убрать фото">×</button>
          </span>)}</div>}
          {menuActive && <ul id="rc-commands" className="rc-commands" role="listbox" aria-label="Команды">{menu.map((command) =>
            <li key={command.name} id={`rc-command-${command.name}`} role="option" aria-selected={command === menuActive}
              onMouseDown={(event) => { event.preventDefault(); pick(command) }} onMouseEnter={() => setMenuIndex(menu.indexOf(command))}>
              <code>/{command.name}{command.arg && <i> {command.arg}</i>}</code>
              <b>{command.label}</b>
              <small>{command.hint}</small>
            </li>)}</ul>}
          <div className="rc-input-row">
            <label className="rc-attach" title="Добавить фото (JPG, PNG)">
              <svg width="18" height="18" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round"><path d="m15.5 9.5-5.6 5.6a3.4 3.4 0 0 1-4.8-4.8l6-6a2.3 2.3 0 0 1 3.2 3.2l-6 6a1.1 1.1 0 0 1-1.6-1.6l5.3-5.3" /></svg>
              <input type="file" accept="image/jpeg,image/png" multiple onChange={(event) => { if (event.target.files) void addFiles(event.target.files); event.target.value = '' }} disabled={busy || photos.length >= maxPhotos} />
            </label>
            <textarea ref={inputRef} value={text} onChange={(event) => { setText(event.target.value); setMenuIndex(0) }} onKeyDown={onKeyDown} onPaste={onPaste} rows={1} maxLength={2000}
              placeholder={placeholder} aria-label="Вопрос о товаре" aria-autocomplete="list" aria-expanded={Boolean(menuActive)}
              aria-controls={menuActive ? 'rc-commands' : undefined} aria-activedescendant={menuActive ? `rc-command-${menuActive.name}` : undefined} disabled={!configured || (working && photos.length === 0)} />
            {/* Пока агент ищет, кнопка отправки — «стоп» (если не прикреплено новое фото). */}
            {session?.status === 'running' && photos.length === 0
              ? <button type="button" className="rc-send" onClick={() => void stop()} disabled={busy} aria-label="Остановить" title="Остановить">
                <svg width="14" height="14" viewBox="0 0 14 14" fill="currentColor"><rect x="2" y="2" width="10" height="10" rx="2" /></svg>
              </button>
              : <button type="submit" className="rc-send" disabled={busy || !configured || (!text.trim() && photos.length === 0) || (working && photos.length === 0)} aria-label="Отправить">
                <svg width="18" height="18" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round"><path d="M10 16V4M5 9l5-5 5 5" /></svg>
              </button>}
          </div>
          <p className="rc-hint">
            {!configured ? 'ИИ не настроен на сервере.' : <span>Enter — отправить, Shift+Enter — новая строка, <kbd>/</kbd> — команды{status && status.dailyLimit > 0 && <> · <b className={nearLimit ? 'rc-limit near' : 'rc-limit'}>сегодня {status.usedToday} из {status.dailyLimit}</b></>}</span>}
            <span className="rc-disclaimer">Подсказка ИИ — код выбирает декларант</span>
          </p>
        </form>
      </div>
    </div>
    {dragging && <div className="rc-drop" aria-hidden="true"><span>Отпустите фото — оно добавится к вопросу</span></div>}
    {lightbox && <PhotoLightbox photos={lightbox.photos} startIndex={lightbox.index} onClose={() => setLightbox(null)} />}
  </section>
}

// Ответ «/троис»: какие бренды и рисунки ИИ увидел на фото и знаки реестра (или что их нет).
function BrandAnswer({ result, withPhotos }: { result: BrandCheck; withPhotos: boolean }) {
  return <>
    <div className="rc-answer">
      {withPhotos && <p className="rc-answer-text">{result.brands.length > 0 ? <>На фото: <b>{result.brands.join(', ')}</b></> : 'На фото брендов не видно.'}</p>}
      {result.pictures.length > 0 && <p className="rc-answer-text">Рисунок на знаке: <b>{result.pictures.join(', ')}</b></p>}
      {result.compared > 0 && <p className="rc-note">Фото сверено с {result.compared} {plural(result.compared, 'логотипом', 'логотипами', 'логотипами')} реестра: с похожим рисунком и той же группы товаров.</p>}
      {result.photoNote && <p className="rc-note">{result.photoNote}</p>}
      {result.textNote && <p className="rc-note">{result.textNote}</p>}
      {result.hits.length > 0 ? <>
        <TrademarkHits hits={result.hits} />
        <p className="rc-note">Решение за декларантом: совпадение — повод проверить, нужен ли товарный знак и согласие правообладателя.</p>
      </> : <p className="rc-answer-text">В реестре ТРОИС по написанию не найдено: ни этого знака, ни отличающихся на одну букву. Это не значит, что знака нет у правообладателя.</p>}
    </div>
  </>
}

// Разговор: вопрос сотрудника, что нашлось в базе, ход поиска, вопросы агента, ответ.
type ThreadProps = { session: AgentSession; onPhoto: (photos: string[], index: number) => void; documentsDisabled: boolean; onDocuments: (code: string) => void }

function Thread({ session, onPhoto, documentsDisabled, onDocuments }: ThreadProps) {
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
          {session.trademarkCheck ? <p>Проверка по ТРОИС{message.content.item.name && ` · ${message.content.item.name}`}</p>
            : message.content.item.name && <p>{message.content.item.name}</p>}
        </div>
        case 'trademarks': return <BrandAnswer key={message.id} result={message.content} withPhotos={photos.length > 0} />
        case 'user': return <div key={message.id} className="rc-user"><p>{message.content.text}</p></div>
        case 'documents': return <div key={message.id} className="rc-user"><p>Документы для ввоза · <code>{formatCode(message.content.code)}</code></p></div>
        case 'question': return <div key={message.id} className="rc-bot rc-question"><b>Уточните</b>{message.content.text}</div>
        case 'match': return message.content.matches.length > 0 ? <BaseHits key={message.id} items={message.content.matches.map((match) => ({ id: match.decision.id, code: match.decision.code, name: match.decision.finalName || match.decision.invoiceName, thumb: match.decision.thumb, note: match.byPhoto ? 'то же фото' : 'то же наименование', verified: match.decision.status === 'verified' }))} title="Этот товар уже оформляли" /> : null
        // Только записи, которые сверка фото подтвердила («тот же», «похожий»): похожесть по описанию без
        // сверки приводила мультиметр к салфеткам.
        case 'similar': {
          const confirmed = message.content.matches.filter((match) => match.verdict === 'same' || match.verdict === 'similar')
          return confirmed.length > 0 ? <BaseHits key={message.id} items={confirmed.map((match) => ({ id: match.decision.id, code: match.decision.code, name: match.decision.finalName || match.decision.invoiceName, thumb: match.decision.thumb, note: match.verdict === 'same' ? `тот же товар · ${match.score}%` : `похожий · ${match.score}%`, verified: match.decision.status === 'verified' }))} title="Похожие товары в базе решений" /> : null
        }
        case 'result': return message.content.documentsCode ? <DocumentsAnswer key={message.id} result={message.content} />
          : <Answer key={message.id} sessionId={session?.id ?? 0} result={message.content} previous={block.previous} documentsDisabled={documentsDisabled} onDocuments={onDocuments} />
        case 'notice': return <p key={message.id} className="rc-notice">{message.content.text}</p>
        case 'error': return <p key={message.id} className="rc-error">{message.content.message}
          {block.retryDocuments && <button type="button" className="rc-retry" onClick={() => onDocuments(block.retryDocuments!)} disabled={documentsDisabled}>Повторить</button>}
        </p>
        case 'rerun': return <p key={message.id} className="rc-divider"><span>заново</span></p>
        default: return null
      }
    })}
    {session.status === 'searching' && <p className="rc-typing"><span /><span /><span /> ищу в базе решений…</p>}
    {session.status === 'running' && <p className="rc-typing"><span /><span /><span /> {documentsRequested(session.messages) ? 'агенты разбирают справку и перечни параллельно…' : 'агент ищет код…'}</p>}
  </>
}

// previous — прошлый ответ в разговоре: с ним сравниваются коды ответа на уточнение. retryDocuments — у
// ошибки запроса «Документы для ввоза»: код для кнопки «Повторить».
type Block = { type: 'steps'; messages: AgentMessage[] } | { type: 'message'; message: AgentMessage; previous?: AgentResult; retryDocuments?: string }

function threadBlocks(messages: AgentMessage[]): Block[] {
  const blocks: Block[] = []
  let previous: AgentResult | undefined
  let documents = ''
  for (const message of messages) {
    if (message.kind === 'documents') documents = message.content.code
    else if (message.kind === 'user' || message.kind === 'rerun' || message.kind === 'result') documents = ''
    if (message.kind === 'error') {
      blocks.push({ type: 'message', message, retryDocuments: documents || undefined })
      continue
    }
    if (message.kind === 'result') {
      blocks.push({ type: 'message', message, previous })
      // Ответ про документы — не ответ о коде: следующее уточнение сравнивается с прежним.
      if (!message.content.documentsCode) previous = message.content
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
      ? <li key={message.id} className={[message.content.failed && 'failed', (message.content.tool === 'photo' || message.content.tool === 'look_at_photo') && 'block'].filter(Boolean).join(' ') || undefined}><span aria-hidden="true">{toolIcons[message.content.tool] ?? '•'}</span>{message.content.label}<small>{message.content.detail}</small>{message.content.url && <a href={message.content.url} target="_blank" rel="noreferrer">↗</a>}</li>
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
type AnswerProps = { sessionId: number; result: AgentResult; previous?: AgentResult; documentsDisabled: boolean; onDocuments: (code: string) => void }

function Answer({ sessionId, result, previous, documentsDisabled, onDocuments }: AnswerProps) {
  const main = result.candidates.filter((candidate) => candidate.verdict !== 'no')
  const rejected = result.candidates.filter((candidate) => candidate.verdict === 'no')
  if (previous && main.length > 0 && mainCodes(previous) === mainCodes(result)) {
    return <div className="rc-answer rc-answer-short">
      {sameAnswer(result.description, previous.description) ? <p className="rc-answer-text rc-muted">Ответ тот же.</p>
        : result.description && <p className="rc-answer-text"><LinkText text={result.description} /></p>}
      {result.notes && <p className="rc-note"><LinkText text={result.notes} /></p>}
      {result.trademarks && <TrademarkHits hits={result.trademarks} />}
      <p className="rc-same-code">
        <span>{main.length > 1 ? 'Коды без изменений' : 'Код без изменений'}</span>
        {main.map((candidate) => <CopyCode key={candidate.code} code={candidate.code} className="rc-code small" />)}
      </p>
    </div>
  }
  return <div className="rc-answer">
    {result.description && <p className="rc-answer-text"><LinkText text={result.description} /></p>}
    <AgentEvidence result={result} />
    {main.map((candidate) => <Candidate key={candidate.code} candidate={candidate} documentsDisabled={documentsDisabled} onDocuments={onDocuments} />)}
    {rejected.length > 0 && <div className="rc-rejected">
      <p>Когда был бы другой код</p>
      {rejected.map((candidate) => <div key={candidate.code} className="rc-rejected-row">
        <CopyCode code={candidate.code} className="rc-code small" />
        <span><LinkText text={candidate.reason} /> <DeclarationsLink candidate={candidate} className="rc-more" /></span>
      </div>)}
    </div>}
    <DeclarationNames sessionId={sessionId} short={result.name} full={result.nameFull ?? ''} />
    {result.notes && <p className="rc-note"><LinkText text={result.notes} /></p>}
    {result.trademarks && <TrademarkHits hits={result.trademarks} />}
  </div>
}

// Наименование для декларации — краткое и подробное: копировать или перевести на китайский
// («русское / 中文», перевод — тем же запросом, что «Применить наименование» в подборе строки).
function DeclarationNames({ sessionId, short, full }: { sessionId: number; short: string; full: string }) {
  const variants = [{ label: 'Краткое', name: short.trim() }, { label: 'Подробное', name: full.trim() }]
    .filter((variant, index, all) => variant.name && all.findIndex((other) => other.name === variant.name) === index)
  if (variants.length === 0) return null
  return <div className="rc-names">
    <p>Наименование для декларации</p>
    {variants.map((variant) => <NameVariant key={variant.label} sessionId={sessionId} label={variants.length > 1 ? variant.label : ''} name={variant.name} />)}
  </div>
}

function NameVariant({ sessionId, label, name }: { sessionId: number; label: string; name: string }) {
  const [shown, setShown] = useState(name)
  const [busy, setBusy] = useState(false)
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState('')
  const translated = shown !== name

  async function copy() {
    try {
      await navigator.clipboard.writeText(shown)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1400)
    } catch {
      // Буфер обмена недоступен — текст можно выделить и скопировать вручную.
    }
  }

  async function translate() {
    setBusy(true); setError('')
    try { setShown((await agentApi.name(sessionId, name)).name) } catch (reason) { setError(errorMessage(reason)) } finally { setBusy(false) }
  }

  return <div className="rc-name-row">
    {label && <span className="rc-name-label">{label}</span>}
    <span className="rc-name-text">{shown}</span>
    <span className="rc-name-actions">
      <button type="button" className={copied ? 'rc-copy copied' : 'rc-copy'} onClick={() => void copy()}><CopyIcon copied={copied} />{copied ? 'Скопировано' : 'Копировать'}</button>
      {!translated && sessionId > 0 && <button type="button" onClick={() => void translate()} disabled={busy}>{busy ? 'Перевод…' : 'Перевести на китайский'}</button>}
    </span>
    {error && <span className="rc-name-error" role="alert">{error}</span>}
  </div>
}

function Candidate({ candidate, documentsDisabled, onDocuments }: { candidate: AgentCandidate; documentsDisabled: boolean; onDocuments: (code: string) => void }) {
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
      {nomenclature.antiDumping?.map((measure) => <span key={measure.number} className={measure.exact && !measure.suspended ? 'dump' : 'dump maybe'} title={`${measure.number} «${measure.shortName}»`}>
        {measure.suspended ? 'антидемпинг не применяется' : `${measure.exact ? 'антидемпинг' : 'возможен антидемпинг'} ${measure.countries.map((country) => `${(measure.rates[country] ?? '').replace('.', ',')}%`).join(' / ')} (${measure.countryNames})`}
      </span>)}
    </p>}
    {candidate.quote && <blockquote>{candidate.quote}{quoteSourceLabels[candidate.quoteSource] && <cite>{quoteSourceLabels[candidate.quoteSource]}</cite>}</blockquote>}
    {candidate.decisionQuote && <blockquote>{candidate.decisionQuote}<cite>{candidate.decisionSource} (TWS.BY)</cite></blockquote>}
    {candidate.reason && <p className="rc-reason"><LinkText text={candidate.reason} /></p>}
    {candidate.check && <p className="rc-check"><b>Уточнить:</b> <LinkText text={candidate.check} /></p>}
    <div className="rc-cand-actions">
      {candidate.verdict !== 'no' && <DocumentsButton code={candidate.code} disabled={documentsDisabled} onClick={onDocuments} />}
      {nomenclature?.detailsUrl && <a className="rc-more" href={nomenclature.detailsUrl} target="_blank" rel="noreferrer">Страница кода ↗</a>}
      <DeclarationsLink candidate={candidate} className="rc-more" />
    </div>
  </article>
}
