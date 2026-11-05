import { useEffect, useMemo, useState } from 'react'
import { goodsEstimatesApi } from './goodsEstimatesApi'
import { GoodsEstimateInput, GoodsEstimateSummary } from '@/shared/types/goods'
import { isAdmin, User } from '@/shared/types/user'
import { Link } from '@/shared/router/Link'
import { GoodsCalculator } from './calculator/GoodsCalculator'
import { estimateName, estimateSubtitle, estimateTitle, hasOwnName } from '@/shared/lib/estimateTitle'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatRuDate, todayIso } from '@/shared/lib/dates'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { navigate, paths } from '@/shared/router/router'
import { EstimatesSidebar } from './components/EstimatesSidebar/EstimatesSidebar'
import './GoodsEstimatesPage.css'

// estimateId — номер сметы из адреса /estimates/goods/:id (null — адрес без номера: начальный
// экран, смета создаётся кнопкой на нём).
type Props = { user: User; estimateId: number | null; onLogout: () => void }

export function GoodsEstimatesPage({ user, estimateId, onLogout }: Props) {
  const fresh = estimateId === null
  const [estimates, setEstimates] = useState<GoodsEstimateSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [savingMeta, setSavingMeta] = useState(false)
  const [error, setError] = useState('')
  // Смета сотрудника, открытая администратором по ссылке со страницы «Сотрудники»: в его
  // списке её нет, но открыть, изменить и пересчитать её он может.
  const [foreign, setForeign] = useState<GoodsEstimateSummary | null>(null)
  const [foreignFailed, setForeignFailed] = useState<number | null>(null)
  const selected = useMemo(() => estimates.find((estimate) => estimate.id === estimateId) ?? (foreign?.id === estimateId ? foreign : null), [estimates, estimateId, foreign])
  const [meta, setMeta] = useState<GoodsEstimateInput>({ name: '', client: '', assessmentDate: todayIso() })

  useEffect(() => {
    let active = true
    void goodsEstimatesApi.list().then((items) => {
      if (!active) return
      setEstimates(items)
    }).catch((reason) => {
      if (active) setError(errorMessage(reason))
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
  }, [])

  // Номер удалённой/недоступной сметы — на начальный экран. Администратор сначала пробует
  // открыть смету сотрудника по номеру.
  useEffect(() => {
    if (loading || selected || estimateId === null) return
    if (isAdmin(user) && foreignFailed !== estimateId) {
      let active = true
      goodsEstimatesApi.get(estimateId)
        .then((estimate) => { if (active) setForeign({ id: estimate.id, name: estimate.name, client: estimate.client, assessmentDate: estimate.assessmentDate, status: estimate.status, createdAt: estimate.createdAt, updatedAt: estimate.updatedAt, ownerId: estimate.ownerId, ownerName: estimate.ownerName }) })
        .catch(() => { if (active) setForeignFailed(estimateId) })
      return () => { active = false }
    }
    navigate(paths.goods(), { replace: true })
  }, [loading, selected, estimateId, user, foreignFailed])
  const employeeEstimate = selected !== null && selected === foreign && foreign.ownerId !== undefined && foreign.ownerId !== user.id

  useEffect(() => {
    if (!selected) return
    // Стандартное название в поле не показываем: пустое поле — смета называется по клиенту или номеру.
    setMeta({ name: hasOwnName(selected.name) ? estimateTitle(selected.name) : '', client: selected.client, assessmentDate: selected.assessmentDate })
  }, [selected])

  async function create() {
    setError('')
    try {
      const estimate = await goodsEstimatesApi.createToday()
      // Сервер мог взять заново пустую смету и удалить другие пустые — список перечитываем.
      setEstimates(await goodsEstimatesApi.list().catch(() => [estimate, ...estimates.filter((item) => item.id !== estimate.id)]))
      navigate(paths.goods(estimate.id), { replace: fresh })
    } catch (reason) {
      setError(errorMessage(reason))
    }
  }

  async function saveMeta() {
    if (!selected || savingMeta) return false
    setSavingMeta(true); setError('')
    try {
      const normalizedMeta = { ...meta, name: meta.name.trim() ? estimateTitle(meta.name) : 'Смета товаров' }
      const saved = await goodsEstimatesApi.update(selected.id, normalizedMeta)
      setEstimates((current) => current.map((item) => item.id === saved.id ? saved : item))
      setForeign((current) => current?.id === saved.id ? { ...current, ...saved, ownerId: current.ownerId, ownerName: current.ownerName } : current)
      return true
    } catch (reason) {
      setError(errorMessage(reason))
      return false
    } finally {
      setSavingMeta(false)
    }
  }

  async function remove(estimate: GoodsEstimateSummary) {
    if (!window.confirm(`Удалить смету «${estimateName(estimate)}»? Все строки и результат будут удалены.`)) return
    try {
      await goodsEstimatesApi.remove(estimate.id)
      const remaining = estimates.filter((item) => item.id !== estimate.id)
      setEstimates(remaining)
      if (estimateId === estimate.id) navigate(paths.goods(remaining[0]?.id), { replace: true })
    } catch (reason) {
      setError(errorMessage(reason))
    }
  }

  function onEstimateSaved(saved: GoodsEstimateSummary) {
    setForeign((current) => current?.id === saved.id ? { ...current, name: saved.name, client: saved.client, assessmentDate: saved.assessmentDate, status: saved.status, updatedAt: saved.updatedAt } : current)
    setEstimates((current) => current.map((item) => item.id === saved.id ? {
      id: saved.id,
      name: saved.name,
      client: saved.client,
      assessmentDate: saved.assessmentDate,
      status: saved.status,
      createdAt: saved.createdAt,
      updatedAt: saved.updatedAt,
    } : item))
  }

  return <main className="app-shell goods-estimates-app">
    <AppHeader user={user} section="estimates" onLogout={onLogout} />
    {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}
    <div className="estimates-workspace">
      <EstimatesSidebar title="Сметы товаров" estimates={estimates} selectedId={selected?.id ?? null} docked={!selected && !loading && estimates.length === 0} loading={loading} nameOf={estimateName} hrefOf={paths.goods} onCreate={() => void create()} onRemove={(estimate) => void remove(estimate)} />
      <section className="estimates-content">
        {selected ? <GoodsCalculator
          key={selected.id}
          estimateId={selected.id}
          onEstimateSaved={onEstimateSaved}
          exportName={[estimateTitle(selected.name), selected.client].filter(Boolean).join(' — ')}
          headerSlot={<div className="estimate-bar-meta">
            <h1 title={estimateName(selected)}>{estimateName(selected)}</h1>
            {/* Дата уже в названии — под ним не повторяется. */}
            <span className="estimate-bar-sub">{estimateName(selected).includes(formatRuDate(selected.assessmentDate)) ? '' : `${formatRuDate(selected.assessmentDate)} · `}{selected.client.trim() ? estimateSubtitle(selected) : 'Клиент не указан'} ·</span>
            {employeeEstimate && <span className="badge warn estimate-owner" title="Изменения и расчёт сохраняются в смете сотрудника">Смета сотрудника: {foreign?.ownerName || 'сотрудник'} · <Link href={paths.employees}>к сотрудникам</Link></span>}
            <details className="estimate-bar-editor"><summary>{selected.client ? 'Изменить' : 'Указать'}</summary><div className="estimate-bar-fields"><label>Название<input value={meta.name} onChange={(event) => setMeta((current) => ({ ...current, name: event.target.value }))} maxLength={200} placeholder="По клиенту или номеру" /></label><label>Клиент<input value={meta.client} onChange={(event) => setMeta((current) => ({ ...current, client: event.target.value }))} maxLength={200} placeholder="Необязательно" /></label><label>Дата<input type="date" value={meta.assessmentDate} onChange={(event) => setMeta((current) => ({ ...current, assessmentDate: event.target.value }))} /></label><button type="button" className="secondary" onClick={(event) => { const details = event.currentTarget.closest('details'); void saveMeta().then((saved) => { if (saved) details?.removeAttribute('open') }) }} disabled={savingMeta}>{savingMeta ? 'Сохраняем…' : 'Сохранить'}</button></div></details>
          </div>}
        /> : !loading && fresh && <div className="estimates-empty">
          {/* Как карточки этапов сметы: заголовок, три шага (как в шапке сметы) и зелёная кнопка. */}
          <div className="es-empty-card">
            <svg className="es-empty-icon" width="32" height="32" viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><rect x="3.5" y="3.5" width="17" height="17" rx="2.5" /><path d="M3.5 9h17M9 9v11.5M14.5 9v11.5" /></svg>
            <h1>Начните с новой сметы</h1>
            <p>Загрузите инвойс в Excel, проверьте строки и получите расчёт растаможки. Все сметы сохраняются в списке слева.</p>
            <ol className="es-empty-steps" aria-label="Этапы сметы">
              <li><b>1</b><span>Файл<small>Excel поставщика</small></span></li>
              <li><b>2</b><span>Проверка строк<small>коды, количество, вес</small></span></li>
              <li><b>3</b><span>Результат<small>пошлина, НДС, к оплате</small></span></li>
            </ol>
            <button type="button" className="es-empty-create" onClick={() => void create()}>+ Создать смету</button>
          </div>
        </div>}
      </section>
    </div>
  </main>
}
