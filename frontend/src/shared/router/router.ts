import { useEffect, useState } from 'react'

// Адреса страниц приложения. Обычные пути (без #): Vite отдаёт index.html на любой путь,
// поэтому прямые ссылки и обновление страницы работают.
//   /                        — главная: быстрые действия и последние сметы
//   /estimates/goods[/id]    — сметы товаров (без номера — начальный экран новой сметы)
//   /directory/products      — справочник товаров
//   /directory/nomenclature  — полная номенклатура ТН ВЭД ЕАЭС (выгрузка TWS.BY)
//   /directory/trademarks    — ТРОИС: таможенный реестр объектов интеллектуальной собственности КГД
//   /decisions               — база решений «товар из инвойса → код ТН ВЭД»
//   /employees               — сотрудники и роли (только администратор)
//   /agent-log               — журнал агента ТН ВЭД: подборы всех сотрудников и теневая модель (только администратор)
//   /user-actions            — журнал действий сотрудников: изменения, вход и выход (только администратор)
//   /documents-base         — база документов: ответы «Документы для ввоза» по кодам, справки и акты (только администратор)
//   /ceramics[/base]         — подбор наименования и цены керамики (отдельно от смет)
//   /util-fee                — калькулятор утильсбора
//   /export-check            — сверка инвойса с экспортной декларацией КНР
//   /translator[/id]         — перевод Word-документов: список или документ
//   /login, /register        — вход и регистрация
type Route =
  | { page: 'home' }
  | { page: 'goods'; estimateId: number | null }
  | { page: 'products' }
  | { page: 'nomenclature' }
  | { page: 'trademarks' }
  | { page: 'decisions' }
  | { page: 'employees' }
  | { page: 'agentLog' }
  | { page: 'userActions' }
  | { page: 'documentsBase' }
  | { page: 'ceramics'; tab: 'match' | 'base' }
  | { page: 'utilFee' }
  | { page: 'exportCheck' }
  | { page: 'translator'; documentId: number | null }
  | { page: 'login' }
  | { page: 'register' }

export const paths = {
  home: '/',
  goods: (id?: number | null) => id ? `/estimates/goods/${id}` : '/estimates/goods',
  products: '/directory/products',
  nomenclature: '/directory/nomenclature',
  trademarks: '/directory/trademarks',
  decisions: '/decisions',
  employees: '/employees',
  agentLog: '/agent-log',
  userActions: '/user-actions',
  documentsBase: '/documents-base',
  ceramics: '/ceramics',
  ceramicsBase: '/ceramics/base',
  utilFee: '/util-fee',
  exportCheck: '/export-check',
  translator: (id?: number | null) => id ? `/translator/${id}` : '/translator',
  login: '/login',
  register: '/register',
}

// null — неизвестный адрес (приложение перенаправит на главную).
export function parseRoute(pathname: string): Route | null {
  const path = pathname.replace(/\/+$/, '') || '/'
  if (path === paths.home) return { page: 'home' }
  const estimate = /^\/estimates\/goods(?:\/(\d+))?$/.exec(path)
  if (estimate) return { page: 'goods', estimateId: estimate[1] ? Number(estimate[1]) : null }
  if (path === paths.products) return { page: 'products' }
  if (path === paths.nomenclature) return { page: 'nomenclature' }
  if (path === paths.trademarks) return { page: 'trademarks' }
  if (path === paths.decisions) return { page: 'decisions' }
  if (path === paths.employees) return { page: 'employees' }
  if (path === paths.agentLog) return { page: 'agentLog' }
  if (path === paths.userActions) return { page: 'userActions' }
  if (path === paths.documentsBase) return { page: 'documentsBase' }
  if (path === paths.ceramics) return { page: 'ceramics', tab: 'match' }
  if (path === paths.ceramicsBase) return { page: 'ceramics', tab: 'base' }
  if (path === paths.utilFee) return { page: 'utilFee' }
  if (path === paths.exportCheck) return { page: 'exportCheck' }
  const translatorDocument = /^\/translator(?:\/(\d+))?$/.exec(path)
  if (translatorDocument) return { page: 'translator', documentId: translatorDocument[1] ? Number(translatorDocument[1]) : null }
  if (path === paths.login) return { page: 'login' }
  if (path === paths.register) return { page: 'register' }
  return null
}

const navigateEvent = 'tradeflow-navigate'

export function navigate(path: string, options: { replace?: boolean } = {}) {
  if (path === window.location.pathname) return
  if (options.replace) window.history.replaceState(null, '', path)
  else window.history.pushState(null, '', path)
  window.dispatchEvent(new Event(navigateEvent))
}

const currentPath = () => window.location.pathname

// Текущий путь; меняется при navigate() и кнопках «назад/вперёд» браузера.
export function usePathname() {
  const [pathname, setPathname] = useState(currentPath)
  useEffect(() => {
    const sync = () => setPathname(currentPath())
    window.addEventListener('popstate', sync)
    window.addEventListener(navigateEvent, sync)
    return () => {
      window.removeEventListener('popstate', sync)
      window.removeEventListener(navigateEvent, sync)
    }
  }, [])
  return pathname
}
