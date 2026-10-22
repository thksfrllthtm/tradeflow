import { SearchIcon } from '@/shared/components/icons'

const statuses = [
  { value: 'all', label: 'Все' },
  { value: 'operator_verified', label: 'Проверены' },
  { value: 'needs_review', label: 'Требуют проверки' },
]

type Props = { query: string; placeholder: string; status: string; showArchived: boolean; count?: string; onQuery: (value: string) => void; onStatus: (value: string) => void; onShowArchived: (value: boolean) => void }

// Верхняя полоса карточки справочника (как в «Базе решений»): поиск во всю ширину, фильтр по статусу,
// показ архива и справа — сколько записей.
export function DirectoryToolbar({ query, placeholder, status, showArchived, count, onQuery, onStatus, onShowArchived }: Props) {
  return <div className="dir-filterbar">
    <label className="dir-search">
      <SearchIcon />
      <input type="search" value={query} onChange={(event) => onQuery(event.target.value)} placeholder={placeholder} aria-label="Поиск" autoFocus />
    </label>
    <div className="dir-seg" role="radiogroup" aria-label="Статус проверки">
      {statuses.map((option) => <button type="button" role="radio" aria-checked={status === option.value} key={option.value} className={status === option.value ? 'active' : undefined} onClick={() => onStatus(option.value)}>{option.label}</button>)}
    </div>
    <label className="dir-archive"><input type="checkbox" checked={showArchived} onChange={(event) => onShowArchived(event.target.checked)} />Показать архив</label>
    {count && <span className="dir-filter-count">{count}</span>}
  </div>
}
