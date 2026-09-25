import { useEffect, useState } from 'react'
import { tnvedDirectoryApi } from '../tnved/tnvedDirectoryApi'
import { TNVEDDirectoryEntry } from '../tnved/types'
import { rateLabel } from '../tnved/tnvedLabels'
import { errorMessage } from '@/shared/lib/errorMessage'
import { isAdmin, User } from '@/shared/types/user'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { verificationStatusLabel } from '../directoryLabels'
import { DirectoryToolbar } from '../components/DirectoryToolbar'
import { RowActions } from '../components/RowActions'
import { ProductEntryPanel } from './ProductEntryPanel'
import { productDirectoryApi } from './productDirectoryApi'
import { ProductDirectoryEntry } from './types'
import '../directory.css'
import '../verificationStatus.css'

export function ProductDirectoryPage({ user, onLogout }: { user: User; onLogout: () => void }) {
  // Справочники меняет только администратор; остальные сотрудники их просматривают.
  const canEdit = isAdmin(user)
  const [items, setItems] = useState<ProductDirectoryEntry[]>([])
  const [tnved, setTnved] = useState<TNVEDDirectoryEntry[]>([])
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('all')
  const [showArchived, setShowArchived] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [panel, setPanel] = useState<{ item?: ProductDirectoryEntry } | null>(null)
  const [refresh, setRefresh] = useState(0)
  const search = useDebouncedValue(query.trim(), 250)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    Promise.all([
      productDirectoryApi.list({ q: search, status, includeArchived: showArchived }),
      tnvedDirectoryApi.list({ status: 'all' }),
    ]).then(([products, codes]) => { if (!cancelled) { setItems(products); setTnved(codes) } }).catch((reason) => { if (!cancelled) setError(errorMessage(reason)) }).finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [search, status, showArchived, refresh])

  async function archive(item: ProductDirectoryEntry) {
    if (!window.confirm(`Перенести в архив карточку «${item.name}»?`)) return
    try { await productDirectoryApi.archive(item.id); setRefresh((value) => value + 1) } catch (reason) { setError(errorMessage(reason)) }
  }
  async function restore(item: ProductDirectoryEntry) {
    try { await productDirectoryApi.restore(item.id); setRefresh((value) => value + 1) } catch (reason) { setError(errorMessage(reason)) }
  }
  async function copy(item: ProductDirectoryEntry) {
    try { await productDirectoryApi.copy(item.id); setRefresh((value) => value + 1) } catch (reason) { setError(errorMessage(reason)) }
  }

  const filtered = search !== '' || status !== 'all'
  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="products" onLogout={onLogout} />
    <section className="directory-page">
      <div className="dir-head">
        <div>
          <h1>Справочник товаров{!loading && <span className="dir-count">{items.length}</span>}</h1>
          <p>Карточки товаров с кодом ТН ВЭД и ИТС. При выборе карточки в смете всё подставляется автоматически.</p>
        </div>
        {canEdit && <button type="button" className="dir-add" onClick={() => setPanel({})}>+ Добавить товар</button>}
      </div>
      <DirectoryToolbar query={query} placeholder="Поиск по названию, коду или синониму" status={status} showArchived={showArchived} onQuery={setQuery} onStatus={setStatus} onShowArchived={setShowArchived} />
      {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}
      <div className={loading && items.length > 0 ? 'dir-table-wrap dir-loading' : 'dir-table-wrap'}>
        <table className="dir-table product-table">
          <thead><tr><th>Товар</th><th className="c-tnved">Код ТН ВЭД и ставки</th><th className="c-num">ИТС, USD/кг</th><th className="c-unit">Ед.</th><th className="c-status">Статус</th><th className="c-actions" aria-label="Действия" /></tr></thead>
          <tbody>
            {items.length === 0 ? <tr><td colSpan={6} className="dir-empty">{loading ? 'Загрузка…' : filtered ? 'Ничего не найдено. Измените поиск или фильтр.' : canEdit ? 'Карточек товаров пока нет. Добавьте первую.' : 'Карточек товаров пока нет.'}</td></tr>
              : items.map((item) => <tr key={item.id} className={item.archivedAt ? 'archived' : undefined} onClick={() => setPanel({ item })}>
                <td className="c-name">
                  <button type="button" className="dir-name" onClick={(event) => { event.stopPropagation(); setPanel({ item }) }} title={item.name}>{item.name}</button>
                  {(item.nameZh || item.synonymsRu[0]) && <small>{[item.nameZh, item.synonymsRu[0] && `синоним: ${item.synonymsRu[0]}`].filter(Boolean).join(' · ')}</small>}
                </td>
                <td className="c-tnved">
                  {item.tnvedCode ? <code>{item.tnvedCode}</code> : <span className="dir-warn">Код не привязан</span>}
                  {item.tnved && <small>пошлина {rateLabel(item.tnved.duty, 'Пошлина')} · НДС {rateLabel(item.tnved.vat, 'НДС')}{item.antiDumpingPercent ? ` · АД ${item.antiDumpingPercent}%` : ''}</small>}
                </td>
                <td className="c-num">{item.companyCoefficient || <span className="dir-none">—</span>}</td>
                <td className="c-unit">{item.unit}</td>
                <td className="c-status"><span className={`directory-status ${item.archivedAt ? 'archived' : item.verificationStatus}`}>{item.archivedAt ? 'В архиве' : verificationStatusLabel(item.verificationStatus)}</span></td>
                <td className="c-actions">{canEdit && <RowActions name={item.name} archived={Boolean(item.archivedAt)} onEdit={() => setPanel({ item })} onCopy={() => void copy(item)} onArchive={() => void archive(item)} onRestore={() => void restore(item)} />}</td>
              </tr>)}
          </tbody>
        </table>
      </div>
      <p className="dir-foot">Карточка не меняет исходный файл. В смету записывается снимок выбранного кода ТН ВЭД и его ставок.</p>
    </section>
    {panel && <ProductEntryPanel item={panel.item} readOnly={!canEdit} tnved={tnved.filter((entry) => !entry.archivedAt)} onClose={() => setPanel(null)} onSaved={() => { setPanel(null); setRefresh((value) => value + 1) }} />}
  </main>
}
