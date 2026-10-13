import { useEffect, useState } from 'react'
import { NomenclatureCode } from '@/shared/tnved/nomenclature'
import { nomenclatureApi } from '../nomenclature/nomenclatureApi'
import { errorMessage } from '@/shared/lib/errorMessage'
import { isAdmin, User } from '@/shared/types/user'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { verificationStatusLabel } from '../common/directoryLabels'
import { DirectoryToolbar } from '../components/DirectoryToolbar'
import { useDirectoryTransfer } from '../components/DirectoryTransfer'
import { RowActions } from '../components/RowActions'
import { ProductEntryPanel } from './ProductEntryPanel'
import { productDirectoryApi } from './productDirectoryApi'
import { ProductDirectoryEntry } from './types'
import '@/shared/styles/list-page.css'
import '@/shared/styles/verification-status.css'

export function ProductDirectoryPage({ user, onLogout }: { user: User; onLogout: () => void }) {
  // Справочники меняет только администратор; остальные сотрудники их просматривают.
  const canEdit = isAdmin(user)
  const [items, setItems] = useState<ProductDirectoryEntry[]>([])
  // Ставки кодов товаров — из номенклатуры ТН ВЭД ЕАЭС.
  const [codes, setCodes] = useState<Record<string, NomenclatureCode>>({})
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('all')
  const [showArchived, setShowArchived] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [panel, setPanel] = useState<{ item?: ProductDirectoryEntry } | null>(null)
  const [refresh, setRefresh] = useState(0)
  const search = useDebouncedValue(query.trim(), 250)
  const transfer = useDirectoryTransfer({
    apiPath: '/api/product-directory', title: 'Справочник товаров', onImported: () => setRefresh((value) => value + 1),
    hint: 'Обязательны колонки «Наименование» и «Код ТН ВЭД» (код из номенклатуры ТН ВЭД ЕАЭС). Товар с тем же наименованием и категорией обновляется, остальные добавляются.',
  })

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    productDirectoryApi.list({ q: search, status, includeArchived: showArchived })
      .then(async (products) => {
        if (cancelled) return
        setItems(products)
        const unique = [...new Set(products.map((product) => product.code).filter(Boolean))]
        const found = unique.length ? await nomenclatureApi.lookup(unique).catch(() => null) : null
        if (!cancelled) setCodes(Object.fromEntries((found?.items ?? []).map((item) => [item.code, item])))
      })
      .catch((reason) => { if (!cancelled) setError(errorMessage(reason)) }).finally(() => { if (!cancelled) setLoading(false) })
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
        {canEdit && <div className="dir-head-actions">
          {transfer.buttons}
          <button type="button" className="dir-add" onClick={() => setPanel({})}>+ Добавить товар</button>
        </div>}
      </div>
      {canEdit && transfer.panel}
      <DirectoryToolbar query={query} placeholder="Поиск по названию, коду или синониму" status={status} showArchived={showArchived} onQuery={setQuery} onStatus={setStatus} onShowArchived={setShowArchived} />
      {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}
      <div className={loading && items.length > 0 ? 'dir-table-wrap dir-loading' : 'dir-table-wrap'}>
        <table className="sheet sheet-rownum dir-table product-table">
          <thead><tr><th>Товар</th><th className="c-tnved">Код ТН ВЭД и ставки</th><th className="c-num">ИТС, USD/кг</th><th className="c-unit">Ед.</th><th className="c-status">Статус</th><th className="c-actions" aria-label="Действия" /></tr></thead>
          <tbody>
            {items.length === 0 ? <tr><td colSpan={6} className="dir-empty">{loading ? 'Загрузка…' : filtered ? 'Ничего не найдено. Измените поиск или фильтр.' : canEdit ? 'Карточек товаров пока нет. Добавьте первую.' : 'Карточек товаров пока нет.'}</td></tr>
              : items.map((item) => <tr key={item.id} className={item.archivedAt ? 'archived' : undefined} onClick={() => setPanel({ item })}>
                <td className="c-name">
                  <button type="button" className="dir-name" onClick={(event) => { event.stopPropagation(); setPanel({ item }) }} title={item.name}>{item.name}</button>
                  {(item.nameZh || item.synonymsRu[0]) && <small>{[item.nameZh, item.synonymsRu[0] && `синоним: ${item.synonymsRu[0]}`].filter(Boolean).join(' · ')}</small>}
                </td>
                <td className="c-tnved">
                  {item.code ? <CopyCode code={item.code} /> : <span className="dir-warn">Код не указан</span>}
                  {codes[item.code] && <small>ЕТТ {codes[item.code].tariffText || 'не указана'}{codes[item.code].vatPercent && ` · НДС ${codes[item.code].vatPercent}%`}</small>}
                </td>
                <td className="c-num">{item.companyCoefficient || <span className="dir-none">—</span>}</td>
                <td className="c-unit">{item.unit}</td>
                <td className="c-status"><span className={`directory-status ${item.archivedAt ? 'archived' : item.verificationStatus}`}>{item.archivedAt ? 'В архиве' : verificationStatusLabel(item.verificationStatus)}</span></td>
                <td className="c-actions">{canEdit && <RowActions name={item.name} archived={Boolean(item.archivedAt)} onEdit={() => setPanel({ item })} onCopy={() => void copy(item)} onArchive={() => void archive(item)} onRestore={() => void restore(item)} />}</td>
              </tr>)}
          </tbody>
        </table>
      </div>
      <p className="dir-foot">Карточка не меняет исходный файл. Пошлина и НДС кода — из номенклатуры ТН ВЭД ЕАЭС на момент расчёта сметы.</p>
    </section>
    {panel && <ProductEntryPanel item={panel.item} readOnly={!canEdit} onClose={() => setPanel(null)} onSaved={() => { setPanel(null); setRefresh((value) => value + 1) }} />}
  </main>
}
