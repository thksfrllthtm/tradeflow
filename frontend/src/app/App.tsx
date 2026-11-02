import { useEffect, useRef, useState } from 'react'
import { authApi } from '@/features/auth/authApi'
import { canEstimate, canTranslate, isAdmin, User } from '@/shared/types/user'
import { LoginPage } from '@/features/auth/LoginPage'
import { RegisterPage } from '@/features/auth/RegisterPage'
import { NomenclaturePage } from '@/features/directories/nomenclature/NomenclaturePage'
import { ProductDirectoryPage } from '@/features/directories/products/ProductDirectoryPage'
import { GoodsEstimatesPage } from '@/features/goods/GoodsEstimatesPage'
import { EmployeesPage } from '@/features/employees/EmployeesPage'
import { CeramicsPage } from '@/features/ceramics/CeramicsPage'
import { DecisionsPage } from '@/features/decisions/DecisionsPage'
import { HomePage } from '@/features/home/HomePage'
import { UtilFeePage } from '@/features/utilFee/UtilFeePage'
import { ExportCheckPage } from '@/features/exportCheck/ExportCheckPage'
import { TranslatorPage } from '@/features/translator/TranslatorPage'
import { TranslatorDocumentPage } from '@/features/translator/TranslatorDocumentPage'
import { navigate, parseRoute, paths, usePathname } from '@/shared/router/router'

const estimatePages: string[] = ['goods', 'products', 'decisions', 'ceramics']

export function App() {
  const [user, setUser] = useState<User | null>(null)
  const [checkingSession, setCheckingSession] = useState(true)
  const [sessionNotice, setSessionNotice] = useState('')
  const pathname = usePathname()
  const route = parseRoute(pathname)
  // Куда вернуться после входа, если страницу открыли без сессии.
  const returnTo = useRef<string | null>(null)
  const authPage = route?.page === 'login' || route?.page === 'register'
  // Сотрудники — только администратору, «Переводчик» — переводчику и администратору, сметы,
  // справочник товаров, база решений и керамика — не переводчику; остальных на главную.
  const knownRoute = route !== null && !(user !== null && (route.page === 'employees' && !isAdmin(user) || route.page === 'translator' && !canTranslate(user) || estimatePages.includes(route.page) && !canEstimate(user)))

  useEffect(() => {
    const onSessionExpired = () => {
      sessionStorage.removeItem('tradeflow.authenticated')
      setUser(null)
      setSessionNotice('Сессия завершена. Войдите снова')
    }
    window.addEventListener('tradeflow-session-expired', onSessionExpired)
    const hadSession = sessionStorage.getItem('tradeflow.authenticated') === '1'
    authApi.me().then((currentUser) => {
      setUser(currentUser)
      sessionStorage.setItem('tradeflow.authenticated', '1')
    }).catch((reason) => {
      setUser(null)
      const status = (reason as Error & { status?: number }).status
      if (status === 401 && hadSession) setSessionNotice('Сессия завершена. Войдите снова')
    }).finally(() => setCheckingSession(false))
    return () => window.removeEventListener('tradeflow-session-expired', onSessionExpired)
  }, [])

  // Перенаправления: неизвестный адрес → главная; без входа → /login; после входа — обратно.
  useEffect(() => {
    if (checkingSession) return
    if (!user && !authPage) {
      if (knownRoute) returnTo.current = pathname
      navigate(paths.login, { replace: true })
    } else if (user && (authPage || !knownRoute)) {
      navigate(returnTo.current ?? paths.home, { replace: true })
      returnTo.current = null
    }
  }, [checkingSession, user, authPage, knownRoute, pathname])

  function login(currentUser: User) {
    sessionStorage.setItem('tradeflow.authenticated', '1')
    setSessionNotice('')
    setUser(currentUser)
  }

  async function logout() {
    sessionStorage.removeItem('tradeflow.authenticated')
    try { await authApi.logout() } catch { /* сессия уже недействительна — всё равно выходим */ }
    setUser(null)
  }

  if (checkingSession) return <main className="centered"><p>Проверяем вход…</p></main>
  if (!user) return route?.page === 'register'
    ? <RegisterPage onLogin={login} onLoginMode={() => { setSessionNotice(''); navigate(paths.login) }} />
    : <LoginPage notice={sessionNotice} onLogin={login} onRegister={() => { setSessionNotice(''); navigate(paths.register) }} />
  if (!route || authPage || !knownRoute) return null

  switch (route.page) {
    case 'home': return <HomePage user={user} onLogout={() => void logout()} />
    case 'nomenclature': return <NomenclaturePage user={user} onLogout={() => void logout()} />
    case 'products': return <ProductDirectoryPage user={user} onLogout={() => void logout()} />
    case 'goods': return <GoodsEstimatesPage user={user} estimateId={route.estimateId} onLogout={() => void logout()} />
    case 'employees': return isAdmin(user) ? <EmployeesPage user={user} onLogout={() => void logout()} /> : null
    case 'decisions': return <DecisionsPage user={user} onLogout={() => void logout()} />
    case 'ceramics': return <CeramicsPage user={user} tab={route.tab} onLogout={() => void logout()} />
    case 'utilFee': return <UtilFeePage user={user} onLogout={() => void logout()} />
    case 'exportCheck': return <ExportCheckPage user={user} onLogout={() => void logout()} />
    case 'translator': return !canTranslate(user) ? null : route.documentId
      ? <TranslatorDocumentPage key={route.documentId} user={user} documentId={route.documentId} onLogout={() => void logout()} />
      : <TranslatorPage user={user} onLogout={() => void logout()} />
  }
}
