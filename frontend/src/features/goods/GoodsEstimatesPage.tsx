import { useEffect, useMemo, useState } from 'react'
import { goodsEstimatesApi } from './goodsEstimatesApi'
import { GoodsEstimateInput, GoodsEstimateSummary } from '@/shared/types/goods'
import { isAdmin, User } from '@/shared/types/user'
import { Link } from '@/shared/router/Link'
import { GoodsCalculator } from './calculator/GoodsCalculator'
import { estimateTitle } from '@/shared/lib/estimateTitle'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatRuDate, todayIso } from '@/shared/lib/dates'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { navigate, paths } from '@/shared/router/router'
import { EstimatesSidebar } from './components/EstimatesSidebar/EstimatesSidebar'
import './GoodsEstimatesPage.css'

// estimateId — номер сметы из адреса /estimates/goods/:id (null — адрес без номера).
type Props = { user: User; estimateId: number | null; onLogout: () => void }

export function GoodsEstimatesPage({ user, estimateId, onLogout }: Props) {
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

  // Адрес без номера или с номером удалённой/недоступной сметы — открываем первую из списка.
  // Администратор сначала пробует открыть смету сотрудника по номеру.
  useEffect(() => {
    if (loading || selected) return
    if (estimateId && isAdmin(user) && foreignFailed !== estimateId) {
      let active = true
      goodsEstimatesApi.get(estimateId)
        .then((estimate) => { if (active) setForeign({ id: estimate.id, name: estimate.name, client: estimate.client, assessmentDate: estimate.assessmentDate, status: estimate.status, createdAt: estimate.createdAt, updatedAt: estimate.updatedAt, ownerId: estimate.ownerId, ownerName: estimate.ownerName }) })
        .catch(() => { if (active) setForeignFailed(estimateId) })
      return () => { active = false }
    }
    navigate(paths.goods(estimates[0]?.id), { replace: true })
  }, [loading, selected, estimates, estimateId, user, foreignFailed])
  const employeeEstimate = selected !== null && selected === foreign && foreign.ownerId !== undefined && foreign.ownerId !== user.id

  useEffect(() => {
    if (!selected) return
    setMeta({ name: estimateTitle(selected.name), client: selected.client, assessmentDate: selected.assessmentDate })
  }, [selected])

  // Новая смета с главной (?new=1): сразу открыть поля названия и клиента, курсор — в название.
  useEffect(() => {
    if (!selected || new URLSearchParams(window.location.search).get('new') !== '1') return
    window.history.replaceState(window.history.state, '', window.location.pathname)
    const editor = document.querySelector<HTMLDetailsElement>('.estimate-bar-editor')
    if (!editor) return
    editor.open = true
    const input = editor.querySelector<HTMLInputElement>('input')
    input?.focus()
    input?.select()
  }, [selected])

  async function create() {
    setError('')
    try {
      const estimate = await goodsEstimatesApi.create({ name: `Смета товаров · ${todayIso()}`, client: '', assessmentDate: todayIso() })
      setEstimates((current) => [estimate, ...current])
      navigate(paths.goods(estimate.id))
    } catch (reason) {
      setError(errorMessage(reason))
    }
  }

  async function saveMeta() {
    if (!selected || !meta.name.trim() || savingMeta) return false
    setSavingMeta(true); setError('')
    try {
      const normalizedMeta = { ...meta, name: estimateTitle(meta.name) }
      const saved = await goodsEstimatesApi.update(selected.id, normalizedMeta)
      setMeta(normalizedMeta)
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
    if (!window.confirm(`Удалить смету «${estimateTitle(estimate.name)}»? Все строки и результат будут удалены.`)) return
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
      <EstimatesSidebar title="Сметы товаров" estimates={estimates} selectedId={selected?.id ?? null} loading={loading} nameOf={(estimate) => estimateTitle(estimate.name)} hrefOf={paths.goods} onCreate={() => void create()} onRemove={(estimate) => void remove(estimate)} />
      <section className="estimates-content">
        {selected ? <GoodsCalculator
          key={selected.id}
          estimateId={selected.id}
          onEstimateSaved={onEstimateSaved}
          exportName={[estimateTitle(selected.name), selected.client].filter(Boolean).join(' — ')}
          headerSlot={<div className="estimate-bar-meta">
            <h1 title={estimateTitle(selected.name)}>{estimateTitle(selected.name)}</h1>
            <span className="estimate-bar-sub">{selected.client || 'Клиент не указан'} · {formatRuDate(selected.assessmentDate)}</span>
            {employeeEstimate && <span className="estimate-owner" title="Изменения и расчёт сохраняются в смете сотрудника">Смета сотрудника: {foreign?.ownerName || 'сотрудник'} · <Link href={paths.employees}>к сотрудникам</Link></span>}
            <details className="estimate-bar-editor"><summary>Изменить</summary><div className="estimate-bar-fields"><label>Название<input value={estimateTitle(meta.name)} onChange={(event) => setMeta((current) => ({ ...current, name: event.target.value }))} maxLength={200} /></label><label>Клиент<input value={meta.client} onChange={(event) => setMeta((current) => ({ ...current, client: event.target.value }))} maxLength={200} placeholder="Необязательно" /></label><label>Дата<input type="date" value={meta.assessmentDate} onChange={(event) => setMeta((current) => ({ ...current, assessmentDate: event.target.value }))} /></label><button type="button" className="secondary" onClick={(event) => { const details = event.currentTarget.closest('details'); void saveMeta().then((saved) => { if (saved) details?.removeAttribute('open') }) }} disabled={savingMeta || !meta.name.trim()}>{savingMeta ? 'Сохраняем…' : 'Сохранить'}</button></div></details>
          </div>}
        /> : !loading && estimates.length === 0 && <div className="estimates-empty"><div><h1>Начните с новой сметы</h1><p>Загрузите инвойс в Excel, проверьте строки и получите расчёт растаможки. Все сметы сохраняются в списке слева.</p><button type="button" onClick={() => void create()}>+ Создать смету</button></div></div>}
      </section>
    </div>
  </main>
}
