import { useEffect, useState } from 'react'

// Адреса страниц приложения. Обычные пути (без #): Vite отдаёт index.html на любой путь,
// поэтому прямые ссылки и обновление страницы работают.
//   /estimates/goods[/id]    — сметы товаров (главная)
//   /directory/products      — справочник товаров
//   /directory/tnved         — справочник ТН ВЭД
//   /employees               — сотрудники и роли (только администратор)
//   /ceramics[/base]         — подбор наименования и цены керамики (отдельно от смет)
//   /login, /register        — вход и регистрация
export type Route =
  | { page: 'goods'; estimateId: number | null }
  | { page: 'products' }
  | { page: 'tnved' }
  | { page: 'employees' }
  | { page: 'ceramics'; tab: 'match' | 'base' }
  | { page: 'login' }
  | { page: 'register' }

export const paths = {
  home: '/estimates/goods',
  goods: (id?: number | null) => id ? `/estimates/goods/${id}` : '/estimates/goods',
  products: '/directory/products',
  tnved: '/directory/tnved',
  employees: '/employees',
  ceramics: '/ceramics',
  ceramicsBase: '/ceramics/base',
  login: '/login',
  register: '/register',
}

// Старые адреса вида /#gloves-copy продолжают открываться — их заменяем на новые.
const legacyHashes: Record<string, string> = {
  '#gloves-copy': paths.goods(),
  '#directory': paths.tnved,
  '#products': paths.products,
}

// null — неизвестный адрес (приложение перенаправит на главную).
export function parseRoute(pathname: string): Route | null {
  const path = pathname.replace(/\/+$/, '') || '/'
  const estimate = /^\/estimates\/goods(?:\/(\d+))?$/.exec(path)
  if (estimate) return { page: 'goods', estimateId: estimate[1] ? Number(estimate[1]) : null }
  if (path === paths.products) return { page: 'products' }
  if (path === paths.tnved) return { page: 'tnved' }
  if (path === paths.employees) return { page: 'employees' }
  if (path === paths.ceramics) return { page: 'ceramics', tab: 'match' }
  if (path === paths.ceramicsBase) return { page: 'ceramics', tab: 'base' }
  if (path === paths.login) return { page: 'login' }
  if (path === paths.register) return { page: 'register' }
  return null
}

const navigateEvent = 'tradeflow-navigate'

export function navigate(path: string, options: { replace?: boolean } = {}) {
  if (path === window.location.pathname && !window.location.hash) return
  if (options.replace) window.history.replaceState(null, '', path)
  else window.history.pushState(null, '', path)
  window.dispatchEvent(new Event(navigateEvent))
}

function currentPath() {
  const legacy = legacyHashes[window.location.hash]
  if (legacy) window.history.replaceState(null, '', legacy)
  return window.location.pathname
}

// Текущий путь; меняется при navigate() и кнопках «назад/вперёд» браузера.
export function usePathname() {
  const [pathname, setPathname] = useState(currentPath)
  useEffect(() => {
    const sync = () => setPathname(currentPath())
    window.addEventListener('popstate', sync)
    window.addEventListener('hashchange', sync)
    window.addEventListener(navigateEvent, sync)
    return () => {
      window.removeEventListener('popstate', sync)
      window.removeEventListener('hashchange', sync)
      window.removeEventListener(navigateEvent, sync)
    }
  }, [])
  return pathname
}
