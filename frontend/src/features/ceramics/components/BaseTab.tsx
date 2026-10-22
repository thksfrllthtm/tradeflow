import { useEffect, useState } from 'react'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { errorMessage } from '@/shared/lib/errorMessage'
import { isAdmin, User } from '@/shared/types/user'
import { ceramicsApi } from '../ceramicsApi'
import { CeramicsProduct, CeramicsType } from '../types'
import { formatSize } from '../lib/sizes'
import { ProductForm } from './ProductForm'
import { BaseImport } from './BaseImport'
import { TypeCombo } from './TypeCombo'

const PAGE = 100

type Props = { user: User; types: CeramicsType[]; onBaseChanged: () => void }
type Panel = '' | 'add' | 'import' | { edit: CeramicsProduct }

// База известных марок: наименование, марка и цена. Тип и размер сервер выделяет из
// наименования: по ним подбираются цены (самая частая цена типа и размера) и список типов для ИИ.
export function BaseTab({ user, types, onBaseChanged }: Props) {
  const admin = isAdmin(user)
  const [items, setItems] = useState<CeramicsProduct[]>([])
  const [total, setTotal] = useState(0)
  const [query, setQuery] = useState('')
  const [type, setType] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [panel, setPanel] = useState<Panel>('')
  const [reloadKey, setReloadKey] = useState(0)
  const debouncedQuery = useDebouncedValue(query.trim(), 300)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    ceramicsApi.products({ q: debouncedQuery, type, limit: PAGE }).then((result) => {
      if (cancelled) return
      setItems(result.items)
      setTotal(result.total)
    }).catch((reason) => { if (!cancelled) setError(errorMessage(reason)) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [debouncedQuery, type, reloadKey])

  async function loadMore() {
    setLoading(true)
    try {
      const result = await ceramicsApi.products({ q: debouncedQuery, type, limit: PAGE, offset: items.length })
      setItems((current) => [...current, ...result.items])
      setTotal(result.total)
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setLoading(false)
    }
  }

  function changed(message: string) {
    setPanel('')
    setNotice(message)
    setReloadKey((key) => key + 1)
    onBaseChanged()
  }

  async function remove(product: CeramicsProduct) {
    if (!window.confirm(`Удалить марку ${product.mark} из базы?`)) return
    setError(''); setNotice('')
    try {
      await ceramicsApi.deleteProduct(product.id)
      changed(`Марка ${product.mark} удалена`)
    } catch (reason) {
      setError(errorMessage(reason))
    }
  }

  const togglePanel = (next: Panel) => { setNotice(''); setPanel(panel === next ? '' : next) }

  // Как таблица этапа «Результат» сметы: карточка, в ней строка фильтров и кнопок, лист в одну строку на марку.
  return <div className="cer-base">
    {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}
    {notice && <p className="banner ok" role="status">{notice}<button type="button" onClick={() => setNotice('')}>×</button></p>}

    {panel === 'add' && <ProductForm onCancel={() => setPanel('')} onSaved={(product) => changed(`Марка ${product.mark} добавлена`)} />}
    {typeof panel === 'object' && <ProductForm key={panel.edit.id} product={panel.edit} onCancel={() => setPanel('')} onSaved={(product) => changed(`Марка ${product.mark} сохранена`)} />}
    {panel === 'import' && <BaseImport onCancel={() => setPanel('')} onImported={(message) => changed(message)} />}

    <section className="cer-sheet-card" aria-label="База марок">
      <div className="cer-filterbar">
        <label className="cer-search">
          <svg width="14" height="14" viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></svg>
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Марка или наименование" aria-label="Поиск по базе" />
        </label>
        <div className="cer-type-filter">
          <TypeCombo label="Тип товара" value={type} onChange={setType} emptyLabel="Все типы" placeholder="Тип товара"
            names={types.map((item) => item.name)} counts={new Map(types.map((item) => [item.name, item.productCount]))} />
        </div>
        <p className="cer-filter-count">Показано <b>{items.length}</b> из {total}</p>
        <span className="cer-toolbar-spacer" />
        {admin && <button type="button" className="cer-secondary" onClick={() => togglePanel('import')}>Загрузить из Excel</button>}
        <button type="button" onClick={() => togglePanel('add')}>+ Добавить марку</button>
      </div>
      <div className={loading && items.length > 0 ? 'dir-table-wrap dir-loading' : 'dir-table-wrap'}>
        <table className="sheet sheet-rownum dir-table cer-table">
          <thead><tr>
            <th className="c-name">Наименование</th><th className="c-type">Тип</th><th className="c-size">Размер</th><th className="c-mark">Марка</th><th className="c-price">Цена</th><th className="c-actions" aria-label="Действия" />
          </tr></thead>
          <tbody>
            {items.length === 0 && <tr><td colSpan={6} className="dir-empty">{loading ? 'Загрузка…' : query || type ? 'Ничего не найдено.' : 'База пуста. Добавьте марки вручную или загрузите прайс из Excel (наименование, марка, цена).'}</td></tr>}
            {items.map((product) => {
              const size = formatSize(product.sizeValue && product.sizeUnit ? { value: product.sizeValue, unit: product.sizeUnit } : null)
              return <tr key={product.id} onClick={() => setPanel({ edit: product })}>
                <td className="c-name" title={product.name}>{product.name}</td>
                <td className="c-type" title={product.productType}>{product.productType}</td>
                <td className="c-size">{size || <span className="dir-none">—</span>}</td>
                <td className="c-mark">{product.mark}</td>
                <td className="c-price">{product.price}</td>
                <td className="c-actions" onClick={(event) => event.stopPropagation()}>
                  {admin && <button type="button" className="cer-link cer-delete" onClick={() => void remove(product)}>Удалить</button>}
                </td>
              </tr>
            })}
          </tbody>
        </table>
      </div>
      {items.length < total && <div className="cer-sheet-foot">
        <button type="button" className="cer-secondary" disabled={loading} onClick={() => void loadMore()}>Показать ещё {Math.min(PAGE, total - items.length)}</button>
      </div>}
    </section>
    <p className="dir-foot">В базе хранятся наименование, марка и цена. Тип и размер система берёт из наименования — по ним подбирается цена.</p>
  </div>
}
