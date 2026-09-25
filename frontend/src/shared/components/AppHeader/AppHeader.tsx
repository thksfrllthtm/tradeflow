import { isAdmin, roleLabels, User } from '@/shared/types/user'
import { Link } from '@/shared/router/Link'
import { paths } from '@/shared/router/router'
import './AppHeader.css'

export type AppSection = 'estimates' | 'products' | 'tnved' | 'employees'

const sections: { value: AppSection; label: string; href: string }[] = [
  { value: 'estimates', label: 'Сметы', href: paths.goods() },
  { value: 'products', label: 'Справочник товаров', href: paths.products },
  { value: 'tnved', label: 'Справочник ТН ВЭД', href: paths.tnved },
]

const adminSections: typeof sections = [
  { value: 'employees', label: 'Сотрудники', href: paths.employees },
]

function initials(user: User) {
  const source = user.name?.trim() || user.email
  const parts = source.split(/[\s@._-]+/).filter(Boolean)
  return ((parts[0]?.[0] ?? '') + (parts[1]?.[0] ?? '')).toUpperCase() || '?'
}

// Общая шапка всех страниц: логотип, разделы, пользователь и выход.
export function AppHeader({ user, section, onLogout }: { user: User; section: AppSection; onLogout: () => void }) {
  return <header className="app-header">
    <Link className="app-header-brand" href={paths.home} aria-label="TradeFlow — на главную">
      <span className="app-header-logo" aria-hidden="true">TF</span>
      <span className="app-header-name">TradeFlow</span>
    </Link>
    <nav className="app-header-nav" aria-label="Разделы">
      {(isAdmin(user) ? [...sections, ...adminSections] : sections).map((item) => <Link key={item.value} href={item.href} className={item.value === section ? 'active' : undefined} aria-current={item.value === section ? 'page' : undefined}>{item.label}</Link>)}
    </nav>
    <div className="app-header-user">
      <span className="app-header-avatar" aria-hidden="true">{initials(user)}</span>
      <span className="app-header-who" title={user.email}>{user.name || user.email}<small>{roleLabels[user.role] ?? ''}</small></span>
      <button type="button" className="app-header-logout" onClick={onLogout}>Выйти</button>
    </div>
  </header>
}
