import { useEffect, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { SearchIcon } from '@/shared/components/icons'
import { errorMessage } from '@/shared/lib/errorMessage'
import { User } from '@/shared/types/user'
import { formatCode, nameParts } from '@/shared/tnved/codes'
import { AgentResult } from '@/features/goods/agent/agentApi'
import { DocumentsAnswer, documentKindLabels } from '@/features/goods/agent/DocumentsAnswer'
import { CachedPage, DocumentsBaseEntry, DocumentsBaseItem, DocumentsBaseSummary, documentsBaseApi } from './documentsBaseApi'
import { Drawer } from '@/shared/components/Drawer/Drawer'
import { PageHead } from '@/shared/components/PageHead/PageHead'
import '@/features/agentLog/agentLog.css'
import './documentsBase.css'

type Tab = 'codes' | 'acts'

const dateTime = (value: string | null) => value ? new Date(value).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—'
const When = ({ value }: { value: string | null }) => <time dateTime={value ?? undefined}>{dateTime(value)}</time>
const sizeText = (size: number) => size >= 1_000_000 ? `${(size / 1_000_000).toFixed(1)} млн знаков` : size >= 1000 ? `${Math.round(size / 1000)} тыс. знаков` : `${size} знаков`

// База документов: что программа уже собрала по «Документам для ввоза». Ответы по кодам (последний и все
// прошлые версии), справка tnved.info по коду, акты, на которые ссылаются ответы, и все акты в кэше.
// Только для администратора: сервер проверяет права сам, приложение не показывает страницу остальным.
export function DocumentsBasePage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [tab, setTab] = useState<Tab>('codes')
  const [query, setQuery] = useState('')
  const [summary, setSummary] = useState<DocumentsBaseSummary | null>(null)
  const [items, setItems] = useState<DocumentsBaseItem[] | null>(null)
  const [acts, setActs] = useState<CachedPage[] | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [openCode, setOpenCode] = useState('')

  useEffect(() => {
    let active = true
    const timer = window.setTimeout(() => {
      setLoading(true)
      const load = tab === 'codes'
        ? documentsBaseApi.list(query).then((page) => { if (active) { setSummary(page.summary); setItems(page.items ?? []) } })
        : documentsBaseApi.sources(query).then((page) => { if (active) setActs(page.items ?? []) })
      load.catch((reason) => { if (active) setError(errorMessage(reason)) }).finally(() => { if (active) setLoading(false) })
    }, 250)
    return () => { active = false; window.clearTimeout(timer) }
  }, [tab, query])

  useEffect(() => {
    if (summary || tab === 'codes') return
    documentsBaseApi.list('').then((page) => setSummary(page.summary)).catch(() => undefined)
  }, [summary, tab])

  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="documentsBase" onLogout={onLogout} />
    <section className="directory-page alog dbase">
      <PageHead title="База документов" subtitle="Что программа собрала по «Документам для ввоза»: ответы по кодам со всеми версиями, справки tnved.info и тексты актов, которые читали агенты." />
      {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}
      {summary && <div className="alog-summary dbase-summary">
        <Stat label="Кодов с ответом" value={summary.codes} />
        <Stat label="Ответов сохранено" value={summary.answers} note="с прошлыми версиями" />
        <Stat label="Справок tnved.info" value={summary.helps} />
        <Stat label="Актов в кэше" value={summary.acts} />
      </div>}
      <div className="alog-card">
        <div className="alog-filterbar">
          <div className="dir-seg" role="tablist" aria-label="Раздел">
            <button type="button" role="tab" className={tab === 'codes' ? 'active' : undefined} aria-selected={tab === 'codes'} onClick={() => setTab('codes')}>Ответы по кодам</button>
            <button type="button" role="tab" className={tab === 'acts' ? 'active' : undefined} aria-selected={tab === 'acts'} onClick={() => setTab('acts')}>Акты в кэше</button>
          </div>
          <label className="dir-search dbase-search">
            <SearchIcon />
            <input type="search" value={query} onChange={(event) => setQuery(event.target.value)}
              placeholder={tab === 'codes' ? 'Код (начало) или товар' : 'Название акта или адрес'} aria-label="Поиск" />
          </label>
          {loading && <span className="alog-loading" role="status">Загрузка…</span>}
        </div>
        <div className={loading ? 'dir-table-wrap dir-loading' : 'dir-table-wrap'}>
          {tab === 'codes' ? <CodesTable items={items} onOpen={setOpenCode} /> : <ActsTable acts={acts} />}
        </div>
      </div>
    </section>
    {openCode && <CodePanel code={openCode} onClose={() => setOpenCode('')} />}
  </main>
}

function Stat({ label, value, note }: { label: string; value: number; note?: string }) {
  return <div className="alog-stat"><small>{label}</small><strong>{value.toLocaleString('ru-RU')}</strong>{note && <span>{note}</span>}</div>
}

function CodesTable({ items, onOpen }: { items: DocumentsBaseItem[] | null; onOpen: (code: string) => void }) {
  return <table className="sheet dir-table dbase-table">
    <thead><tr>
      <th className="c-code">Код</th><th className="c-name">Товар</th><th className="c-measures">Меры</th>
      <th className="c-kinds">Документы</th><th className="c-versions">Версий</th><th className="c-when">Сохранён</th>
    </tr></thead>
    <tbody>
      {!items ? <tr><td colSpan={6} className="dbase-empty">Загрузка…</td></tr>
        : items.length === 0 ? <tr><td colSpan={6} className="dbase-empty">Ответов «Документы для ввоза» не найдено.</td></tr>
        : items.map((item) => <tr key={item.code} tabIndex={0} onClick={() => onOpen(item.code)} onKeyDown={(event) => { if (event.key === 'Enter') onOpen(item.code) }}>
          <td className="c-code"><code>{formatCode(item.code)}</code></td>
          <td className="c-name"><span title={item.product}>{item.product || '—'}</span></td>
          <td className="c-measures"><span className="dbase-counts">
            <b className="badge ok" title="Нужно">{item.yes}</b><b className="badge warn" title="Возможно">{item.maybe}</b><b className="badge" title="Не относится">{item.no}</b>
          </span></td>
          <td className="c-kinds">{item.kinds.length > 0 ? item.kinds.map((kind) => <i key={kind} className="badge dbase-kind">{documentKindLabels[kind] || kind}</i>) : <span className="alog-none">—</span>}</td>
          <td className="c-versions">{item.versions}</td>
          <td className="c-when"><When value={item.savedAt} /><small>{item.author || '—'}</small></td>
        </tr>)}
    </tbody>
  </table>
}

function ActsTable({ acts }: { acts: CachedPage[] | null }) {
  return <table className="sheet dir-table dbase-table">
    <thead><tr><th className="c-title">Документ</th><th className="c-when">Получен</th><th className="c-size">Объём</th></tr></thead>
    <tbody>
      {!acts ? <tr><td colSpan={3} className="dbase-empty">Загрузка…</td></tr>
        : acts.length === 0 ? <tr><td colSpan={3} className="dbase-empty">Актов в кэше не найдено.</td></tr>
        : acts.map((act) => <tr key={act.url} className="dbase-static">
          <td className="c-title"><span>{act.title || act.url}</span><a href={act.url} target="_blank" rel="noreferrer">{act.url}</a></td>
          <td className="c-when"><When value={act.fetchedAt} /></td>
          <td className="c-size">{sizeText(act.size)}</td>
        </tr>)}
    </tbody>
  </table>
}

// Карточка кода: выбранная версия ответа целиком (как в чате), справка tnved.info и акты из ответов.
function CodePanel({ code, onClose }: { code: string; onClose: () => void }) {
  const [entry, setEntry] = useState<DocumentsBaseEntry | null>(null)
  const [version, setVersion] = useState(0)
  const [error, setError] = useState('')
  useEffect(() => {
    let active = true
    documentsBaseApi.code(code).then((value) => { if (active) setEntry(value) }).catch((reason) => { if (active) setError(errorMessage(reason)) })
    return () => { active = false }
  }, [code])
  const shown = entry?.versions[version]
  const answer: AgentResult | null = shown ? {
    description: shown.description, documentsCode: code, documents: shown.documents, candidates: [], name: '', notes: '', droppedCodes: [], quotesRemoved: 0,
  } : null
  const name = entry?.name ? nameParts(entry.name).at(-1) : ''
  return <Drawer className="dbase-entry" title={<code>{formatCode(code)}</code>} subtitle={name} onClose={onClose}>
        {error && <p className="banner error" role="alert">{error}</p>}
        {!entry && !error && <p className="dbase-empty">Загрузка…</p>}
        {entry && <>
          {entry.versions.length > 1 && <div className="dbase-versions">
            <span>Версии</span>
            <div className="dir-seg" role="group" aria-label="Версии ответа">{entry.versions.map((item, index) => <button key={item.id} type="button" className={index === version ? 'active' : undefined} aria-pressed={index === version} onClick={() => setVersion(index)}>
              <When value={item.savedAt} />{index === 0 && ' · последняя'}
            </button>)}</div>
          </div>}
          {shown && <p className="dbase-product"><b>Товар:</b> {shown.product || '—'}<small><When value={shown.savedAt} /> · {shown.author || '—'}{shown.sessionId ? ` · подбор №${shown.sessionId}` : ''}</small></p>}
          {answer ? <DocumentsAnswer result={answer} /> : <p className="dbase-empty">По этому коду ответов ещё нет.</p>}

          <section className="dbase-block">
            <h3>Акты из ответов</h3>
            {entry.acts.length === 0 ? <p className="dbase-muted">Ссылок на акты в ответах нет.</p>
              : <ul className="dbase-acts">{entry.acts.map((act) => <li key={act.url}>
                <a href={act.url} target="_blank" rel="noreferrer">{act.title || act.url}</a>
                <small>{act.cached ? <>в кэше с <When value={act.fetchedAt} /> · {sizeText(act.size)}</> : 'текста в кэше нет'}</small>
              </li>)}</ul>}
          </section>

          <section className="dbase-block">
            <h3>Справка tnved.info</h3>
            {entry.help ? <details className="dbase-help">
              <summary>Получена <When value={entry.help.fetchedAt} /> · {sizeText(entry.help.text.length)} <a href={entry.help.url} target="_blank" rel="noreferrer" onClick={(event) => event.stopPropagation()}>на сайте ↗</a></summary>
              <pre>{entry.help.text}</pre>
            </details> : <p className="dbase-muted">Справки по коду в кэше нет.</p>}
          </section>
        </>}
  </Drawer>
}
