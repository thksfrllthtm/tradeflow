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
import { PhotoLightbox } from '@/shared/components/PhotoLightbox/PhotoLightbox'
import { CopyButton } from '@/shared/components/CopyCode/CopyButton'
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
  // Фото записи: сразу миниатюра, следом — полные фото (в списке их нет, только в карточке записи).
  const [photos, setPhotos] = useState<string[] | null>(null)
  async function openPhotos(item: CodeDecision) {
    setPhotos([item.thumb])
    try {
      const full = await decisionsApi.get(item.id)
      if (full.photos.length) setPhotos((current) => current && current[0] === item.thumb ? full.photos : current)
    } catch {
      // Полные фото не загрузились — остаётся миниатюра.
    }
  }

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

  const counts: Partial<Record<Filter, number>> = stats ? { '': stats.pending + stats.verified + stats.rejected, pending: stats.pending, verified: stats.verified, rejected: stats.rejected } : {}
  // Как этапы 2–3 сметы: страница во всю ширину, таблица — карточка до низа окна со своей прокруткой; сверху в
  // карточке — поиск, вкладки статуса со счётчиками, точность ИИ и кнопки; строки — в одну линию.
  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="decisions" onLogout={onLogout} />
    <section className="directory-page dc-page">
      <div className="dir-head">
        <div>
          <h1>База решений</h1>
          <p>Проверенные примеры «товар из инвойса → код ТН ВЭД» с фото и обоснованием. На них опирается ИИ при подборе кода.</p>
        </div>
        {admin && <div className="dc-actions">
          <button type="button" className="dc-btn" onClick={() => setImportOpen(true)}>Импорт базы из Excel</button>
          <button type="button" className="dc-btn primary" onClick={() => setManualOpen(true)}>+ Добавить вручную</button>
        </div>}
      </div>
      {notice && <p className="dir-transfer-ok" role="status">{notice}</p>}
      {error && <p className="dir-transfer-error" role="alert">{error}</p>}
      <section className="dc-card" aria-label="Решения">
        <div className="dc-filterbar">
          <label className="dc-search">
            <SearchIcon />
            <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Наименование или код" aria-label="Поиск по базе решений" />
          </label>
          <div className="dc-tabs" role="radiogroup" aria-label="Статус">
            {filters.map((item) => <button key={item.value} type="button" role="radio" aria-checked={filter === item.value}
              className={`${filter === item.value ? 'active' : ''}${item.value === 'pending' && (stats?.pending ?? 0) > 0 ? ' warn' : ''}`} onClick={() => setFilter(item.value)}>
              {item.label}{counts[item.value] !== undefined && <b>{counts[item.value]}</b>}
            </button>)}
          </div>
        </div>
        <div className={loading ? 'dir-table-wrap dir-loading' : 'dir-table-wrap'}>
          <table className="sheet sheet-rownum dir-table dc-table">
            <thead><tr>
              <th className="dc-photo-col">Фото</th><th>Наименование</th><th className="dc-code-col">Код ТН ВЭД</th><th className="dc-price-col">Цена за 1 шт</th><th className="dc-status-col">Статус</th>
              <th className="dc-source-col">Источник</th><th className="dc-by-col">Добавил</th><th className="dc-date-col">Дата</th>
            </tr></thead>
            <tbody>
              {items.length === 0 ? <tr><td colSpan={8} className="dir-empty">{loading ? 'Загрузка…' : search || filter ? 'Ничего не найдено' : 'База пока пустая. Записи появятся из подбора кода ИИ (кнопка «ИИ» в строке сметы)' + (admin ? ' или из импорта старого инвойса.' : '.')}</td></tr>
                : items.map((item) => {
                  const name = item.finalName || item.invoiceName || item.supplierName
                  return <tr key={item.id} onClick={() => setOpenId(item.id)}>
                    <td className="dc-photo-col">{item.thumb
                      ? <button type="button" className="dc-thumb" title="Открыть фото" aria-label="Открыть фото" onClick={(event) => { event.stopPropagation(); void openPhotos(item) }}><img src={item.thumb} alt="" /></button>
                      : <span className="dc-nophoto" aria-hidden="true" />}</td>
                    <td className="dc-names" title={name}>{name ? <span className="dc-name"><span className="dc-name-text">{name}</span><CopyButton text={name} label="наименование" /></span> : <span className="dir-none">—</span>}</td>
                    <td className="dc-code-col"><CopyCode code={item.code} />{!item.nomenclature && <span className="dc-warn" title="Кода нет в номенклатуре ЕАЭС"> ⚠</span>}</td>
                    <td className="dc-price-col">{item.unitPriceUsd ? <><CopyCode code={item.unitPriceUsd.replace('.', ',')} number /> $</> : <span className="dir-none">—</span>}</td>
                    <td className="dc-status-col">
                      <span className={`dc-status ${item.status}`}>{decisionStatusLabels[item.status]}</span>
                      {item.agentCorrect === false && <span className="dc-ai bad" title={`ИИ ошибся: предлагал ${formatCode(item.agentCode)}`}>ИИ</span>}
                      {item.agentCorrect === true && <span className="dc-ai ok" title="ИИ подобрал этот код верно">ИИ</span>}
                    </td>
                    <td className="dc-source-col">{decisionSourceLabels[item.source]}</td>
                    <td className="dc-by-col">{item.createdByName || <span className="dir-none">—</span>}</td>
                    <td className="dc-date-col">{formatRuDate(item.createdAt.slice(0, 10))}</td>
                  </tr>
                })}
            </tbody>
          </table>
        </div>
        {items.length < total && <div className="dc-more">Показано <b>{items.length}</b> из {total}<button type="button" className="dc-btn" onClick={() => void loadMore()}>Показать ещё</button></div>}
      </section>
      <p className="dc-disclaimer">
        <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"><circle cx="8" cy="8" r="6.3" /><path d="M8 7.3v3.6" /><circle cx="8" cy="5" r=".4" fill="currentColor" /></svg>
        <span>«Проверено» ставит администратор. Исправленный при проверке код отмечается «ИИ ошибся». Проверенное решение агент считает сильным доводом, а тот же товар находится сразу, без ИИ.</span>
      </p>
    </section>
    {photos && <PhotoLightbox photos={photos} startIndex={0} onClose={() => setPhotos(null)} />}
    {openId && <DecisionDrawer id={openId} admin={admin} onClose={() => setOpenId(null)} onChanged={() => setRefreshKey((key) => key + 1)} />}
    {manualOpen && <DecisionManual onClose={() => setManualOpen(false)} onAdded={() => { setNotice('Решение добавлено в базу'); setRefreshKey((key) => key + 1) }} />}
    {importOpen && <DecisionImport onClose={() => setImportOpen(false)} onImported={(created, skipped, replaced) => { setNotice(`${replaced ? 'База заменена: ' : 'В базу добавлено '}${created} ${plural(created, 'решение', 'решения', 'решений')}${skipped ? `, повторов пропущено: ${skipped}` : ''}`); setRefreshKey((key) => key + 1) }} />}
  </main>
}
