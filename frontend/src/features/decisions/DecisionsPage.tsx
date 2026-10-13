import { useEffect, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { errorMessage } from '@/shared/lib/errorMessage'
import { plural } from '@/shared/lib/plural'
import { isAdmin, User } from '@/shared/types/user'
import { formatRuDate } from '@/shared/lib/dates'
import { SearchIcon } from '@/shared/components/icons'
import { DecisionStats, decisionSourceLabels, decisionsApi } from './decisionsApi'
import { DecisionDrawer } from './DecisionDrawer'
import { DecisionImport } from './DecisionImport'
import { DecisionManual } from './DecisionManual'
import '@/shared/styles/list-page.css'
import './decisions.css'
import { formatCode } from '@/shared/tnved/codes'
import { CodeDecision, decisionStatusLabels } from '@/shared/tnved/decisions'

const pageSize = 50
type Filter = '' | 'pending' | 'verified' | 'rejected' | 'agentWrong'
const filters: { value: Filter; label: string }[] = [
  { value: '', label: 'Все' }, { value: 'pending', label: 'Ждут проверки' }, { value: 'verified', label: 'Проверено' },
  { value: 'rejected', label: 'Отклонено' }, { value: 'agentWrong', label: 'ИИ ошибся' },
]

function initialFilter(): Filter {
  const value = new URLSearchParams(window.location.search).get('status')
  return filters.some((item) => item.value === value) ? value as Filter : ''
}

// База решений «товар из инвойса → код ТН ВЭД»: на неё опирается ИИ-агент при подборе кода.
// Записи «Не проверено» приходят из подбора и от сотрудников; администратор проверяет их,
// исправляет код (отмечается, что ИИ ошибся) и импортирует старые инвойсы с правильными кодами.
export function DecisionsPage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const admin = isAdmin(user)
  const [query, setQuery] = useState('')
  const search = useDebouncedValue(query.trim(), 300)
  const [filter, setFilter] = useState<Filter>(initialFilter)
  const [items, setItems] = useState<CodeDecision[]>([])
  const [total, setTotal] = useState(0)
  const [stats, setStats] = useState<DecisionStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [refreshKey, setRefreshKey] = useState(0)
  const [openId, setOpenId] = useState<number | null>(() => Number(new URLSearchParams(window.location.search).get('id')) || null)
  const [importOpen, setImportOpen] = useState(false)
  const [manualOpen, setManualOpen] = useState(false)

  const params = () => ({ q: search, status: filter === 'agentWrong' ? '' : filter, agentWrong: filter === 'agentWrong' })
  useEffect(() => {
    let active = true
    setLoading(true); setError('')
    decisionsApi.list({ ...params(), offset: 0, limit: pageSize })
      .then((page) => { if (active) { setItems(page.items); setTotal(page.total); setStats(page.stats) } })
      .catch((reason) => { if (active) setError(errorMessage(reason)) })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [search, filter, refreshKey])

  useEffect(() => {
    if (!notice) return
    const timer = window.setTimeout(() => setNotice(''), 4000)
    return () => window.clearTimeout(timer)
  }, [notice])

  async function loadMore() {
    try {
      const page = await decisionsApi.list({ ...params(), offset: items.length, limit: pageSize })
      setItems((current) => [...current, ...page.items]); setTotal(page.total)
    } catch (reason) {
      setError(errorMessage(reason))
    }
  }

  const accuracy = stats && stats.agentChecked > 0 ? Math.round((stats.agentCorrect / stats.agentChecked) * 100) : null
  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="decisions" onLogout={onLogout} />
    <section className="directory-page">
      <div className="dir-head">
        <div>
          <h1>База решений{stats && <span className="dir-count">{stats.pending + stats.verified}</span>}</h1>
          <p>Проверенные примеры «товар из инвойса → код ТН ВЭД» с фото и обоснованием. На них опирается ИИ при подборе кода.</p>
        </div>
        {admin && <div className="dir-head-actions"><button type="button" className="dir-add" onClick={() => setManualOpen(true)}>+ Добавить вручную</button><button type="button" className="dir-add" onClick={() => setImportOpen(true)}>Импорт старого инвойса</button></div>}
      </div>
      {stats && <div className="dc-stats">
        <button type="button" className={stats.pending > 0 ? 'dc-stat warn' : 'dc-stat'} onClick={() => setFilter('pending')}><b>{stats.pending}</b> {plural(stats.pending, 'ждёт', 'ждут', 'ждут')} проверки</button>
        <button type="button" className="dc-stat" onClick={() => setFilter('verified')}><b>{stats.verified}</b> проверено</button>
        <span className="dc-stat" title="Решения из подбора ИИ, проверенные администратором за 90 дней">{accuracy === null ? 'Точность ИИ: проверенных подборов пока нет' : <>Точность ИИ: <b>{accuracy}%</b> ({stats.agentCorrect} из {stats.agentChecked} за 90 дней)</>}</span>
      </div>}
      {notice && <p className="dir-transfer-ok" role="status">{notice}</p>}
      {error && <p className="dir-transfer-error" role="alert">{error}</p>}
      <div className="dir-toolbar">
        <label className="dir-search">
          <SearchIcon />
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Поиск по наименованию или коду" aria-label="Поиск по базе решений" />
        </label>
        <div className="dir-seg" role="radiogroup" aria-label="Статус">
          {filters.map((item) => <button key={item.value} type="button" role="radio" aria-checked={filter === item.value} className={filter === item.value ? 'active' : undefined} onClick={() => setFilter(item.value)}>{item.label}</button>)}
        </div>
      </div>
      <div className={loading ? 'dir-table-wrap dir-loading' : 'dir-table-wrap'}>
        <table className="sheet sheet-rownum dir-table dc-table">
          <thead><tr><th className="dc-photo-col">Фото</th><th>Итог</th><th className="dc-code-col">Код</th><th className="dc-status-col">Статус</th><th className="dc-from-col">Откуда</th></tr></thead>
          <tbody>
            {items.length === 0 ? <tr><td colSpan={5} className="dir-empty">{loading ? 'Загрузка…' : search || filter ? 'Ничего не найдено' : 'База пока пустая. Записи появятся из подбора кода ИИ (кнопка «ИИ» в строке сметы)' + (admin ? ' или из импорта старого инвойса.' : '.')}</td></tr>
              : items.map((item) => <tr key={item.id} onClick={() => setOpenId(item.id)}>
                <td className="dc-photo-col">{item.thumb ? <img src={item.thumb} alt="" /> : <span className="dc-nophoto" aria-hidden="true" />}</td>
                <td className="dc-names"><span>{item.finalName || item.invoiceName || item.supplierName || <span className="dir-none">—</span>}</span></td>
                <td className="dc-code-col"><CopyCode code={item.code} />{!item.nomenclature && <small className="dc-warn">нет в номенклатуре</small>}</td>
                <td className="dc-status-col"><span className={`dc-status ${item.status}`}>{decisionStatusLabels[item.status]}</span>{item.agentCorrect === false && <small className="dc-warn" title={`ИИ предлагал ${formatCode(item.agentCode)}`}>ИИ ошибся</small>}{item.agentCorrect === true && <small className="dc-ok">ИИ верно</small>}</td>
                <td className="dc-from-col"><span>{decisionSourceLabels[item.source]}</span><small>{item.createdByName || '—'} · {formatRuDate(item.createdAt.slice(0, 10))}</small></td>
              </tr>)}
          </tbody>
        </table>
        {items.length < total && <div className="nom-more dc-more">Показано {items.length} из {total}<button type="button" className="dir-transfer-button" onClick={() => void loadMore()}>Показать ещё</button></div>}
      </div>
      <p className="dir-foot">«Проверено» ставит администратор. Если при проверке код исправлен, запись отмечается «ИИ ошибся» — из этих отметок считается точность ИИ. Проверенное решение агент считает сильным доводом, а тот же товар (по фото или наименованию) находится сразу, без ИИ.</p>
    </section>
    {openId && <DecisionDrawer id={openId} admin={admin} onClose={() => setOpenId(null)} onChanged={() => setRefreshKey((key) => key + 1)} />}
    {manualOpen && <DecisionManual onClose={() => setManualOpen(false)} onAdded={() => { setNotice('Решение добавлено в базу'); setRefreshKey((key) => key + 1) }} />}
    {importOpen && <DecisionImport onClose={() => setImportOpen(false)} onImported={(created, skipped) => { setNotice(`В базу добавлено ${created} ${plural(created, 'решение', 'решения', 'решений')}${skipped ? `, уже были в базе: ${skipped}` : ''}`); setRefreshKey((key) => key + 1) }} />}
  </main>
}
