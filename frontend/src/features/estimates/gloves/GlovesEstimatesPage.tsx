import { useEffect, useMemo, useState } from 'react'
import { glovesEstimatesApi } from './glovesEstimatesApi'
import { GlovesEstimate, GlovesEstimateInput, GlovesEstimateSummary } from '../shared/types'
import { User } from '@/shared/types/user'
import { GlovesCalculator } from './calculator/GlovesCalculator'
import { errorMessage } from '@/shared/lib/errorMessage'
import { todayIso } from '../shared/lib/dates'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { navigate, paths } from '@/shared/router/router'
import { EstimatesSidebar } from '../shared/components/EstimatesSidebar/EstimatesSidebar'
import '../shared/styles/estimatesWorkspace.css'

// estimateId — номер сметы из адреса /estimates/gloves/:id (null — адрес без номера).
type Props = { user: User; estimateId: number | null; onLogout: () => void }

export function GlovesEstimatesPage({ user, estimateId, onLogout }: Props) {
  const [estimates, setEstimates] = useState<GlovesEstimateSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [savingMeta, setSavingMeta] = useState(false)
  const [error, setError] = useState('')
  const selected = useMemo(() => estimates.find((estimate) => estimate.id === estimateId) ?? null, [estimates, estimateId])
  const [meta, setMeta] = useState<GlovesEstimateInput>({ name: '', client: '', assessmentDate: todayIso() })

  useEffect(() => {
    let active = true
    void glovesEstimatesApi.list().then((items) => {
      if (!active) return
      setEstimates(items)
    }).catch((reason) => {
      if (active) setError(errorMessage(reason))
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
  }, [])

  // Адрес без номера или с номером удалённой/чужой сметы — открываем первую из списка.
  useEffect(() => {
    if (loading || selected) return
    navigate(paths.gloves(estimates[0]?.id), { replace: true })
  }, [loading, selected, estimates])

  useEffect(() => {
    if (!selected) return
    setMeta({ name: selected.name, client: selected.client, assessmentDate: selected.assessmentDate })
  }, [selected])

  async function create() {
    setError('')
    try {
      const estimate = await glovesEstimatesApi.create({ name: `Перчатки · ${todayIso()}`, client: '', assessmentDate: todayIso() })
      setEstimates((current) => [estimate, ...current])
      navigate(paths.gloves(estimate.id))
    } catch (reason) {
      setError(errorMessage(reason))
    }
  }

  async function saveMeta() {
    if (!selected || !meta.name.trim() || savingMeta) return
    setSavingMeta(true); setError('')
    try {
      const saved = await glovesEstimatesApi.update(selected.id, meta)
      setEstimates((current) => current.map((item) => item.id === saved.id ? saved : item))
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setSavingMeta(false)
    }
  }

  async function remove(estimate: GlovesEstimateSummary) {
    if (!window.confirm(`Удалить смету «${estimate.name}»? Все строки и результат будут удалены.`)) return
    try {
      await glovesEstimatesApi.remove(estimate.id)
      const remaining = estimates.filter((item) => item.id !== estimate.id)
      setEstimates(remaining)
      if (estimateId === estimate.id) navigate(paths.gloves(remaining[0]?.id), { replace: true })
    } catch (reason) {
      setError(errorMessage(reason))
    }
  }

  function onEstimateSaved(saved: GlovesEstimate) {
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

  return <main className="app-shell">
    <AppHeader user={user} section="estimates" onLogout={onLogout} />
    {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}
    <div className="estimates-workspace">
      <EstimatesSidebar title="Сметы перчаток" estimates={estimates} selectedId={selected?.id ?? null} loading={loading} nameOf={(estimate) => estimate.name} hrefOf={paths.gloves} onCreate={() => void create()} onRemove={(estimate) => void remove(estimate)} />
      <section className="estimates-content">
        {selected ? <>
          <div className="gloves-estimate-meta">
            <div><p className="eyebrow">ВЫБРАННАЯ СМЕТА</p><h1>{selected.name}</h1></div>
            <div className="gloves-estimate-meta-fields"><label>Название<input value={meta.name} onChange={(event) => setMeta((current) => ({ ...current, name: event.target.value }))} maxLength={200} /></label><label>Клиент<input value={meta.client} onChange={(event) => setMeta((current) => ({ ...current, client: event.target.value }))} maxLength={200} placeholder="Необязательно" /></label><label>Дата<input type="date" value={meta.assessmentDate} onChange={(event) => setMeta((current) => ({ ...current, assessmentDate: event.target.value }))} /></label><button type="button" className="secondary" onClick={() => void saveMeta()} disabled={savingMeta || !meta.name.trim()}>{savingMeta ? 'Сохраняем…' : 'Сохранить данные сметы'}</button></div>
          </div>
          <GlovesCalculator key={selected.id} estimateId={selected.id} onEstimateSaved={onEstimateSaved} />
        </> : !loading && estimates.length === 0 && <div className="estimates-empty"><div><h1>Начните с новой сметы</h1><p>Создайте отдельную смету для партии перчаток. Она не будет смешиваться с обычными сметами.</p><button type="button" onClick={() => void create()}>+ Создать смету</button></div></div>}
      </section>
    </div>
  </main>
}

