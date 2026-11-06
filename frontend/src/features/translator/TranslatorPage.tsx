import { useEffect, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { PageHead } from '@/shared/components/PageHead/PageHead'
import { SearchIcon } from '@/shared/components/icons'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { formatRuDate } from '@/shared/lib/dates'
import { errorMessage } from '@/shared/lib/errorMessage'
import { User } from '@/shared/types/user'
import { Link } from '@/shared/router/Link'
import { navigate, paths } from '@/shared/router/router'
import { documentStatusLabels, documentStatusTone, languageLabel, TranslatorDocument, translatorApi } from './translatorApi'
import { UploadDrawer } from './UploadDrawer'
import '@/shared/styles/list-page.css'
import './translator.css'

// «Переводчик»: список документов Word и Excel всех сотрудников с прогрессом подтверждения перевода.
export function TranslatorPage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [query, setQuery] = useState('')
  const search = useDebouncedValue(query.trim(), 300)
  const [items, setItems] = useState<TranslatorDocument[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [uploadOpen, setUploadOpen] = useState(false)

  useEffect(() => {
    let active = true
    setLoading(true); setError('')
    translatorApi.list(search)
      .then((list) => { if (active) setItems(list) })
      .catch((reason) => { if (active) setError(errorMessage(reason)) })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [search])

  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="translator" onLogout={onLogout} />
    <section className="directory-page tr-page">
      <PageHead title="Переводчик" count={items.length > 0 ? items.length : undefined}
        subtitle="Перевод документов Word (.docx) и книг Excel (.xlsx) по абзацам и строкам: правка на сайте, выгрузка файла с исходным форматированием."
        actions={<button type="button" className="dir-add" onClick={() => setUploadOpen(true)}>Загрузить документ</button>} />
      {error && <p className="dir-transfer-error" role="alert">{error}</p>}
      <div className="dir-toolbar">
        <label className="dir-search">
          <SearchIcon />
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Поиск по названию" aria-label="Поиск документов" />
        </label>
      </div>
      <div className={loading ? 'dir-table-wrap dir-loading' : 'dir-table-wrap'}>
        <table className="sheet sheet-rownum dir-table tr-table">
          <thead><tr><th>Документ</th><th className="tr-langs-col">Языки</th><th className="tr-who-col">Загрузил</th><th className="tr-progress-col">Подтверждено</th><th className="tr-status-col">Статус</th></tr></thead>
          <tbody>
            {items.length === 0 ? <tr><td colSpan={5} className="dir-empty">{loading ? 'Загрузка…' : search ? 'Ничего не найдено' : 'Документов пока нет. Загрузите .docx или .xlsx, чтобы перевести.'}</td></tr>
              : items.map((item) => <tr key={item.id} onClick={() => navigate(paths.translator(item.id))}>
                <td><Link className="dir-name" href={paths.translator(item.id)} onClick={(event) => event.stopPropagation()}>{item.name}<small className="tr-ext">.{item.format}</small></Link></td>
                <td className="tr-langs-col">{languageLabel(item.sourceLang)} → {languageLabel(item.targetLang)}</td>
                <td className="tr-who-col"><span>{item.createdByName || '—'}</span><small><time dateTime={item.createdAt}>{formatRuDate(item.createdAt.slice(0, 10))}</time></small></td>
                <td className="tr-progress-col">
                  <span className="tr-progress" aria-hidden="true"><span style={{ width: `${item.segmentCount ? (item.confirmedCount / item.segmentCount) * 100 : 0}%` }} /></span>
                  <small>{item.confirmedCount} из {item.segmentCount}</small>
                </td>
                <td className="tr-status-col"><span className={`badge tr-status ${documentStatusTone[item.status]}`}>{documentStatusLabels[item.status]}</span></td>
              </tr>)}
          </tbody>
        </table>
      </div>
    </section>
    {uploadOpen && <UploadDrawer onClose={() => setUploadOpen(false)} onUploaded={(id) => navigate(paths.translator(id))} />}
  </main>
}
