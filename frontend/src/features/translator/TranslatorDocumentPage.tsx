import { useEffect, useRef, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { PageHead } from '@/shared/components/PageHead/PageHead'
import { SearchIcon } from '@/shared/components/icons'
import { formatRuDate } from '@/shared/lib/dates'
import { errorMessage } from '@/shared/lib/errorMessage'
import { plural } from '@/shared/lib/plural'
import { downloadFile } from '@/shared/api/download'
import { Link } from '@/shared/router/Link'
import { navigate, paths } from '@/shared/router/router'
import { isAdmin, User } from '@/shared/types/user'
import { HistoryDrawer } from './HistoryDrawer'
import { SegmentEditor } from './SegmentEditor'
import { TaggedText } from './TaggedText'
import { documentStatusLabels, documentStatusTone, languageLabel, Segment, SegmentStatus, segmentStatusLabels, skippedSummary, TranslatorDocument, translatorApi } from './translatorApi'
import '@/shared/styles/list-page.css'
import './translator.css'

// Как часто страница забирает новые сегменты, пока идёт перевод (как подбор кода ИИ-агентом).
const pollInterval = 1500
// Автосохранение правки через секунду после последнего изменения.
const saveDelay = 1000

const translated = (segment: Segment) => segment.locked || segment.targetText !== '' || segment.mtText !== ''
const problem = (segment: Segment) => segment.qa.length > 0 || segment.status === 'source_changed'

type Filter = 'all' | 'unconfirmed' | 'problems'
const filters: { value: Filter; label: string; test: (segment: Segment) => boolean }[] = [
  { value: 'all', label: 'Все', test: () => true },
  { value: 'unconfirmed', label: 'Неподтверждённые', test: (segment) => segment.status !== 'confirmed' },
  { value: 'problems', label: 'Проблемные', test: problem },
]
const statuses: SegmentStatus[] = ['machine', 'edited', 'confirmed', 'source_changed']

// Правка открытого сегмента: text — в редакторе, saved — уже на сервере.
type Draft = { id: number; text: string; saved: string }

// Документ: оригинал и перевод по сегментам в две колонки, строка — один абзац. Перевод идёт на
// сервере в фоне; страница опрашивает его и подставляет готовые сегменты. Щелчок по переводу
// открывает редактор: правка сохраняется сама, сегмент становится «исправлен» и автоперевод его
// больше не трогает.
export function TranslatorDocumentPage({ user, documentId, onLogout }: { user: User; documentId: number; onLogout: () => void }) {
  const [doc, setDoc] = useState<TranslatorDocument | null>(null)
  const [segments, setSegments] = useState<Segment[]>([])
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [filter, setFilter] = useState<Filter>('all')
  const [search, setSearch] = useState('')
  const [activeId, setActiveId] = useState<number | null>(null)
  // Меняется, когда текст открытого сегмента заменён не из редактора (история, «Перевести заново»).
  const [editorVersion, setEditorVersion] = useState(0)
  const [historyFor, setHistoryFor] = useState<Segment | null>(null)
  const [retranslating, setRetranslating] = useState<number | null>(null)
  const [toast, setToast] = useState<{ text: string; error: boolean; key: number } | null>(null)
  // Время последнего ответа сервера: следующий опрос получает только изменённые после него сегменты.
  const since = useRef('')
  const draft = useRef<Draft | null>(null)
  const saveTimer = useRef(0)
  // Сохранения идут по очереди, чтобы более ранняя правка не перезаписала более позднюю.
  const saving = useRef(Promise.resolve())

  useEffect(() => {
    let active = true
    translatorApi.get(documentId)
      .then((view) => { if (active) { setDoc(view.document); setSegments(view.segments); since.current = view.now } })
      .catch((reason) => { if (active) setError(errorMessage(reason)) })
    return () => { active = false }
  }, [documentId])

  const translating = doc?.status === 'translating'
  useEffect(() => {
    if (!translating) return
    let active = true
    let waiting = false
    const timer = window.setInterval(() => {
      if (waiting) return
      waiting = true
      translatorApi.get(documentId, since.current).then((view) => {
        if (!active) return
        const changed = new Map(view.segments.map((segment) => [segment.id, segment]))
        setSegments((current) => current.map((segment) => changed.get(segment.id) ?? segment))
        setDoc(view.document)
        since.current = view.now
      }).catch(() => { /* сеть пропала — следующий опрос повторит */ }).finally(() => { waiting = false })
    }, pollInterval)
    return () => { active = false; window.clearInterval(timer) }
  }, [translating, documentId])

  useEffect(() => {
    if (!toast) return
    const timer = window.setTimeout(() => setToast(null), toast.error ? 6000 : 1800)
    return () => window.clearTimeout(timer)
  }, [toast])

  // Уход со страницы — несохранённая правка сохраняется.
  useEffect(() => () => { void save() }, [])

  useEffect(() => {
    if (activeId !== null) window.document.getElementById(`tr-seg-${activeId}`)?.scrollIntoView({ block: 'nearest' })
  }, [activeId])

  const showToast = (text: string, isError = false) => setToast({ text, error: isError, key: Date.now() })
  const applySegment = (updated: Segment) => setSegments((current) => current.map((segment) => segment.id === updated.id ? updated : segment))

  // Сохраняет правку открытого сегмента (и статус, если он передан). true — сохранено или нечего сохранять.
  function save(status?: 'confirmed'): Promise<boolean> {
    window.clearTimeout(saveTimer.current)
    const current = draft.current
    if (!current) return Promise.resolve(true)
    const text = current.text
    const changed = text !== current.saved
    if (!changed && !status) return Promise.resolve(true)
    if (!text.trim() && !status) return Promise.resolve(true) // пустой перевод не сохраняем, пока сотрудник печатает
    const run = saving.current.then(async () => {
      try {
        const updated = await translatorApi.editSegment(current.id, { ...(changed ? { targetText: text } : {}), ...(status ? { status } : {}) })
        current.saved = text
        applySegment(updated)
        showToast(status === 'confirmed' ? 'Подтверждено' : 'Сохранено')
        return true
      } catch (reason) {
        showToast(errorMessage(reason), true)
        return false
      }
    })
    saving.current = run.then(() => undefined)
    return run
  }

  function open(segment: Segment) {
    if (activeId === segment.id) return
    void save()
    const text = segment.targetText || segment.mtText
    draft.current = { id: segment.id, text, saved: text }
    setActiveId(segment.id)
  }

  function close() {
    void save()
    draft.current = null
    setActiveId(null)
  }

  function move(step: 1 | -1) {
    const index = visible.findIndex((segment) => segment.id === activeId)
    const next = visible[index + step]
    if (next) open(next)
  }

  async function confirm() {
    const list = visible
    const index = list.findIndex((segment) => segment.id === activeId)
    if (await save('confirmed')) {
      const next = list[index + 1]
      if (next) open(next)
      else close()
    }
  }

  // Текст открытого сегмента заменён не из редактора: редактор открывается заново с новым текстом.
  function replaceDraft(updated: Segment) {
    if (draft.current?.id !== updated.id) return
    window.clearTimeout(saveTimer.current)
    const text = updated.targetText || updated.mtText
    draft.current = { id: updated.id, text, saved: text }
    setEditorVersion((version) => version + 1)
  }

  async function retranslate(segment: Segment) {
    if (segment.locked && !window.confirm('Сегмент исправлен вручную. Заменить правку новым машинным переводом? Прежние версии останутся в истории.')) return
    setRetranslating(segment.id)
    try {
      const updated = await translatorApi.retranslate(segment.id)
      applySegment(updated)
      replaceDraft(updated)
      showToast('Переведено заново')
    } catch (reason) {
      showToast(errorMessage(reason), true)
    } finally {
      setRetranslating(null)
    }
  }

  async function restore(segment: Segment, text: string) {
    const updated = await translatorApi.editSegment(segment.id, { targetText: text })
    applySegment(updated)
    replaceDraft(updated)
    showToast('Версия возвращена')
  }

  // Скачивание .docx: перед ним сохраняется открытая правка и предупреждение о неподтверждённых сегментах.
  async function download(mode: 'target' | 'bilingual') {
    if (!doc || !(await save())) return
    const unconfirmed = segments.filter((segment) => segment.status !== 'confirmed').length
    if (unconfirmed > 0 && !window.confirm(`Не подтверждено ${unconfirmed} ${plural(unconfirmed, 'сегмент', 'сегмента', 'сегментов')} из ${segments.length}. Скачать перевод как есть?`)) return
    setBusy(true)
    try {
      await downloadFile(translatorApi.exportPath(doc.id, mode), `${doc.name}_${mode === 'bilingual' ? `${doc.sourceLang}-` : ''}${doc.targetLang}.${doc.format}`, 'Не удалось собрать документ')
    } catch (reason) {
      showToast(errorMessage(reason), true)
    } finally {
      setBusy(false)
    }
  }

  async function translate() {
    if (!doc) return
    setBusy(true); setError('')
    try {
      await translatorApi.translate(doc.id)
      setDoc({ ...doc, status: 'translating', error: '' })
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setBusy(false)
    }
  }

  async function remove() {
    if (!doc || !window.confirm(`Удалить документ «${doc.name}» вместе с переводом?`)) return
    setBusy(true)
    try {
      await translatorApi.remove(doc.id)
      navigate(paths.translator())
    } catch (reason) {
      setError(errorMessage(reason))
      setBusy(false)
    }
  }

  // Подтвердить или снять подтверждение прямо из строки, не открывая редактор: так быстрее
  // просматривать машинный перевод, когда править нечего.
  async function quickStatus(segment: Segment, status: 'confirmed' | 'edited') {
    try {
      applySegment(await translatorApi.editSegment(segment.id, { status }))
      showToast(status === 'confirmed' ? 'Подтверждено' : 'Подтверждение снято')
    } catch (reason) {
      showToast(errorMessage(reason), true)
    }
  }

  const skipped = doc ? skippedSummary(doc.skipped) : ''
  const canDelete = doc !== null && (isAdmin(user) || doc.createdBy === user.id)
  const done = segments.filter(translated).length
  const counts = Object.fromEntries(statuses.map((status) => [status, segments.filter((segment) => segment.status === status).length]))
  const filterCounts = Object.fromEntries(filters.map((item) => [item.value, segments.filter(item.test).length]))
  const filterTest = filters.find((item) => item.value === filter)!.test
  // Поиск по оригиналу и переводу — поверх выбранного фильтра.
  const needle = search.trim().toLowerCase()
  const matches = (segment: Segment) => !needle || segment.sourceText.toLowerCase().includes(needle) || (segment.targetText || segment.mtText).toLowerCase().includes(needle)
  // Открытый сегмент не пропадает из списка, пока его правят (например, подтверждённый в «Неподтверждённых»).
  const visible = segments.filter((segment) => segment.id === activeId || (filterTest(segment) && matches(segment)))
  const number = new Map(segments.map((segment, index) => [segment.id, index + 1]))

  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="translator" onLogout={onLogout} />
    <section className="directory-page tr-page">
      <PageHead
        back={<Link className="secondary tr-back" href={paths.translator()}>← Все документы</Link>}
        title={<>{doc?.name ?? 'Документ'}{doc && <span className={`badge tr-status ${documentStatusTone[doc.status]}`}>{documentStatusLabels[doc.status]}</span>}</>}
        subtitle={doc && <>{languageLabel(doc.sourceLang)} → {languageLabel(doc.targetLang)} · загрузил {doc.createdByName || '—'} {formatRuDate(doc.createdAt.slice(0, 10))} · сегментов: {doc.segmentCount}</>}
        actions={doc && <>
          {canDelete && <button type="button" className="danger" onClick={() => void remove()} disabled={busy || translating} title="Удалить документ вместе с переводом">Удалить</button>}
          <button type="button" className="secondary" onClick={() => void download('target')} disabled={busy || done === 0} title="Исходный документ, где текст заменён переводом">Скачать перевод</button>
          {doc.format === 'docx' && <button type="button" className="secondary" onClick={() => void download('bilingual')} disabled={busy || done === 0} title="Под каждым абзацем оригинала — перевод серым курсивом">Скачать двуязычный</button>}
          {!translating && segments.length > 0 && done < segments.length && <button type="button" onClick={() => void translate()} disabled={busy}>{done === 0 ? 'Перевести' : 'Перевести оставшиеся'}</button>}
        </>} />
      {skipped && <p className="tr-warning" role="status">Не переведено: {skipped}. Эти части останутся в файле на языке оригинала.</p>}
      {doc?.status === 'failed' && doc.error && <p className="dir-transfer-error" role="alert">{doc.error}</p>}
      {error && <p className="dir-transfer-error" role="alert">{error}</p>}

      {/* Одна строка над таблицей: ход перевода (светлое — переведено, тёмное — подтверждено),
          счётчики статусов, поиск и фильтр. На узком экране переносится. */}
      {doc && segments.length > 0 && <div className="tr-toolbar">
        <div className="tr-doc-progress" role="status">
          <span className="tr-progress" aria-hidden="true">
            <span className="tr-progress-done" style={{ width: `${(done / segments.length) * 100}%` }} />
            <span className="tr-progress-confirmed" style={{ width: `${(counts.confirmed / segments.length) * 100}%` }} />
          </span>
          <span>{translating ? 'Переводим…' : 'Переведено'} <b>{done}</b> из {segments.length}</span>
        </div>
        <div className="tr-counts">
          {statuses.map((status) => <span key={status} className={`tr-count st-${status}${counts[status] === 0 ? ' zero' : ''}`}><i aria-hidden="true" />{segmentStatusLabels[status]} <b>{counts[status]}</b></span>)}
        </div>
        <label className="dir-search tr-search">
          <SearchIcon />
          <input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Найти в оригинале или переводе" aria-label="Поиск по сегментам" />
          {search && <button type="button" className="icon-button ghost sm" onClick={() => setSearch('')} aria-label="Очистить поиск">×</button>}
        </label>
        <div className="dir-seg" role="radiogroup" aria-label="Показать сегменты">
          {filters.map((item) => <button key={item.value} type="button" role="radio" aria-checked={filter === item.value} className={filter === item.value ? 'active' : undefined} onClick={() => setFilter(item.value)}>
            {item.label} <span className="tr-filter-count">{filterCounts[item.value]}</span>
          </button>)}
        </div>
      </div>}

      {doc && <div className="tr-segments">
        <div className="tr-seg tr-seg-head"><span>№</span><span>Оригинал · {languageLabel(doc.sourceLang)}</span><span>Перевод · {languageLabel(doc.targetLang)}</span></div>
        {visible.length === 0 && <p className="dir-empty">{needle ? 'Ничего не найдено' : filter === 'problems' ? 'Проблемных сегментов нет' : 'Все сегменты подтверждены'}</p>}
        {visible.map((segment, index) => {
          const active = segment.id === activeId
          const target = segment.targetText || segment.mtText
          return <div id={`tr-seg-${segment.id}`} className={`tr-seg st-${segment.status}${active ? ' active' : ''}${segment.qa.length > 0 ? ' has-qa' : ''}`} key={segment.id}>
            <span className="tr-seg-num" title={segmentStatusLabels[segment.status]}>{number.get(segment.id)}{segment.kind === 'table_cell' && doc.format === 'docx' && <small title="Абзац в ячейке таблицы">табл.</small>}</span>
            <div className="tr-seg-source"><TaggedText text={segment.sourceText} tags={segment.tags} /></div>
            {active
              ? <div className="tr-seg-target">
                <SegmentEditor key={`${segment.id}-${editorVersion}`} text={draft.current?.text ?? target} tags={segment.tags} actions={{
                  onChange: (text) => {
                    if (!draft.current) return
                    draft.current.text = text
                    window.clearTimeout(saveTimer.current)
                    saveTimer.current = window.setTimeout(() => void save(), saveDelay)
                  },
                  onConfirm: () => void confirm(),
                  onMove: move,
                  onClose: close,
                }} />
                {segment.qa.length > 0 && <small className="tr-qa tr-qa-chip">⚠ {segment.qa.map((item) => item.message).join(' · ')}</small>}
                {/* Одна строка: главное действие с подсказкой клавиш внутри, вспомогательные рядом,
                    переход по абзацам и закрытие — справа. Клавиши — в подсказках кнопок. */}
                <div className="tr-seg-tools">
                  <button type="button" className="tr-main" onClick={() => void confirm()} title="Подтвердить и перейти к следующему абзацу">✓ Подтвердить <kbd>Ctrl+Enter</kbd></button>
                  <button type="button" className="secondary" onClick={() => void retranslate(segment)} disabled={retranslating === segment.id}>{retranslating === segment.id ? 'Переводим…' : 'Перевести заново'}</button>
                  <button type="button" className="secondary" onClick={() => setHistoryFor(segment)}>История</button>
                  <div className="tr-seg-nav">
                    <button type="button" className="icon-button" onClick={() => move(-1)} disabled={index === 0} title="Предыдущий абзац (Alt+↑)" aria-label="Предыдущий абзац">↑</button>
                    <button type="button" className="icon-button" onClick={() => move(1)} disabled={index === visible.length - 1} title="Следующий абзац (Alt+↓)" aria-label="Следующий абзац">↓</button>
                    <button type="button" className="icon-button" onClick={close} title="Закрыть редактор (Esc)" aria-label="Закрыть редактор">×</button>
                  </div>
                </div>
              </div>
              : <div className="tr-seg-target editable" role="button" tabIndex={0} onClick={() => open(segment)}
                onKeyDown={(event) => { if (event.key === 'Enter' && event.target === event.currentTarget) { event.preventDefault(); open(segment) } }}>
                {target ? <TaggedText text={target} tags={segment.tags} /> : <span className="tr-seg-empty">{translating && !segment.locked ? 'Переводится…' : 'Нет перевода — щёлкните, чтобы ввести'}</span>}
                {segment.qa.length > 0 && <small className="tr-qa">⚠ {segment.qa.map((item) => item.message).join(' · ')}</small>}
                {/* Быстрое действие при наведении: подтвердить прочитанный перевод или снять подтверждение. */}
                {target && !translating && <span className="tr-quick">
                  {segment.status === 'confirmed'
                    ? <button type="button" className="secondary sm" onClick={(event) => { event.stopPropagation(); void quickStatus(segment, 'edited') }} title="Снять подтверждение">Снять</button>
                    : <button type="button" className="secondary sm confirm" onClick={(event) => { event.stopPropagation(); void quickStatus(segment, 'confirmed') }} title="Подтвердить, не открывая редактор">✓ Подтвердить</button>}
                </span>}
              </div>}
          </div>
        })}
      </div>}
    </section>
    {historyFor && <HistoryDrawer segment={historyFor} onClose={() => setHistoryFor(null)} onRestore={(text) => restore(historyFor, text)} />}
    {toast && <div key={toast.key} className={toast.error ? 'tr-toast error' : 'tr-toast'} role={toast.error ? 'alert' : 'status'}>{toast.text}</div>}
  </main>
}
