import { useEffect, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { User } from '@/shared/types/user'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { verificationStatusLabel } from '../directoryLabels'
import { DirectoryToolbar } from '../components/DirectoryToolbar'
import { RowActions } from '../components/RowActions'
import { TNVEDEntryPanel } from './TNVEDEntryPanel'
import { tnvedDirectoryApi } from './tnvedDirectoryApi'
import { entrySubline, rateLabel } from './tnvedLabels'
import { TNVEDDirectoryEntry } from './types'
import '../directory.css'
import '../verificationStatus.css'

export function TNVEDDirectoryPage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [items, setItems] = useState<TNVEDDirectoryEntry[]>([])
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('all')
  const [showArchived, setShowArchived] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [panel, setPanel] = useState<{ item?: TNVEDDirectoryEntry } | null>(null)
  const [refreshKey, setRefreshKey] = useState(0)
  const search = useDebouncedValue(query.trim(), 250)

  useEffect(() => {
    let cancelled = false
    setLoading(true); setError('')
    tnvedDirectoryApi.list({ q: search, status, includeArchived: showArchived }).then((result) => { if (!cancelled) setItems(result) }).catch((reason) => { if (!cancelled) setError(errorMessage(reason)) }).finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [search, status, showArchived, refreshKey])

  async function archive(item: TNVEDDirectoryEntry) {
    if (!window.confirm(`Перенести в архив код ${item.code}? Он останется в истории и пропадёт из обычного поиска.`)) return
    try { await tnvedDirectoryApi.archive(item.id); setRefreshKey((value) => value + 1) } catch (reason) { setError(errorMessage(reason)) }
  }

  async function restore(item: TNVEDDirectoryEntry) {
    try { await tnvedDirectoryApi.restore(item.id); setRefreshKey((value) => value + 1) } catch (reason) { setError(errorMessage(reason)) }
  }

  async function copy(item: TNVEDDirectoryEntry) {
    if (!window.confirm(`Создать копию кода ${item.code}?`)) return
    try { await tnvedDirectoryApi.copy(item.id); setRefreshKey((value) => value + 1) } catch (reason) { setError(errorMessage(reason)) }
  }

  const filtered = search !== '' || status !== 'all'
  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="tnved" onLogout={onLogout} />
    <section className="directory-page">
      <div className="dir-head">
        <div>
          <h1>Справочник ТН ВЭД{!loading && <span className="dir-count">{items.length}</span>}</h1>
          <p>Коды и ставки для предварительного расчёта. Ручные записи — не официальное подтверждение.</p>
        </div>
        <button type="button" className="dir-add" onClick={() => setPanel({})}>+ Добавить код</button>
      </div>
      <DirectoryToolbar query={query} placeholder="Поиск по коду, описанию или синониму" status={status} showArchived={showArchived} onQuery={setQuery} onStatus={setStatus} onShowArchived={setShowArchived} />
      {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}
      <div className={loading && items.length > 0 ? 'dir-table-wrap dir-loading' : 'dir-table-wrap'}>
        <table className="dir-table tnved-table">
          <thead><tr><th className="c-code">Код ТН ВЭД</th><th>Описание</th><th className="c-rate">Пошлина</th><th className="c-rate">НДС</th><th className="c-rate">Антидемпинг</th><th className="c-status">Статус</th><th className="c-actions" aria-label="Действия" /></tr></thead>
          <tbody>
            {items.length === 0 ? <tr><td colSpan={7} className="dir-empty">{loading ? 'Загрузка…' : filtered ? 'Ничего не найдено. Измените поиск или фильтр.' : 'Кодов пока нет. Добавьте первый.'}</td></tr>
              : items.map((item) => <tr key={item.id} className={item.archivedAt ? 'archived' : undefined} onClick={() => setPanel({ item })}>
                <td className="c-code"><code>{item.code}</code></td>
                <td className="c-name">
                  <button type="button" className="dir-name" onClick={(event) => { event.stopPropagation(); setPanel({ item }) }} title={item.name}>{item.name || 'Без описания'}</button>
                  {entrySubline(item) && <small>{entrySubline(item)}</small>}
                </td>
                <td className="c-rate">{rateLabel(item.duty, 'Пошлина')}</td>
                <td className="c-rate">{rateLabel(item.vat, 'НДС')}</td>
                <td className="c-rate">{item.antiDumpingPercent ? `${item.antiDumpingPercent}%` : <span className="dir-none">—</span>}</td>
                <td className="c-status"><span className={`directory-status ${item.archivedAt ? 'archived' : item.verificationStatus}`}>{item.archivedAt ? 'В архиве' : verificationStatusLabel(item.verificationStatus)}</span></td>
                <td className="c-actions"><RowActions name={item.code} archived={Boolean(item.archivedAt)} onEdit={() => setPanel({ item })} onCopy={() => void copy(item)} onArchive={() => void archive(item)} onRestore={() => void restore(item)} /></td>
              </tr>)}
          </tbody>
        </table>
      </div>
      <p className="dir-foot">Код проверяется только по формату 10 цифр. Существование кода, актуальность ставки и официальные пояснения нужно проверить отдельно.</p>
    </section>
    {panel && <TNVEDEntryPanel item={panel.item} onClose={() => setPanel(null)} onSaved={() => { setPanel(null); setRefreshKey((value) => value + 1) }} />}
  </main>
}
