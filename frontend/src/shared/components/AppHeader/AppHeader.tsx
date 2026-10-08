import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { isAdmin, roleLabels, User } from '@/shared/types/user'
import { Link } from '@/shared/router/Link'
import { paths } from '@/shared/router/router'
import { useTodayRates } from '@/shared/api/todayRates'
import { RatesConverter } from './RatesConverter'
import './AppHeader.css'

// home — главная: ни один раздел меню не подсвечен, на неё ведёт логотип.
type AppSection = 'home' | 'estimates' | 'products' | 'nomenclature' | 'decisions' | 'ceramics' | 'employees'

const sections: { value: AppSection; label: string; href: string }[] = [
  { value: 'estimates', label: 'Сметы', href: paths.goods() },
  { value: 'products', label: 'Справочник товаров', href: paths.products },
  { value: 'nomenclature', label: 'Номенклатура', href: paths.nomenclature },
  { value: 'decisions', label: 'База решений', href: paths.decisions },
  { value: 'ceramics', label: 'Керамика', href: paths.ceramics },
]

const adminSections: typeof sections = [
  { value: 'employees', label: 'Сотрудники', href: paths.employees },
]

function initials(user: User) {
  const source = user.name?.trim() || user.email
  const parts = source.split(/[\s@._-]+/).filter(Boolean)
  return ((parts[0]?.[0] ?? '') + (parts[1]?.[0] ?? '')).toUpperCase() || '?'
}

// Ширина «Ещё ▾» и промежуток между пунктами меню, px.
const MORE_WIDTH = 76
const GAP = 4

// Общая шапка всех страниц: логотип, разделы, курс НБРК на сегодня с конвертером валют, пользователь и выход. Разделы, которые не
// помещаются по ширине (окно рядом с Excel), уходят в меню «Ещё» — ничего не обрезается.
export function AppHeader({ user, section, onLogout }: { user: User; section: AppSection; onLogout: () => void }) {
  const items = isAdmin(user) ? [...sections, ...adminSections] : sections
  const navRef = useRef<HTMLElement>(null)
  const measureRef = useRef<HTMLDivElement>(null)
  const moreRef = useRef<HTMLDetailsElement>(null)
  const [visible, setVisible] = useState(items.length)
  const rates = useTodayRates()

  // Сколько пунктов помещается: ширины пунктов берутся из скрытой копии меню.
  useLayoutEffect(() => {
    const nav = navRef.current
    const measure = measureRef.current
    if (!nav || !measure) return
    const fit = () => {
      const widths = [...measure.children].map((child) => (child as HTMLElement).offsetWidth)
      const available = nav.clientWidth
      let used = 0
      let count = 0
      for (const [index, width] of widths.entries()) {
        const rest = index < widths.length - 1 ? MORE_WIDTH + GAP : 0
        if (used + width + rest > available) break
        used += width + GAP
        count++
      }
      setVisible(count)
    }
    fit()
    const observer = new ResizeObserver(fit)
    observer.observe(nav)
    return () => observer.disconnect()
  }, [items.length])

  // Меню «Ещё» закрывается щелчком вне его и клавишей Esc (фокус — обратно на «Ещё»).
  useEffect(() => {
    const close = (event: Event) => {
      const more = moreRef.current
      if (!more?.open) return
      if (event instanceof KeyboardEvent) {
        if (event.key !== 'Escape') return
        more.open = false
        more.querySelector('summary')?.focus()
        return
      }
      if (!more.contains(event.target as Node)) more.open = false
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', close)
    return () => { document.removeEventListener('mousedown', close); document.removeEventListener('keydown', close) }
  }, [])

  const shown = items.slice(0, visible)
  const hidden = items.slice(visible)
  const link = (item: (typeof items)[number]) => <Link key={item.value} href={item.href} className={item.value === section ? 'active' : undefined} aria-current={item.value === section ? 'page' : undefined}>{item.label}</Link>

  return <header className="app-header">
    <Link className="app-header-brand" href={paths.home} aria-label="TradeFlow — на главную">
      <span className="app-header-logo" aria-hidden="true">TF</span>
      <span className="app-header-name">TradeFlow</span>
    </Link>
    <nav className="app-header-nav" aria-label="Разделы" ref={navRef}>
      {shown.map(link)}
      {hidden.length > 0 && <details className={hidden.some((item) => item.value === section) ? 'app-header-more active' : 'app-header-more'} ref={moreRef}>
        <summary>Ещё<svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><path d="M2.5 4l2.5 2.5L7.5 4" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" /></svg></summary>
        <div className="app-header-more-menu" onClick={() => { if (moreRef.current) moreRef.current.open = false }}>{hidden.map(link)}</div>
      </details>}
      <div className="app-header-measure" ref={measureRef} aria-hidden="true">{items.map((item) => <span key={item.value}>{item.label}</span>)}</div>
    </nav>
    {rates && rates.length > 0 && <RatesConverter rates={rates} />}
    <div className="app-header-user">
      <span className="app-header-avatar" aria-hidden="true">{initials(user)}</span>
      <span className="app-header-who" title={user.email}>{user.name || user.email}<small>{roleLabels[user.role] ?? ''}</small></span>
      <button type="button" className="app-header-logout" onClick={onLogout}>Выйти</button>
    </div>
  </header>
}
