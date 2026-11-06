import { useEffect, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { Link } from '@/shared/router/Link'
import { paths } from '@/shared/router/router'
import { goodsEstimatesApi } from '@/features/goods/goodsEstimatesApi'
import { estimateName, estimateSubtitle } from '@/shared/lib/estimateTitle'
import { formatRuDate } from '@/shared/lib/dates'
import { GoodsEstimateSummary } from '@/shared/types/goods'
import { Employee } from './employeesApi'
import { Drawer } from '@/shared/components/Drawer/Drawer'

// Сметы сотрудника (администратор): список справа, каждая открывается по ссылке — там её
// можно смотреть, менять и пересчитывать. В своих «Сметах» администратор чужих не видит.
export function EmployeeEstimatesPanel({ employee, onClose }: { employee: Employee; onClose: () => void }) {
  const [items, setItems] = useState<GoodsEstimateSummary[] | null>(null)
  const [error, setError] = useState('')
  const who = employee.name || employee.email

  useEffect(() => {
    goodsEstimatesApi.listForUser(employee.id).then(setItems).catch((reason) => { setItems([]); setError(errorMessage(reason)) })
  }, [employee.id])

  return <Drawer className="emp-estimates" title={`Сметы: ${who}`} onClose={onClose}
    subtitle={items ? `${items.length} — откройте, чтобы посмотреть, изменить или пересчитать` : 'Загрузка…'}>
        {error && <p className="emp-estimates-error" role="alert">{error}</p>}
        {items?.length === 0 && !error && <p className="emp-estimates-empty">У сотрудника пока нет смет.</p>}
        {items && items.length > 0 && <ul className="emp-estimates-list">
          {items.map((estimate) => <li key={estimate.id}>
            <Link href={paths.goods(estimate.id)}>
              <span className="emp-estimate-top"><strong>{estimateName(estimate)}</strong><i className={estimate.status === 'Рассчитано' ? 'badge ok' : 'badge'}>{estimate.status}</i></span>
              <small>{estimateSubtitle(estimate)} · <time dateTime={estimate.assessmentDate.slice(0, 10)}>{formatRuDate(estimate.assessmentDate)}</time> · изменена <time dateTime={estimate.updatedAt}>{formatRuDate(estimate.updatedAt.slice(0, 10))}</time></small>
            </Link>
          </li>)}
        </ul>}
  </Drawer>
}
