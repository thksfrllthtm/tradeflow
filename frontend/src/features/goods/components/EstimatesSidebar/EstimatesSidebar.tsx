import { useEffect, useRef, useState } from 'react'
import { Link } from '@/shared/router/Link'
import { GoodsEstimateSummary } from '@/shared/types/goods'
import { estimateSubtitle } from '@/shared/lib/estimateTitle'
import { formatRuDate } from '@/shared/lib/dates'
import './EstimatesSidebar.css'

type Props = {
  title: string
  estimates: GoodsEstimateSummary[]
  selectedId: number | null
  // docked — список стоит слева открытым (на странице больше ничего нет), иначе — узкая полоса.
  docked: boolean
  loading: boolean
  nameOf: (estimate: GoodsEstimateSummary) => string
  hrefOf: (id: number) => string
  onCreate: () => void
  onRemove: (estimate: GoodsEstimateSummary) => void
}

// Список смет. Смета открыта (или начальный экран новой) — слева узкая полоса («Сметы», «+»), а список
// выезжает поверх и остаётся открытым при выборе сметы; закрывается кликом мимо или Esc. Иначе (docked)
// список стоит слева открытым: больше на странице ничего нет.
export function EstimatesSidebar({ title, estimates, selectedId, docked, loading, nameOf, hrefOf, onCreate, onRemove }: Props) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const railRef = useRef<HTMLElement>(null)
  const needle = query.trim().toLowerCase()
  const visible = needle ? estimates.filter((estimate) => [nameOf(estimate), estimate.client, estimate.ownerName, formatRuDate(estimate.assessmentDate)].join(' ').toLowerCase().includes(needle)) : estimates

  useEffect(() => {
    if (!open) return
    const outside = (event: MouseEvent) => { if (!railRef.current?.contains(event.target as Node)) setOpen(false) }
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') setOpen(false) }
    document.addEventListener('mousedown', outside)
    document.addEventListener('keydown', escape)
    return () => { document.removeEventListener('mousedown', outside); document.removeEventListener('keydown', escape) }
  }, [open])

  const list = <>
    <div className="es-head">
      <h2>{title}{estimates.length > 0 && <span>{estimates.length}</span>}</h2>
      {!docked && <button type="button" className="es-icon" onClick={() => setOpen(false)} title="Закрыть список (Esc)" aria-label="Закрыть список смет">×</button>}
    </div>
    <div className="es-tools">
      <button type="button" className="es-create" onClick={() => { setOpen(false); onCreate() }}>
        <svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round"><path d="M7 2.5v9M2.5 7h9" /></svg>
        Новая смета
      </button>
      {estimates.length > 5 && <label className="es-search">
        <svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"><circle cx="6" cy="6" r="4.2" /><path d="m9.2 9.2 3 3" /></svg>
        <input type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Название, клиент, дата" aria-label="Поиск сметы" autoFocus={!docked} />
      </label>}
    </div>
    {loading ? <p className="es-note">Загрузка…</p>
      : estimates.length === 0 ? <p className="es-note">Смет пока нет. Создайте первую.</p>
      : visible.length === 0 ? <p className="es-note">Ничего не найдено.</p>
      : <nav className="es-list" aria-label="Список смет">
        {visible.map((estimate) => {
          const name = nameOf(estimate)
          const date = formatRuDate(estimate.assessmentDate)
          const calculated = estimate.status === 'Рассчитано'
          return <div className={estimate.id === selectedId ? 'es-item active' : 'es-item'} key={estimate.id}>
            <Link href={hrefOf(estimate.id)} aria-current={estimate.id === selectedId ? 'page' : undefined} title={name}>
              <span className="es-item-top">
                <strong>{name}</strong>
                <i className={calculated ? 'es-status done' : 'es-status'}>{estimate.status}</i>
              </span>
              <span className="es-item-sub">
                <span className={estimate.client.trim() ? undefined : 'es-none'}>{estimateSubtitle(estimate)}{estimate.ownerName && <em className="es-owner" title="Сотрудник, создавший смету"> · {estimate.ownerName}</em>}</span>
                {/* Дата уже в своём названии («Поставка · 21.10.2026») — второй раз не пишем. */}
                {!name.includes(date) && <span>{date}</span>}
              </span>
            </Link>
            <button type="button" className="es-remove" onClick={() => onRemove(estimate)} title="Удалить смету" aria-label={`Удалить смету ${name}`}>×</button>
          </div>
        })}
      </nav>}
  </>

  if (docked) return <aside className="es-sidebar" aria-label={title}>{list}</aside>

  return <aside className={open ? 'es-sidebar es-rail open' : 'es-sidebar es-rail'} aria-label={title} ref={railRef}>
    <button type="button" className="es-rail-button" onClick={() => setOpen(!open)} aria-expanded={open} title={open ? 'Закрыть список смет' : 'Открыть список смет'}>
      <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round"><path d="M3 4.5h12M3 9h12M3 13.5h12" /></svg>
      <span>Сметы</span>
      {estimates.length > 0 && <b>{estimates.length}</b>}
    </button>
    <button type="button" className="es-icon es-icon-primary" onClick={onCreate} title="Новая смета" aria-label="Новая смета">+</button>
    {open && <div className="es-panel" role="dialog" aria-label={title}>{list}</div>}
  </aside>
}
