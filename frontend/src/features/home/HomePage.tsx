import { MouseEvent, ReactNode, useCallback, useEffect, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { errorMessage } from '@/shared/lib/errorMessage'
import { plural } from '@/shared/lib/plural'
import { Link } from '@/shared/router/Link'
import { navigate, paths } from '@/shared/router/router'
import { canEstimate, isAdmin, User } from '@/shared/types/user'
import { decisionsApi } from '@/features/decisions/decisionsApi'
import { goodsEstimatesApi } from '@/features/goods/goodsEstimatesApi'
import { estimateName, estimateSubtitle } from '@/shared/lib/estimateTitle'
import { GoodsEstimateSummary } from '@/shared/types/goods'
import { greeting, longDate, updatedText } from './homeText'
import { ReferenceChat } from './ReferenceChat/ReferenceChat'
import '@/shared/styles/list-page.css'
import './HomePage.css'

// Сколько последних смет в списке: он прокручивается внутри, полный список — «Все сметы».
const recentLimit = 30

const icon = { width: 20, height: 20, viewBox: '0 0 20 20', fill: 'none', stroke: 'currentColor', strokeWidth: 1.6, strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const, 'aria-hidden': true }
const PlusIcon = () => <svg {...icon}><path d="M10 4.5v11M4.5 10h11" /></svg>
const CodesIcon = () => <svg {...icon}><rect x="3.5" y="3" width="13" height="14" rx="2" /><path d="M7 7h6M7 10h6M7 13h3.5" /></svg>
const UtilFeeIcon = () => <svg {...icon}><rect x="4.5" y="3" width="11" height="14" rx="2" /><path d="M7.5 6.5h5M7.5 10h1M11.5 10h1M7.5 13.5h1M11.5 13.5h1" /></svg>
const CheckIcon = () => <svg {...icon}><path d="M4 10.5l4 4 8-9" /><path d="M3.5 4h5M3.5 7h3" /></svg>
const ArrowIcon = () => <svg {...icon} width={16} height={16}><path d="M7 5l5 5-5 5" /></svg>


// Главная: приветствие и курсы; справа (на узком окне — сверху) быстрые действия, что ждёт
// проверки и последние сметы; слева — чат подбора кода ТН ВЭД. В разметке действия и сметы идут
// первыми: с клавиатуры «Новая смета» — первая остановка после шапки.
export function HomePage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const admin = isAdmin(user)
  // Переводчику — номенклатура, утильсбор и справка: без смет.
  const estimator = canEstimate(user)
  // «Новая смета» сразу создаёт смету и открывает её (Ctrl-клик — как обычная ссылка на список смет);
  // не получилось создать — открывается список смет с кнопкой «Создать смету».
  function createEstimate(event: MouseEvent<HTMLAnchorElement>) {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
    event.preventDefault()
    goodsEstimatesApi.createToday().then((estimate) => navigate(paths.goods(estimate.id))).catch(() => navigate(paths.goods()))
  }
  const [estimates, setEstimates] = useState<GoodsEstimateSummary[] | null>(null)
  const [listError, setListError] = useState('')
  const [decisionCount, setDecisionCount] = useState(0)
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

  useEffect(() => estimator ? loadEstimates() : undefined, [estimator, loadEstimates])
  useEffect(() => {
    let active = true
    // Счётчики проверок — подсказки администратору: без них главная работает.
    if (admin) decisionsApi.stats().then((stats) => { if (active) setDecisionCount(stats.pending) }).catch(() => undefined)
    return () => { active = false }
  }, [admin])

  const recent = estimates?.slice(0, recentLimit) ?? []
  return <div className="app-shell home-shell">
    <AppHeader user={user} section="home" onLogout={onLogout} />
    <main className="home-scroll">
      <div className="home">
        <header className="home-hero">
          <div>
            <p className="home-date"><time>{longDate(now)}</time></p>
            <h1>{greeting(now.getHours())}{firstName ? `, ${firstName}` : ''}</h1>
          </div>
        </header>

        <div className="home-grid">
          <aside className="home-side" aria-label="Действия и последние сметы">
            <div className="home-actions">
              {estimator && <Link className="home-action primary" href={paths.goods()} onClick={createEstimate} title="Загрузите инвойс Excel и получите расчёт растаможки">
                <ActionBody icon={<PlusIcon />} title="Новая смета" text="Загрузите инвойс Excel и получите расчёт растаможки" />
              </Link>}
              <Link className="home-action" href={paths.nomenclature} title="Полная номенклатура ТН ВЭД ЕАЭС: поиск кода, ставки пошлины и НДС">
                <ActionBody icon={<CodesIcon />} title="Номенклатура ТН ВЭД" text="Поиск кода, ставки пошлины и НДС" />
              </Link>
              <Link className="home-action" href={paths.utilFee} title="Расчёт утильсбора за товар или упаковку">
                <ActionBody icon={<UtilFeeIcon />} title="Утильсбор" text="Расчёт утильсбора за товар или упаковку" />
              </Link>
              <Link className="home-action" href={paths.exportCheck} title="Сравнить инвойс с экспортной декларацией из Китая: итоги, реквизиты и каждая строка">
                <ActionBody icon={<CheckIcon />} title="Сверка экспортной" text="Инвойс против экспортной декларации КНР" />
              </Link>
            </div>

            {admin && decisionCount > 0 && <Link className="home-notice" href={`${paths.decisions}?status=pending`}>
              <span className="home-notice-dot" aria-hidden="true" />
              <span><b>{decisionCount} {plural(decisionCount, 'решение', 'решения', 'решений')} по кодам</b> {plural(decisionCount, 'ждёт', 'ждут', 'ждут')} вашей проверки в базе решений</span>
              <ArrowIcon />
            </Link>}

            {estimator && <section className="home-section" aria-labelledby="home-recent">
              <div className="home-section-head">
                <h2 id="home-recent">Последние сметы{estimates && estimates.length > 0 && <span className="dir-count home-count">{estimates.length}</span>}</h2>
                <Link href={paths.goods()}>Все сметы<ArrowIcon /></Link>
              </div>
              {listError ? <div className="home-state error" role="alert">
                <p><b>Сметы не загрузились.</b> {listError}</p>
                <button type="button" className="button-link" onClick={loadEstimates}>Повторить</button>
              </div>
                : estimates === null ? <div className="home-list" aria-busy="true" aria-label="Загружаем сметы">{[0, 1, 2].map((key) => <div key={key} className="home-row skeleton" />)}</div>
                : recent.length === 0 ? <div className="home-state">Смет пока нет. Нажмите «Новая смета» — она появится здесь.</div>
                : <div className="home-list">
                  {recent.map((estimate) =>
                    <Link key={estimate.id} className="home-row" href={paths.goods(estimate.id)} title={estimateName(estimate)}>
                      <strong>{estimateName(estimate)}</strong>
                      <span className={estimate.status === 'Рассчитано' ? 'status-dot ok home-status' : 'status-dot warn home-status'}>{estimate.status}</span>
                      <small>{[estimateSubtitle(estimate), estimate.ownerName].filter(Boolean).join(' · ')}</small>
                      <time dateTime={estimate.updatedAt}>{updatedText(estimate.updatedAt, now)}</time>
                    </Link>
                  )}
                </div>}
            </section>}
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
