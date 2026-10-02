import { useState } from 'react'
import { Link } from '@/shared/router/Link'
import { GoodsEstimateSummary } from '@/shared/types/goods'
import { formatRuDate } from '@/shared/lib/dates'
import './EstimatesSidebar.css'

type Props = {
  title: string
  estimates: GoodsEstimateSummary[]
  selectedId: number | null
  loading: boolean
  nameOf: (estimate: GoodsEstimateSummary) => string
  hrefOf: (id: number) => string
  onCreate: () => void
  onRemove: (estimate: GoodsEstimateSummary) => void
}

// Список смет слева: создание, поиск (когда смет много), выбор по ссылке, удаление, сворачивание.
export function EstimatesSidebar({ title, estimates, selectedId, loading, nameOf, hrefOf, onCreate, onRemove }: Props) {
  const [collapsed, setCollapsed] = useState(false)
  const [query, setQuery] = useState('')
  const needle = query.trim().toLowerCase()
  const visible = needle ? estimates.filter((estimate) => [nameOf(estimate), estimate.client, estimate.ownerName, formatRuDate(estimate.assessmentDate)].join(' ').toLowerCase().includes(needle)) : estimates

  if (collapsed) return <aside className="es-sidebar collapsed" aria-label={title}>
    <button type="button" className="es-icon" onClick={() => setCollapsed(false)} title="Показать список смет" aria-label="Показать список смет">›</button>
    <button type="button" className="es-icon es-icon-primary" onClick={onCreate} title="Новая смета" aria-label="Новая смета">+</button>
  </aside>

  return <aside className="es-sidebar" aria-label={title}>
    <div className="es-head">
      <h2>{title}{estimates.length > 0 && <span>{estimates.length}</span>}</h2>
      <button type="button" className="es-icon" onClick={() => setCollapsed(true)} title="Свернуть список" aria-label="Свернуть список смет">‹</button>
    </div>
    <div className="es-tools">
      <button type="button" className="es-create" onClick={onCreate}>+ Новая смета</button>
      {estimates.length > 5 && <input className="es-search" type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Поиск: название, клиент, дата" aria-label="Поиск сметы" />}
    </div>
    {loading ? <p className="es-note">Загрузка…</p>
      : estimates.length === 0 ? <p className="es-note">Смет пока нет. Создайте первую.</p>
      : visible.length === 0 ? <p className="es-note">Ничего не найдено.</p>
      : <nav className="es-list" aria-label="Список смет">
        {visible.map((estimate) => {
          const name = nameOf(estimate)
          const calculated = estimate.status === 'Рассчитано'
          return <div className={estimate.id === selectedId ? 'es-item active' : 'es-item'} key={estimate.id}>
            <Link href={hrefOf(estimate.id)} aria-current={estimate.id === selectedId ? 'page' : undefined} title={name}>
              <span className="es-item-top">
                <strong>{name}</strong>
                <i className={calculated ? 'es-status done' : 'es-status'}>{estimate.status}</i>
              </span>
              <span className="es-item-sub">
                <span>{estimate.client || 'Без клиента'}{estimate.ownerName && <em className="es-owner" title="Сотрудник, создавший смету"> · {estimate.ownerName}</em>}</span>
                <span>{formatRuDate(estimate.assessmentDate)}</span>
              </span>
            </Link>
            <button type="button" className="es-remove" onClick={() => onRemove(estimate)} title="Удалить смету" aria-label={`Удалить смету ${name}`}>×</button>
          </div>
        })}
      </nav>}
  </aside>
}
