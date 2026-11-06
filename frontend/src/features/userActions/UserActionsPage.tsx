import { useEffect, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { PageHead } from '@/shared/components/PageHead/PageHead'
import { request } from '@/shared/api/client'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatRuTime } from '@/shared/lib/dates'
import { User } from '@/shared/types/user'
import { Employee, employeesApi } from '@/features/employees/employeesApi'
import { isoDaysAgo, rangeText } from '@/features/agentLog/logRows'
import '@/shared/styles/list-page.css'
import '@/features/agentLog/agentLog.css'

// userId null и пустое имя — неудачный вход или удалённый сотрудник: тогда видно только email.
type ActionRow = {
  id: number; userId: number | null; user: string; email: string; action: string; target: string
  status: number; ip: string; count: number; createdAt: string; updatedAt: string
}
type Filter = { from: string; to: string; userId: string }

const pageSize = 50

function when(iso: string) {
  const date = new Date(iso)
  return `${date.toLocaleDateString('ru-RU')} ${formatRuTime(date)}`
}

// Журнал действий: изменения, вход и выход всех сотрудников. Только администратор: сервер проверяет права сам.
export function UserActionsPage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [filter, setFilter] = useState<Filter>({ from: isoDaysAgo(6), to: isoDaysAgo(0), userId: '' })
  const [page, setPage] = useState(1)
  const [data, setData] = useState<{ rows: ActionRow[]; total: number } | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [employees, setEmployees] = useState<Employee[]>([])

  useEffect(() => { employeesApi.list().then(setEmployees).catch(() => setEmployees([])) }, [])
  useEffect(() => {
    let stale = false
    setLoading(true)
    const params = new URLSearchParams({ page: String(page), limit: String(pageSize), from: filter.from, to: filter.to })
    if (filter.userId) params.set('userId', filter.userId)
    request<{ rows: ActionRow[]; total: number }>(`/api/user-actions?${params}`)
      .then((result) => { if (!stale) { setData(result); setError('') } })
      .catch((reason) => { if (!stale) setError(errorMessage(reason)) })
      .finally(() => { if (!stale) setLoading(false) })
    return () => { stale = true }
  }, [filter, page])

  const change = (patch: Partial<Filter>) => { setFilter((current) => ({ ...current, ...patch })); setPage(1) }
  const total = data?.total ?? 0
  const pages = Math.max(1, Math.ceil(total / pageSize))

  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="userActions" onLogout={onLogout} />
    <section className="directory-page alog">
      <PageHead title="Журнал действий" count={data ? total : undefined}
        subtitle="Кто из сотрудников что изменил, когда входил и выходил. Записи хранятся полгода." />
      {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}
      <div className="alog-card">
        <div className="alog-filterbar">
          <label className="alog-period">
            <span>Период</span>
            <input type="date" value={filter.from} max={filter.to} onChange={(event) => change({ from: event.target.value })} aria-label="С" />
            <i>–</i>
            <input type="date" value={filter.to} min={filter.from} onChange={(event) => change({ to: event.target.value })} aria-label="По" />
          </label>
          <label className="alog-select">
            <span>Сотрудник</span>
            <select value={filter.userId} onChange={(event) => change({ userId: event.target.value })}>
              <option value="">все</option>
              {employees.map((employee) => <option key={employee.id} value={employee.id}>{employee.name || employee.email}</option>)}
            </select>
          </label>
          {loading && <span className="alog-loading" role="status">Загрузка…</span>}
        </div>
        <div className={loading && data ? 'dir-table-wrap dir-loading' : 'dir-table-wrap'}>
          <table className="sheet dir-table alog-table">
            <thead><tr>
              <th className="c-when">Когда</th><th className="c-user">Сотрудник</th><th>Действие</th><th className="c-applied">Запись</th>
              <th className="c-verify">Итог</th><th className="c-applied">IP</th>
            </tr></thead>
            <tbody>
              {!data || data.rows.length === 0
                ? <tr><td colSpan={6} className="dir-empty">{loading ? 'Загрузка…' : 'Действий за период нет.'}</td></tr>
                : data.rows.map((row) => <tr key={row.id}>
                  <td className="c-when">
                    <time dateTime={row.updatedAt}>{when(row.updatedAt)}</time>
                    {row.count > 1 && <small title={`первый раз ${when(row.createdAt)}`}>повторов: {row.count}</small>}
                  </td>
                  <td className="c-user" title={row.email}>{row.user || row.email}</td>
                  <td>{row.action}</td>
                  <td className="c-applied">{row.target ? `№ ${row.target}` : <span className="alog-none">—</span>}</td>
                  <td className="c-verify">{row.status < 400
                    ? <span className="status-dot ok">выполнено</span>
                    : <span className="status-dot error" title={`код ответа ${row.status}`}>{row.status === 403 ? 'нет прав' : row.status === 401 ? 'неверный пароль' : 'отказ'}</span>}</td>
                  <td className="c-applied">{row.ip}</td>
                </tr>)}
            </tbody>
          </table>
        </div>
        <div className="alog-more">
          <span>{total > 0 ? rangeText(page, pageSize, total) : 'нет действий'}</span>
          {pages > 1 && <div>
            <button type="button" className="secondary" onClick={() => setPage(page - 1)} disabled={page <= 1 || loading}>← Новее</button>
            <button type="button" className="secondary" onClick={() => setPage(page + 1)} disabled={page >= pages || loading}>Старее →</button>
          </div>}
        </div>
      </div>
    </section>
  </main>
}
