import { MouseEvent, useEffect, useRef } from 'react'
import { canEstimate, canTranslate, isAdmin, roleLabels, User } from '@/shared/types/user'
import { Link } from '@/shared/router/Link'
import { paths } from '@/shared/router/router'
import { useTodayRates } from '@/shared/api/todayRates'
import { RatesConverter } from './RatesConverter'
import './AppHeader.css'

// home — главная, на неё ведут логотип и пункт «Главная».
type AppSection = 'home' | 'estimates' | 'products' | 'nomenclature' | 'decisions' | 'ceramics' | 'utilFee' | 'exportCheck' | 'employees' | 'translator'
type Item = { value: AppSection; label: string; href: string }

const directories: Item[] = [
  { value: 'products', label: 'Справочник товаров', href: paths.products },
  { value: 'nomenclature', label: 'Номенклатура', href: paths.nomenclature },
  { value: 'decisions', label: 'База решений', href: paths.decisions },
  { value: 'ceramics', label: 'Керамика', href: paths.ceramics },
  { value: 'utilFee', label: 'Утильсбор', href: paths.utilFee },
]

// Разделы, закрытые переводчику.
const estimateSections: AppSection[] = ['estimates', 'products', 'decisions', 'ceramics', 'exportCheck']

function initials(user: User) {
  const source = user.name?.trim() || user.email
  const parts = source.split(/[\s@._-]+/).filter(Boolean)
  return ((parts[0]?.[0] ?? '') + (parts[1]?.[0] ?? '')).toUpperCase() || '?'
}

const chevron = <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><path d="M2.5 4l2.5 2.5L7.5 4" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" /></svg>

// Общая шапка всех страниц: логотип, разделы (справочники — в выпадающем меню), курс НБРК с
// конвертером валют и меню профиля (Сотрудники, Выйти). На узком экране меню переносится на вторую строку.
export function AppHeader({ user, section, onLogout }: { user: User; section: AppSection; onLogout: () => void }) {
  const allowed = (item: Item) => canEstimate(user) || !estimateSections.includes(item.value)
  const dirs = directories.filter(allowed)
  const headerRef = useRef<HTMLElement>(null)
  const rates = useTodayRates()

  // Выпадающие меню шапки закрываются щелчком вне их и клавишей Esc (фокус — обратно на кнопку меню).
  useEffect(() => {
    const close = (event: Event) => {
      for (const menu of headerRef.current?.querySelectorAll<HTMLDetailsElement>('details.app-menu[open]') ?? []) {
        if (event instanceof KeyboardEvent) {
          if (event.key !== 'Escape') continue
          menu.open = false
          menu.querySelector('summary')?.focus()
        } else if (!menu.contains(event.target as Node)) menu.open = false
      }
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', close)
    return () => { document.removeEventListener('mousedown', close); document.removeEventListener('keydown', close) }
  }, [])

  const closeMenu = (event: MouseEvent<HTMLElement>) => { const menu = event.currentTarget.closest('details'); if (menu) menu.open = false }
  const link = (item: Item) => <Link key={item.value} href={item.href} className={item.value === section ? 'active' : undefined} aria-current={item.value === section ? 'page' : undefined}>{item.label}</Link>
  const dirActive = dirs.some((item) => item.value === section)

  return <header className="app-header" ref={headerRef}>
    <Link className="app-header-brand" href={paths.home} aria-label="TradeFlow — на главную">
      <span className="app-header-logo" aria-hidden="true">TF</span>
      <span className="app-header-name">TradeFlow</span>
    </Link>
    <nav className="app-header-nav" aria-label="Разделы">
      {link({ value: 'home', label: 'Главная', href: paths.home })}
      {canEstimate(user) && link({ value: 'estimates', label: 'Сметы', href: paths.goods() })}
      {canEstimate(user) && link({ value: 'exportCheck', label: 'Сверка', href: paths.exportCheck })}
      {dirs.length > 0 && <details className={dirActive ? 'app-menu active' : 'app-menu'}>
        <summary>Справочники{chevron}</summary>
        <div className="app-menu-pop" onClick={closeMenu}>{dirs.map(link)}</div>
      </details>}
      {canTranslate(user) && link({ value: 'translator', label: 'Переводчик', href: paths.translator() })}
    </nav>
    {rates && rates.length > 0 && <RatesConverter rates={rates} />}
    <details className="app-menu app-header-user">
      <summary className="app-header-avatar" title={user.name || user.email} aria-label={`Профиль: ${user.name || user.email}`}>{initials(user)}</summary>
      <div className="app-menu-pop app-menu-right" onClick={closeMenu}>
        <div className="app-header-who">
          {user.name && <b>{user.name}</b>}
          <span>{user.email}</span>
          <small>{roleLabels[user.role] ?? ''}</small>
        </div>
        {isAdmin(user) && link({ value: 'employees', label: 'Сотрудники', href: paths.employees })}
        <button type="button" className="app-header-logout" onClick={onLogout}>Выйти</button>
      </div>
    </details>
  </header>
}
