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
import './productDirectory.css'
import { PageHead } from '@/shared/components/PageHead/PageHead'

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
  // Как «База решений» и этапы сметы: страница во всю ширину, таблица — карточка до низа окна со своей
  // прокруткой, поиск и фильтры — в её верхней полосе, строки — в одну линию.
  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="products" onLogout={onLogout} />
    <section className="directory-page pd-page">
      <PageHead title="Справочник товаров" subtitle="Карточки товаров с кодом ТН ВЭД и ИТС. При выборе карточки в смете всё подставляется автоматически."
        actions={canEdit && <>
          {transfer.buttons}
          <button type="button" className="dir-add" onClick={() => setPanel({})}>+ Добавить товар</button>
        </>} />
      {canEdit && transfer.panel}
      {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}
      <section className="pd-card" aria-label="Карточки товаров">
        <DirectoryToolbar query={query} placeholder="Название, код или синоним" status={status} showArchived={showArchived} onQuery={setQuery} onStatus={setStatus} onShowArchived={setShowArchived}
          count={loading ? undefined : `${filtered ? 'Найдено' : 'Всего'}: ${items.length}`} />
        <div className={loading && items.length > 0 ? 'dir-table-wrap dir-loading' : 'dir-table-wrap'}>
          <table className="sheet sheet-rownum dir-table product-table">
            <thead><tr>
              <th className="c-name">Товар</th><th className="c-code">Код ТН ВЭД</th><th className="c-rates">Ставки</th><th className="c-num">ИТС, USD/кг</th>
              <th className="c-unit">Ед.</th><th className="c-status">Статус</th><th className="c-actions" aria-label="Действия" />
            </tr></thead>
            <tbody>
              {items.length === 0 ? <tr><td colSpan={7} className="dir-empty">{loading ? 'Загрузка…' : filtered ? 'Ничего не найдено. Измените поиск или фильтр.' : canEdit ? 'Карточек товаров пока нет. Добавьте первую.' : 'Карточек товаров пока нет.'}</td></tr>
                : items.map((item) => {
                  const extra = [item.nameZh, item.synonymsRu[0] && `синоним: ${item.synonymsRu[0]}`].filter(Boolean).join(' · ')
                  const rates = codes[item.code]
                  return <tr key={item.id} className={item.archivedAt ? 'archived' : undefined} onClick={() => setPanel({ item })}>
                    <td className="c-name" title={[item.name, extra].filter(Boolean).join('\n')}>
                      <span className="pd-name">{item.name}</span>{extra && <small> {extra}</small>}
                    </td>
                    <td className="c-code">{item.code ? <CopyCode code={item.code} /> : <span className="dir-warn">Код не указан</span>}</td>
                    <td className="c-rates">{rates ? <>ЕТТ {rates.tariffText || '—'}{rates.vatPercent && ` · НДС ${rates.vatPercent}%`}</> : <span className="dir-none">—</span>}</td>
                    <td className="c-num">{item.companyCoefficient || <span className="dir-none">—</span>}</td>
                    <td className="c-unit">{item.unit}</td>
                    <td className="c-status"><span className={`status-dot directory-status ${item.archivedAt ? '' : item.verificationStatus === 'operator_verified' ? 'ok' : 'warn'}`} title={item.archivedAt ? undefined : verificationStatusLabel(item.verificationStatus)}>
                      {item.archivedAt ? 'В архиве' : item.verificationStatus === 'operator_verified' ? 'Проверено' : 'Проверить'}
                    </span></td>
                    <td className="c-actions">{canEdit && <RowActions name={item.name} archived={Boolean(item.archivedAt)} onEdit={() => setPanel({ item })} onCopy={() => void copy(item)} onArchive={() => void archive(item)} onRestore={() => void restore(item)} />}</td>
                  </tr>
                })}
            </tbody>
          </table>
        </div>
      </section>
      <p className="pd-disclaimer">
        <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"><circle cx="8" cy="8" r="6.3" /><path d="M8 7.3v3.6" /><circle cx="8" cy="5" r=".4" fill="currentColor" /></svg>
        <span>Карточка не меняет исходный файл. Пошлина и НДС кода — из номенклатуры ТН ВЭД ЕАЭС на момент расчёта сметы.</span>
      </p>
    </section>
    {panel && <ProductEntryPanel item={panel.item} readOnly={!canEdit} onClose={() => setPanel(null)} onSaved={() => { setPanel(null); setRefresh((value) => value + 1) }} />}
  </main>
}
