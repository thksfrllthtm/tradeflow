import { useEffect, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { Link } from '@/shared/router/Link'
import { paths } from '@/shared/router/router'
import { goodsEstimatesApi } from '@/features/goods/goodsEstimatesApi'
import { estimateTitle } from '@/shared/lib/estimateTitle'
import { formatRuDate } from '@/shared/lib/dates'
import { GoodsEstimateSummary } from '@/shared/types/goods'
import { Employee } from './employeesApi'
import '@/shared/styles/drawer.css'

// Сметы сотрудника (администратор): список справа, каждая открывается по ссылке — там её
// можно смотреть, менять и пересчитывать. В своих «Сметах» администратор чужих не видит.
export function EmployeeEstimatesPanel({ employee, onClose }: { employee: Employee; onClose: () => void }) {
  const [items, setItems] = useState<GoodsEstimateSummary[] | null>(null)
  const [error, setError] = useState('')
  const who = employee.name || employee.email

  useEffect(() => {
    goodsEstimatesApi.listForUser(employee.id).then(setItems).catch((reason) => { setItems([]); setError(errorMessage(reason)) })
  }, [employee.id])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose])

  return <div className="drawer-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
    <aside className="drawer emp-estimates" role="dialog" aria-modal="true" aria-labelledby="emp-estimates-title">
      <header className="drawer-head">
        <div><h2 id="emp-estimates-title">Сметы: {who}</h2><p>{items ? `${items.length} — откройте, чтобы посмотреть, изменить или пересчитать` : 'Загрузка…'}</p></div>
        <button type="button" className="drawer-close" onClick={onClose} aria-label="Закрыть" title="Закрыть (Esc)">×</button>
      </header>
      <div className="drawer-body">
        {error && <p className="emp-estimates-error" role="alert">{error}</p>}
        {items?.length === 0 && !error && <p className="emp-estimates-empty">У сотрудника пока нет смет.</p>}
        {items && items.length > 0 && <ul className="emp-estimates-list">
          {items.map((estimate) => <li key={estimate.id}>
            <Link href={paths.goods(estimate.id)}>
              <span className="emp-estimate-top"><strong>{estimateTitle(estimate.name)}</strong><i className={estimate.status === 'Рассчитано' ? 'done' : undefined}>{estimate.status}</i></span>
              <small>{estimate.client || 'Без клиента'} · {formatRuDate(estimate.assessmentDate)} · изменена {formatRuDate(estimate.updatedAt.slice(0, 10))}</small>
            </Link>
          </li>)}
        </ul>}
      </div>
    </aside>
  </div>
}
