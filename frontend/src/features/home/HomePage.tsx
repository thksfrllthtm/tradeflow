import { ReactNode, useCallback, useEffect, useRef, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
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
import { estimateLabel, greeting, longDate, updatedText } from './homeText'
import { ReferenceChat } from './ReferenceChat/ReferenceChat'
import './HomePage.css'

// Сколько последних смет в списке: он прокручивается внутри, полный список — «Все сметы».
const recentLimit = 30

const icon = { width: 20, height: 20, viewBox: '0 0 20 20', fill: 'none', stroke: 'currentColor', strokeWidth: 1.6, strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const, 'aria-hidden': true }
const PlusIcon = () => <svg {...icon}><path d="M10 4.5v11M4.5 10h11" /></svg>
const CodesIcon = () => <svg {...icon}><rect x="3.5" y="3" width="13" height="14" rx="2" /><path d="M7 7h6M7 10h6M7 13h3.5" /></svg>
const CeramicsIcon = () => <svg {...icon}><path d="M4 8.5h12l-1.3 6.2a2 2 0 0 1-2 1.6H7.3a2 2 0 0 1-2-1.6z" /><path d="M7.5 8.5a2.5 2.5 0 0 1 5 0" /></svg>
const ArrowIcon = () => <svg {...icon} width={16} height={16}><path d="M7 5l5 5-5 5" /></svg>

type Rate = { currency: string; rate: string; date: string }

// Главная: приветствие и курсы; справа (на узком окне — сверху) быстрые действия, что ждёт
// проверки и последние сметы; слева — чат подбора кода ТН ВЭД. В разметке действия и сметы идут
// первыми: с клавиатуры «Новая смета» — первая остановка после шапки.
export function HomePage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const admin = isAdmin(user)
  const [estimates, setEstimates] = useState<GoodsEstimateSummary[] | null>(null)
  const [listError, setListError] = useState('')
  const [reviewCount, setReviewCount] = useState(0)
  const [decisionCount, setDecisionCount] = useState(0)
  const [rates, setRates] = useState<Rate[] | null>(null)
  const [creating, setCreating] = useState(false)
  const creatingRef = useRef(false)
  const [createError, setCreateError] = useState('')
  const now = new Date()
  const firstName = user.name?.trim().split(/\s+/)[0]

  // Список смет: загрузка, ошибка с «Повторить» и пустой список — разные состояния.
  const loadEstimates = useCallback(() => {
    let active = true
    setEstimates(null); setListError('')
    goodsEstimatesApi.list()
      .then((items) => { if (active) setEstimates([...items].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))) })
      .catch((reason) => { if (active) setListError(errorMessage(reason)) })
    return () => { active = false }
  }, [])

  // Курсы USD и EUR — справка; не пришёл один, второй всё равно показываем. Курс показывается,
  // только если сервер вернул именно запрошенную валюту: старый сервер не знает ?currency=EUR.
  const loadRates = useCallback(() => {
    let active = true
    setRates(null)
    Promise.allSettled([goodsEstimatesApi.usdRate(todayIso()), goodsEstimatesApi.eurRate(todayIso())])
      .then((results) => { if (active) setRates(results.flatMap((result, index) => result.status === 'fulfilled' && result.value.currency === ['USD', 'EUR'][index] ? [result.value] : [])) })
    return () => { active = false }
  }, [])

  useEffect(() => loadEstimates(), [loadEstimates])
  useEffect(() => loadRates(), [loadRates])
  useEffect(() => {
    let active = true
    // Счётчики проверок — подсказки администратору: без них главная работает.
    if (admin) tnvedDirectoryApi.list({ status: 'needs_review' }).then((items) => { if (active) setReviewCount(items.length) }).catch(() => undefined)
    if (admin) decisionsApi.stats().then((stats) => { if (active) setDecisionCount(stats.pending) }).catch(() => undefined)
    return () => { active = false }
  }, [admin])

  // Повторное нажатие, пока запрос идёт, второй сметы не создаёт.
  async function create() {
    if (creatingRef.current) return
    creatingRef.current = true
    setCreating(true); setCreateError('')
    try {
      const estimate = await goodsEstimatesApi.create({ name: `Смета товаров · ${todayIso()}`, client: '', assessmentDate: todayIso() })
      // ?new=1 — на странице сметы сразу откроется поле названия и клиента.
      navigate(`${paths.goods(estimate.id)}?new=1`)
    } catch (reason) {
      setCreateError(errorMessage(reason))
      creatingRef.current = false
      setCreating(false)
    }
  }

  const recent = estimates?.slice(0, recentLimit) ?? []
  return <div className="app-shell home-shell">
    <AppHeader user={user} section="home" onLogout={onLogout} />
    <main className="home-scroll">
      <div className="home">
        <header className="home-hero">
          <div>
            <p className="home-date">{longDate(now)}</p>
            <h1>{greeting(now.getHours())}{firstName ? `, ${firstName}` : ''}</h1>
          </div>
          {rates === null ? <div className="home-rates" aria-busy="true"><span className="home-rates-note">Курс НБРК…</span></div>
            : rates.length === 0 ? <div className="home-rates"><span className="home-rates-note">Курс НБРК недоступен</span><button type="button" className="home-link-button" onClick={loadRates}>Повторить</button></div>
            : <div className="home-rates" title="Официальный курс Национального банка РК">
              {rates.map((rate) => <div className="home-rate" key={rate.currency}>
                <span>1 {rate.currency}</span>
                <strong>{formatRuNumber(rate.rate)} ₸</strong>
                <small>НБРК · {formatRuDate(rate.date)}</small>
              </div>)}
            </div>}
        </header>

        <div className="home-grid">
          <aside className="home-side" aria-label="Действия и последние сметы">
            <div className="home-actions">
              <button type="button" className="home-action primary" onClick={() => void create()} disabled={creating} aria-busy={creating} title="Создаст черновик сметы: загрузите инвойс Excel и получите расчёт растаможки">
                <ActionBody icon={<PlusIcon />} title={creating ? 'Создаём черновик…' : 'Новая смета'} text="Создаст черновик сметы: загрузите инвойс Excel и получите расчёт растаможки" />
              </button>
              <Link className="home-action" href={`${paths.tnved}?check=1`} title="Вставьте колонку из Excel — покажем, каких кодов нет">
                <ActionBody icon={<CodesIcon />} title="Проверить коды ТН ВЭД" text="Вставьте колонку из Excel — покажем, каких кодов нет" />
              </Link>
              <Link className="home-action" href={paths.ceramics} title="Наименование и цена по марке и фото">
                <ActionBody icon={<CeramicsIcon />} title="Подобрать керамику" text="Наименование и цена по марке и фото" />
              </Link>
            </div>
            {createError && <p className="home-error" role="alert">Смета не создана: {createError}</p>}

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
                <h2 id="home-recent">Последние сметы{estimates && estimates.length > 0 && <span className="home-count"> · {estimates.length}</span>}</h2>
                <Link href={paths.goods()}>Все сметы<ArrowIcon /></Link>
              </div>
              {listError ? <div className="home-state error" role="alert">
                <p><b>Сметы не загрузились.</b> {listError}</p>
                <button type="button" className="home-retry" onClick={loadEstimates}>Повторить</button>
              </div>
                : estimates === null ? <div className="home-list" aria-busy="true" aria-label="Загружаем сметы">{[0, 1, 2].map((key) => <div key={key} className="home-row skeleton" />)}</div>
                : recent.length === 0 ? <div className="home-state">Смет пока нет. Нажмите «Новая смета» — она появится здесь.</div>
                : <div className="home-list">
                  {recent.map((estimate) => {
                    const label = estimateLabel(estimate, estimateTitle(estimate.name))
                    return <Link key={estimate.id} className="home-row" href={paths.goods(estimate.id)} title={estimateTitle(estimate.name)}>
                      <strong className={label.untitled ? 'untitled' : undefined}>{label.title}</strong>
                      <span className={estimate.status === 'Рассчитано' ? 'home-status done' : 'home-status'}>{estimate.status}</span>
                      <small>{label.detail}{estimate.ownerName ? ` · ${estimate.ownerName}` : ''}</small>
                      <time dateTime={estimate.updatedAt}>{updatedText(estimate.updatedAt, now)}</time>
                    </Link>
                  })}
                </div>}
            </section>
          </aside>
          <div className="home-chat"><ReferenceChat /></div>
        </div>
      </div>
    </main>
  </div>
}

function ActionBody({ icon, title, text }: { icon: ReactNode; title: string; text: string }) {
  return <>
    <span className="home-action-icon">{icon}</span>
    <span className="home-action-title">{title}</span>
    <span className="home-action-text">{text}</span>
  </>
}
