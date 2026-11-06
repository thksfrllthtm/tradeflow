import { MouseEvent, useEffect, useRef } from 'react'
import { canEstimate, canTranslate, isAdmin, roleLabels, User } from '@/shared/types/user'
import { Link } from '@/shared/router/Link'
import { paths } from '@/shared/router/router'
import { useTodayRates } from '@/shared/api/todayRates'
import { RatesConverter } from './RatesConverter'
import './AppHeader.css'

// home — главная, на неё ведут логотип и пункт «Главная».
type AppSection = 'home' | 'estimates' | 'products' | 'nomenclature' | 'trademarks' | 'decisions' | 'ceramics' | 'utilFee' | 'exportCheck' | 'employees' | 'agentLog' | 'userActions' | 'documentsBase' | 'dataUpdates' | 'translator'
type Item = { value: AppSection; label: string; href: string; hint?: string }
type Group = { label: string; items: Item[] }

// Разделы меню по смыслу: всё для точного кода ТН ВЭД, разовые калькуляторы, администрирование.
const groups: Group[] = [
  { label: 'Коды ТН ВЭД', items: [
    { value: 'nomenclature', label: 'Номенклатура', href: paths.nomenclature, hint: 'Номенклатура ТН ВЭД ЕАЭС: ставки, перечень РК, антидемпинг' },
    { value: 'decisions', label: 'База решений', href: paths.decisions, hint: 'Проверенные решения «товар → код ТН ВЭД»' },
    { value: 'products', label: 'Справочник товаров', href: paths.products, hint: 'Товары компании с кодами и наименованиями' },
    { value: 'trademarks', label: 'ТРОИС', href: paths.trademarks, hint: 'Таможенный реестр объектов интеллектуальной собственности (товарные знаки)' },
  ] },
  { label: 'Инструменты', items: [
    { value: 'utilFee', label: 'Утильсбор', href: paths.utilFee, hint: 'Калькулятор утилизационного сбора' },
    { value: 'exportCheck', label: 'Сверка с экспортной декларацией', href: paths.exportCheck, hint: 'Сверка инвойса с экспортной декларацией КНР' },
    { value: 'ceramics', label: 'Керамика', href: paths.ceramics, hint: 'Наименование и цена керамики по марке и фото' },
    { value: 'translator', label: 'Переводчик', href: paths.translator(), hint: 'Перевод документов Word' },
  ] },
  { label: 'Администрирование', items: [
    { value: 'employees', label: 'Сотрудники', href: paths.employees, hint: 'Учётные записи, роли и сметы сотрудников' },
    { value: 'agentLog', label: 'Журнал агента', href: paths.agentLog, hint: 'Подборы кодов ИИ-агентом у всех сотрудников' },
    { value: 'userActions', label: 'Журнал действий', href: paths.userActions, hint: 'Изменения, вход и выход сотрудников' },
    { value: 'documentsBase', label: 'База документов', href: paths.documentsBase, hint: 'Ответы «Документы для ввоза» по кодам и акты' },
    { value: 'dataUpdates', label: 'Обновление данных', href: paths.dataUpdates, hint: 'Номенклатура, перечень РК, антидемпинг, СУР, ТРОИС: даты, источники, загрузка' },
  ] },
]

// Разделы, закрытые переводчику; разделы только для администратора.
const estimateSections: AppSection[] = ['estimates', 'products', 'decisions', 'ceramics']
const adminSections: AppSection[] = ['employees', 'agentLog', 'userActions', 'documentsBase', 'dataUpdates']

function initials(user: User) {
  const source = user.name?.trim() || user.email
  const parts = source.split(/[\s@._-]+/).filter(Boolean)
  return ((parts[0]?.[0] ?? '') + (parts[1]?.[0] ?? '')).toUpperCase() || '?'
}

const chevron = <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><path d="M2.5 4l2.5 2.5L7.5 4" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" /></svg>

// Общая шапка всех страниц: логотип, «Главная», «Сметы», группы разделов (Коды ТН ВЭД, Инструменты, Администрирование),
// курс НБРК с конвертером валют и меню профиля (кто вошёл, Выйти). На узком экране меню переносится на вторую строку.
export function AppHeader({ user, section, onLogout }: { user: User; section: AppSection; onLogout: () => void }) {
  // Переводчику «Переводчик» — основная работа: ссылка в верхней строке, а не внутри «Инструментов».
  const translatorOnTop = user.role === 'translator'
  const allowed = (item: Item) => (canEstimate(user) || !estimateSections.includes(item.value)) && (item.value !== 'translator' || (canTranslate(user) && !translatorOnTop)) && (!adminSections.includes(item.value) || isAdmin(user))
  const menus = groups.map((group) => ({ ...group, items: group.items.filter(allowed) })).filter((group) => group.items.length > 0)
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
  const link = (item: Item) => <Link key={item.value} href={item.href} title={item.hint} className={item.value === section ? 'active' : undefined} aria-current={item.value === section ? 'page' : undefined}>{item.label}</Link>

  return <header className="app-header" ref={headerRef}>
    <Link className="app-header-brand" href={paths.home} aria-label="TradeFlow — на главную">
      <span className="app-header-logo" aria-hidden="true">TF</span>
      <span className="app-header-name">TradeFlow</span>
    </Link>
    <nav className="app-header-nav" aria-label="Разделы">
      {link({ value: 'home', label: 'Главная', href: paths.home })}
      {canEstimate(user) && link({ value: 'estimates', label: 'Сметы', href: paths.goods() })}
      {translatorOnTop && link({ value: 'translator', label: 'Переводчик', href: paths.translator() })}
      {menus.map((menu) => {
        // Открыта страница из группы — группа подсвечена, а рядом с её названием видно, какая именно страница.
        const current = menu.items.find((item) => item.value === section)
        return <details key={menu.label} className={current ? 'app-menu active' : 'app-menu'}>
          <summary>{menu.label}{current && <span className="app-menu-current"> · {current.label}</span>}{chevron}</summary>
          <div className="app-menu-pop" onClick={closeMenu}>{menu.items.map(link)}</div>
        </details>
      })}
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
        <button type="button" className="app-header-logout" onClick={onLogout}>Выйти</button>
      </div>
    </details>
  </header>
}
