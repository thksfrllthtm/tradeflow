import { KeyboardEvent, useRef, useState } from 'react'
import { ReferenceSummary } from '@/features/goods/agent/agentApi'
import { formatCode } from '@/shared/tnved/codes'
import { authorName } from './chatSession'

// Строка под вопросом в истории: что с ним сейчас, затем код.
function historyState(item: ReferenceSummary) {
  if (item.status === 'running' || item.status === 'searching') return <b className="rc-state busy">{item.kind === 'trademarks' ? 'проверяет…' : 'ищет…'}</b>
  if (item.status === 'waiting') return <b className="rc-state busy">ждёт ответа</b>
  if (item.status === 'error' || item.status === 'cancelled') return <b className="rc-state bad">не завершён</b>
  if (item.kind === 'trademarks') return item.trademarkHits ? `ТРОИС: найдено ${item.trademarkHits}` : 'ТРОИС: нет в реестре'
  return item.bestCode ? <code>{formatCode(item.bestCode)}</code> : 'код не найден'
}

const kindLabels: Record<ReferenceSummary['kind'], string> = { chat: 'Чат', trademarks: 'Проверка ТРОИС', documents: 'Документы для ввоза' }

// Значки вида вопроса: чат — облачко, ТРОИС — щит (защита знака), документы — лист.
const kindPaths: Record<ReferenceSummary['kind'], string[]> = {
  chat: ['M3 3h10a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H7.2L4.5 13.4V11H3a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1Z'],
  trademarks: ['M8 1.8 3 3.7v3.9c0 3 2.1 5.3 5 6.6 2.9-1.3 5-3.6 5-6.6V3.7L8 1.8Z', 'm5.9 8 1.5 1.5 2.8-3'],
  documents: ['M4 1.8h5.4L12.4 5v9.2H4Z', 'M9.4 1.8V5h3', 'M6 8.2h4.4M6 10.8h4.4'],
}

function KindIcon({ kind, label }: { kind: ReferenceSummary['kind']; label?: string }) {
  return <svg className={`rc-kind ${kind}`} width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round"
    role={label ? 'img' : undefined} aria-label={label} aria-hidden={label ? undefined : true}>
    {kindPaths[kind].map((path) => <path key={path} d={path} />)}
  </svg>
}

function PersonIcon() {
  return <svg width="12" height="12" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"><circle cx="8" cy="5.5" r="2.6" /><path d="M2.8 14c.7-2.6 2.7-4 5.2-4s4.5 1.4 5.2 4" /></svg>
}

type HistoryListProps = { history: ReferenceSummary[]; open: boolean; activeId?: number; onOpen: (id: number) => void; onRemove: (id: number) => void }

// Прошлые вопросы сотрудника. Одна остановка Tab: по вопросам — стрелками (roving tabindex).
export function HistoryList({ history, open, activeId, onOpen, onRemove }: HistoryListProps) {
  const [historyFocus, setHistoryFocus] = useState(0)
  const historyRef = useRef<HTMLUListElement>(null)
  const rovingIndex = Math.min(historyFocus, Math.max(0, history.length - 1))

  function onHistoryKey(event: KeyboardEvent<HTMLButtonElement>, index: number, id: number) {
    const last = history.length - 1
    const next = event.key === 'ArrowDown' ? Math.min(last, index + 1) : event.key === 'ArrowUp' ? Math.max(0, index - 1) : event.key === 'Home' ? 0 : event.key === 'End' ? last : -1
    if (next >= 0) {
      event.preventDefault()
      setHistoryFocus(next)
      historyRef.current?.querySelectorAll<HTMLButtonElement>('.rc-history-open')[next]?.focus()
    } else if (event.key === 'Delete') {
      event.preventDefault()
      onRemove(id)
    }
  }

  return <aside className={open ? 'rc-history open' : 'rc-history'} aria-label="Прошлые вопросы">
    <p className="rc-history-title">История вопросов</p>
    {history.length === 0 ? <p className="rc-history-empty">Здесь появятся ваши вопросы.</p>
      : <>
        <p id="rc-history-help" className="rc-sr">Стрелки вверх и вниз — по вопросам, Enter — открыть, Delete — удалить</p>
        <ul ref={historyRef}>{history.map((item, index) => <li key={item.id} className={[activeId === item.id && 'active', item.shared && 'shared'].filter(Boolean).join(' ') || undefined}>
          <button type="button" className="rc-history-open" tabIndex={index === rovingIndex ? 0 : -1} onFocus={() => setHistoryFocus(index)} onKeyDown={(event) => onHistoryKey(event, index, item.id)} onClick={() => onOpen(item.id)} title={item.title} aria-describedby="rc-history-help" aria-current={activeId === item.id ? 'true' : undefined} aria-keyshortcuts="Delete">
            <span className="rc-history-name"><KindIcon kind={item.kind} label={kindLabels[item.kind]} />{item.hasPhoto && <svg className="rc-history-photo" width="12" height="12" viewBox="0 0 12 12" aria-label="с фото" role="img" fill="none" stroke="currentColor" strokeWidth="1.3"><rect x="1.5" y="3" width="9" height="7" rx="1.5" /><circle cx="6" cy="6.5" r="1.6" /><path d="M4.2 3 5 1.8h2L7.8 3" /></svg>}{item.title}</span>
            {item.shared && <span className="badge info rc-history-from"><PersonIcon />Чат {authorName(item.sharedBy)}</span>}
            <small>{historyState(item)} ·<time dateTime={item.updatedAt}>{new Date(item.updatedAt).toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })}</time></small>
          </button>
          <button type="button" className="icon-button ghost sm rc-history-remove" tabIndex={-1} onClick={() => onRemove(item.id)} aria-label={`Удалить вопрос «${item.title}»`} title="Удалить (Delete)">×</button>
        </li>)}</ul>
      </>}
  </aside>
}
