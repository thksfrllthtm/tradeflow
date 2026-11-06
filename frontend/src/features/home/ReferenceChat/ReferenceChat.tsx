import { ClipboardEvent, DragEvent, FormEvent, KeyboardEvent, useCallback, useEffect, useRef, useState } from 'react'
import { PhotoLightbox } from '@/shared/components/PhotoLightbox/PhotoLightbox'
import { errorMessage } from '@/shared/lib/errorMessage'
import { clipboardImages, clipboardText } from '@/shared/lib/clipboardImages'
import { resizePhoto } from '@/shared/lib/resizePhoto'
import { AgentSession, AgentStatus, ReferenceSummary, agentApi } from '@/features/goods/agent/agentApi'
import { scrollToDocuments } from '@/features/goods/agent/DocumentsAnswer'
import { Command, documentsCode, menuCommands, parseCommand, splitDocumentsArg } from './commands'
import { authorName, isWorking, liveText, readOpenQuestion, saveOpenQuestion } from './chatSession'
import { HistoryList } from './HistoryList'
import { Thread } from './Thread'
import { RoadGame } from './RoadGame'
import { ShareDialog } from './ShareDialog'
import './ReferenceChat.css'

const pollInterval = 1500
// Список истории, пока в нём есть вопрос, который ещё ищет.
const historyPollInterval = 5000
const maxPhotos = 3

const isImage = (file: File) => file.type === 'image/jpeg' || file.type === 'image/png'
// Вставку и «/» обрабатываем только вне полей ввода и открытых окон.
const editable = (target: EventTarget | null) => target instanceof HTMLElement && Boolean(target.closest('input, textarea, select, [contenteditable=""], [contenteditable="true"]'))
const dialogOpen = () => document.querySelector('[role="dialog"][aria-modal="true"]') !== null

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
  const [shareOpen, setShareOpen] = useState(false)
  const [lightbox, setLightbox] = useState<{ photos: string[]; index: number } | null>(null)
  const [menuIndex, setMenuIndex] = useState(0)
  const menu = menuCommands(text)
  const menuActive = menu[Math.min(menuIndex, menu.length - 1)]
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
      // Вопрос коллеги только для чтения: написанное — новый свой вопрос.
      if (fresh || !session || photos.length > 0 || session.trademarkCheck || session.shared) {
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
    if (!session || session.shared || busy || isWorking(session)) return
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
    // Вопрос коллеги не удаляется, а убирается из своей истории (доступ закрывается себе).
    const item = history.find((entry) => entry.id === id)
    const shared = item?.shared ? authorName(item.sharedBy) : ""
    if (!window.confirm(shared ? `Убрать вопрос от ${shared} из вашей истории?` : 'Удалить этот вопрос из истории?')) return
    try {
      await (shared ? agentApi.unshare(id, 'me') : agentApi.removeQuestion(id))
      if (session?.id === id) setSession(null)
      void loadHistory()
    } catch (reason) { setError(errorMessage(reason)) }
  }

  function startNew() {
    setSession(null); setText(''); setPhotos([]); setError(''); setHistoryOpen(false)
    inputRef.current?.focus()
  }

  const configured = status?.configured !== false
  // Лимит вопросов на сегодня: от 80% — подсветка (лимит не меняется, только видно, что он близко).
  const nearLimit = status !== null && status.dailyLimit > 0 && status.usedToday >= status.dailyLimit * .8
  const placeholder = !session ? 'Опишите товар, вставьте фото (Ctrl+V) или перетащите его сюда…'
    : session.shared ? 'Свой вопрос о товаре: опишите его или добавьте фото…'
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
        <button type="button" className="secondary rc-history-toggle" onClick={() => setHistoryOpen((value) => !value)} aria-expanded={historyOpen}>История{history.length > 0 && ` · ${history.length}`}</button>
        {/* Всегда на месте: с пустого экрана видно, где начать заново (без разговора — неактивна). */}
        <button type="button" className="secondary" onClick={startNew} disabled={!session} title={session ? 'Начать новый вопрос' : 'Вы уже в новом вопросе'}>+ Новый вопрос</button>
      </div>
    </header>

    <div className="rc-body">
      <HistoryList history={history} open={historyOpen} activeId={session?.id} onOpen={(id) => void open(id)} onRemove={(id) => void remove(id)} />

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
            documentsDisabled={busy || working || !configured || Boolean(session.shared)} onDocuments={(code) => void requestDocuments(code)}
            onShare={session.shared || working ? undefined : () => setShareOpen(true)} />}
          {session?.shared && <p className="rc-shared-note">Это чат {authorName(session.sharedBy)}: только просмотр. Свой вопрос задайте в поле ниже — он будет вашим.</p>}
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
            {session?.status === 'running' && !session.shared && photos.length === 0
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
    {shareOpen && session && <ShareDialog sessionId={session.id} onClose={() => setShareOpen(false)} />}
  </section>
}
