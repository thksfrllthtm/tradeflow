import { ReactNode, useEffect, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatDisplayNumber } from '@/shared/lib/formatDisplayNumber'
import { plural } from '@/shared/lib/plural'
import { Link } from '@/shared/router/Link'
import { navigate, paths } from '@/shared/router/router'
import { isAdmin, User } from '@/shared/types/user'
import { tnvedDirectoryApi } from '@/features/directories/tnved/tnvedDirectoryApi'
import { decisionsApi } from '@/features/decisions/decisionsApi'
import { goodsEstimatesApi } from '@/features/goods/goodsEstimatesApi'
import { estimateTitle } from '@/shared/lib/estimateTitle'
import { formatRuDate, todayIso } from '@/shared/lib/dates'
import { GoodsEstimateSummary } from '@/shared/types/goods'
import { greeting, longDate, updatedText } from './homeText'
import './HomePage.css'

const recentLimit = 6

const icon = { width: 20, height: 20, viewBox: '0 0 20 20', fill: 'none', stroke: 'currentColor', strokeWidth: 1.6, strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const, 'aria-hidden': true }
const PlusIcon = () => <svg {...icon}><path d="M10 4.5v11M4.5 10h11" /></svg>
const CodesIcon = () => <svg {...icon}><rect x="3.5" y="3" width="13" height="14" rx="2" /><path d="M7 7h6M7 10h6M7 13h3.5" /></svg>
const CeramicsIcon = () => <svg {...icon}><path d="M4 8.5h12l-1.3 6.2a2 2 0 0 1-2 1.6H7.3a2 2 0 0 1-2-1.6z" /><path d="M7.5 8.5a2.5 2.5 0 0 1 5 0" /></svg>
const ArrowIcon = () => <svg {...icon} width={16} height={16}><path d="M7 5l5 5-5 5" /></svg>

// Главная: приветствие, три быстрых действия, что ждёт проверки и последние сметы.
export function HomePage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const admin = isAdmin(user)
  const [estimates, setEstimates] = useState<GoodsEstimateSummary[] | null>(null)
  const [reviewCount, setReviewCount] = useState(0)
  const [decisionCount, setDecisionCount] = useState(0)
  const [rates, setRates] = useState<{ currency: string; rate: string; date: string }[]>([])
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const now = new Date()
  const firstName = user.name?.trim().split(/\s+/)[0]

  useEffect(() => {
    let active = true
    goodsEstimatesApi.list()
      .then((items) => { if (active) setEstimates([...items].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))) })
      .catch((reason) => { if (active) { setEstimates([]); setError(errorMessage(reason)) } })
    // Курс и счётчик кодов — дополнительные подсказки: без них главная работает.
    // Курсы USD и EUR — справка; не пришёл один, второй всё равно показываем.
    Promise.allSettled([goodsEstimatesApi.usdRate(todayIso()), goodsEstimatesApi.eurRate(todayIso())])
      // Курс показывается, только если сервер вернул именно запрошенную валюту: старый сервер
      // не знает ?currency=EUR и отдаёт доллар — второй «1 USD» не нужен.
      .then((results) => { if (active) setRates(results.flatMap((result, index) => result.status === 'fulfilled' && result.value.currency === ['USD', 'EUR'][index] ? [result.value] : [])) })
    if (admin) tnvedDirectoryApi.list({ status: 'needs_review' }).then((items) => { if (active) setReviewCount(items.length) }).catch(() => undefined)
    if (admin) decisionsApi.stats().then((stats) => { if (active) setDecisionCount(stats.pending) }).catch(() => undefined)
    return () => { active = false }
  }, [admin])

  async function create() {
    setCreating(true); setError('')
    try {
      const estimate = await goodsEstimatesApi.create({ name: `Смета товаров · ${todayIso()}`, client: '', assessmentDate: todayIso() })
      navigate(paths.goods(estimate.id))
    } catch (reason) {
      setError(errorMessage(reason))
      setCreating(false)
    }
  }

  const recent = estimates?.slice(0, recentLimit) ?? []
  return <main className="app-shell home-shell">
    <AppHeader user={user} section="home" onLogout={onLogout} />
    <div className="home-scroll">
      <div className="home">
        <header className="home-hero">
          <div>
            <p className="home-date">{longDate(now)}</p>
            <h1>{greeting(now.getHours())}{firstName ? `, ${firstName}` : ''}</h1>
          </div>
          {rates.length > 0 && <div className="home-rates" title="Официальный курс Национального банка РК">
            {rates.map((rate) => <div className="home-rate" key={rate.currency}>
              <span>1 {rate.currency}</span>
              <strong>{formatDisplayNumber(rate.rate)} ₸</strong>
              <small>НБРК · {formatRuDate(rate.date)}</small>
            </div>)}
          </div>}
        </header>

        {error && <p className="home-error" role="alert">{error}</p>}

        <div className="home-actions">
          <button type="button" className="home-action primary" onClick={() => void create()} disabled={creating}>
            <ActionBody icon={<PlusIcon />} title={creating ? 'Создаём…' : 'Новая смета'} text="Загрузите инвойс в Excel и получите расчёт растаможки" />
          </button>
          <Link className="home-action" href={`${paths.tnved}?check=1`}>
            <ActionBody icon={<CodesIcon />} title="Проверить коды ТН ВЭД" text="Вставьте колонку из Excel — покажем, каких кодов нет" />
          </Link>
          <Link className="home-action" href={paths.ceramics}>
            <ActionBody icon={<CeramicsIcon />} title="Подобрать керамику" text="Наименование и цена по марке и фото" />
          </Link>
        </div>

        {admin && reviewCount > 0 && <Link className="home-notice" href={`${paths.tnved}?status=needs_review`}>
          <span className="home-notice-dot" aria-hidden="true" />
          <span><b>{reviewCount} {plural(reviewCount, 'код', 'кода', 'кодов')} ТН ВЭД</b> {plural(reviewCount, 'ждёт', 'ждут', 'ждут')} вашей проверки</span>
          <ArrowIcon />
        </Link>}

        {admin && decisionCount > 0 && <Link className="home-notice" href={`${paths.decisions}?status=pending`}>
          <span className="home-notice-dot" aria-hidden="true" />
          <span><b>{decisionCount} {plural(decisionCount, 'решение', 'решения', 'решений')} по кодам</b> {plural(decisionCount, 'ждёт', 'ждут', 'ждут')} вашей проверки в базе решений</span>
          <ArrowIcon />
        </Link>}

        <section className="home-section" aria-labelledby="home-recent">
          <div className="home-section-head">
            <h2 id="home-recent">Последние сметы</h2>
            {estimates && estimates.length > 0 && <Link href={paths.goods()}>Все сметы<ArrowIcon /></Link>}
          </div>
          {estimates === null ? <div className="home-list" aria-busy="true">{[0, 1, 2].map((key) => <div key={key} className="home-row skeleton" />)}</div>
            : recent.length === 0 ? <div className="home-empty">Смет пока нет. Начните с новой — она появится здесь.</div>
            : <div className="home-list">
              {recent.map((estimate) => <Link key={estimate.id} className="home-row" href={paths.goods(estimate.id)}>
                <span className="home-row-main">
                  <strong>{estimateTitle(estimate.name)}</strong>
                  <small>{estimate.client || 'Без клиента'}{estimate.ownerName ? ` · ${estimate.ownerName}` : ''}</small>
                </span>
                <span className={estimate.status === 'Рассчитано' ? 'home-status done' : 'home-status'}>{estimate.status}</span>
                <span className="home-row-date">{updatedText(estimate.updatedAt, now)}</span>
              </Link>)}
            </div>}
        </section>
      </div>
    </div>
  </main>
}

function ActionBody({ icon, title, text }: { icon: ReactNode; title: string; text: string }) {
  return <>
    <span className="home-action-icon">{icon}</span>
    <span className="home-action-title">{title}</span>
    <span className="home-action-text">{text}</span>
  </>
}
