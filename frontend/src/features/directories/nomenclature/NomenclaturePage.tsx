import { ChangeEvent, useEffect, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatDisplayNumber } from '@/shared/lib/formatDisplayNumber'
import { plural } from '@/shared/lib/plural'
import { canAddTNVED, isAdmin, User } from '@/shared/types/user'
import { formatRuDate } from '@/shared/lib/dates'
import { SearchIcon } from '@/shared/components/icons'
import { DirectoryImportResult } from '../common/directoryTransfer'
import { useTNVEDCodeCheck } from '../tnved/TNVEDCodeCheck'
import { TNVEDEntryPanel } from '../tnved/TNVEDEntryPanel'
import { TNVEDDirectoryInput } from '../tnved/types'
import { useKzListPanel } from '../kzList/KzListPanel'
import { KzImport } from '../kzList/kzListApi'
import { nomenclatureApi } from './nomenclatureApi'
import { directoryDraft } from './nomenclatureDraft'
import { AntiDumpingPanel } from './AntiDumpingPanel'
import { countryNames, formatMeasureCode, percentText } from './antiDumping'
import '@/shared/styles/list-page.css'
import '../components/DirectoryTransfer.css'
import './nomenclature.css'
import { AntiDumpingRef, NomenclatureCode, NomenclatureImport } from '@/shared/tnved/nomenclature'
import { nameParts } from '@/shared/tnved/codes'

const pageSize = 100

// Полная номенклатура ТН ВЭД ЕАЭС: поиск по коду или словам, пошлина ЕТТ и отметка,
// есть ли код в рабочем справочнике. Недостающий код добавляется в справочник с уже
// заполненными описанием и ставками. Список заменяет администратор выгрузкой TWS.BY,
// ставки ниже ЕТТ — загрузкой «Перечня РК» (PDF ЕЭК или Excel из него).
export function NomenclaturePage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const admin = isAdmin(user)
  const canAdd = canAddTNVED(user)
  const [query, setQuery] = useState('')
  const search = useDebouncedValue(query.trim(), 300)
  const [items, setItems] = useState<NomenclatureCode[]>([])
  const [total, setTotal] = useState(0)
  const [source, setSource] = useState<NomenclatureImport | null>(null)
  const [kzSource, setKzSource] = useState<KzImport | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState('')
  const [refreshKey, setRefreshKey] = useState(0)
  const [draft, setDraft] = useState<Partial<TNVEDDirectoryInput> | null>(null)
  const [uploading, setUploading] = useState(false)
  const [uploaded, setUploaded] = useState<NomenclatureImport | null>(null)
  const [rowErrors, setRowErrors] = useState<DirectoryImportResult['errors']>([])
  const [antiDumpingOpen, setAntiDumpingOpen] = useState(false)
  const [antiDumpingCount, setAntiDumpingCount] = useState<number | null>(null)

  useEffect(() => {
    let active = true
    setLoading(true); setError('')
    nomenclatureApi.list({ q: search, offset: 0, limit: pageSize })
      .then((page) => { if (active) { setItems(page.items); setTotal(page.total); setSource(page.import); setKzSource(page.kzImport) } })
      .catch((reason) => { if (active) setError(errorMessage(reason)) })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [search, refreshKey])

  async function loadMore() {
    setLoadingMore(true)
    try {
      const page = await nomenclatureApi.list({ q: search, offset: items.length, limit: pageSize })
      setItems((current) => [...current, ...page.items]); setTotal(page.total)
    } catch (reason) { setError(errorMessage(reason)) } finally { setLoadingMore(false) }
  }

  async function upload(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    setUploading(true); setError(''); setUploaded(null); setRowErrors([])
    try {
      setUploaded(await nomenclatureApi.upload(file))
      setRefreshKey((value) => value + 1)
    } catch (reason) {
      setError(errorMessage(reason))
      setRowErrors((reason as { rows?: DirectoryImportResult['errors'] }).rows ?? [])
    } finally { setUploading(false) }
  }

  // Проверка по номенклатуре: есть ли код в ТН ВЭД ЕАЭС, со ставками; справочник — на своей странице.
  const codeCheck = useTNVEDCodeCheck({ mode: 'nomenclature', canAdd, refreshKey, onAdd: setDraft })
  const kzList = useKzListPanel({ current: kzSource, onImported: () => setRefreshKey((value) => value + 1) })
  const empty = !loading && total === 0
  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="nomenclature" onLogout={onLogout} />
    <section className="directory-page">
      <div className="dir-head">
        <div>
          <h1>Номенклатура ТН ВЭД ЕАЭС{source && <span className="dir-count">{formatDisplayNumber(source.codeCount)}</span>}</h1>
          <p>{source ? <>Источник: {source.source}{source.actualDate && <> · данные на {formatRuDate(source.actualDate)}</>} · загружено {formatRuDate(source.importedAt.slice(0, 10))}{source.importedByName && `, ${source.importedByName}`}</> : 'Все коды со ставками ввозной пошлины Единого таможенного тарифа.'}</p>
        </div>
        <div className="dir-head-actions dir-transfer">
          {codeCheck.button}
          <button type="button" className={antiDumpingOpen ? 'dir-transfer-button active' : 'dir-transfer-button'} onClick={() => setAntiDumpingOpen((value) => !value)} aria-expanded={antiDumpingOpen}>Антидемпинг ЕАЭС{antiDumpingCount !== null && ` · ${antiDumpingCount}`}</button>
          {admin && kzList.button}
          {admin && <label className={uploading ? 'dir-transfer-button nom-upload busy' : 'dir-transfer-button nom-upload'} title="Excel с сайта tws.by/tws/tnved/download">
            {uploading ? 'Загружаем…' : source ? 'Обновить из файла TWS' : 'Загрузить файл TWS'}
            <input type="file" accept=".xlsx" onChange={(event) => void upload(event)} disabled={uploading} />
          </label>}
        </div>
      </div>

      {codeCheck.panel}
      {antiDumpingOpen && <AntiDumpingPanel admin={admin} onLoaded={setAntiDumpingCount} onRefreshed={() => setRefreshKey((value) => value + 1)}
        onPickCode={(code) => { setQuery(formatMeasureCode(code)); window.scrollTo({ top: 0, behavior: 'smooth' }) }} />}
      {admin && kzList.panel}
      {uploaded && <p className="dir-transfer-ok nom-note" role="status">
        Загружено {formatDisplayNumber(uploaded.codeCount)} {plural(uploaded.codeCount, 'код', 'кода', 'кодов')}: новых {formatDisplayNumber(uploaded.addedCount)}, удалено {formatDisplayNumber(uploaded.removedCount)}, изменились наименование или ставка — {formatDisplayNumber(uploaded.changedCount)}.
        <button type="button" onClick={() => setUploaded(null)} aria-label="Закрыть">×</button>
      </p>}
      {error && <p className="dir-transfer-error nom-note" role="alert">{error}<button type="button" onClick={() => { setError(''); setRowErrors([]) }} aria-label="Закрыть">×</button></p>}
      {rowErrors.length > 0 && <ul className="dir-transfer-errors">{rowErrors.map((item) => <li key={`${item.row}-${item.message}`}><b>Строка {item.row}</b> {item.message}</li>)}</ul>}

      <div className="dir-toolbar">
        <label className="dir-search">
          <SearchIcon />
          <input type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Начало кода (4015 19) или слова из наименования (перчатки резиновые)" aria-label="Поиск по номенклатуре" autoFocus />
        </label>
        {!loading && source && <span className="nom-found">{search ? `Найдено: ${formatDisplayNumber(total)}` : `Всего: ${formatDisplayNumber(total)}`}</span>}
      </div>

      <div className={loading && items.length > 0 ? 'dir-table-wrap dir-loading' : 'dir-table-wrap'}>
        <table className="dir-table nom-table">
          <thead><tr><th className="c-code">Код ТН ВЭД</th><th>Наименование</th><th className="nom-rate">Пошлина ЕТТ</th><th className="nom-kz" title="Ставка ниже ЕТТ по обязательствам Казахстана в ВТО">Ставка РК</th><th className="nom-vat">НДС</th><th className="nom-ad" title="Антидемпинговая пошлина ЕАЭС сверх ЕТТ, % для прочих производителей">Антидемпинг</th><th className="nom-link" aria-label="Подробнее" /><th className="nom-state">В справочнике</th></tr></thead>
          <tbody>
            {items.length === 0 ? <tr><td colSpan={8} className="dir-empty">{loading ? 'Загрузка…'
              : !source ? admin ? <>Номенклатура ещё не загружена. Скачайте Excel на <a href="https://www.tws.by/tws/tnved/download" target="_blank" rel="noreferrer">tws.by</a> и нажмите «Загрузить файл TWS».</> : 'Номенклатура ещё не загружена администратором.'
              : empty ? 'Ничего не найдено. Проверьте код или попробуйте другие слова.' : ''}</td></tr>
              : items.map((item) => {
                const parts = nameParts(item.name)
                return <tr key={item.code}>
                  <td className="c-code"><code>{item.code}</code></td>
                  <td className="nom-name" title={parts.join(' ')}>
                    {parts.length > 1 && <small>{parts.slice(0, -1).join(' ')}</small>}
                    <span>{parts[parts.length - 1]}</span>
                  </td>
                  <td className="nom-rate">{item.tariffText || <span className="dir-none">не указана</span>}</td>
                  <td className={item.kz?.active ? 'nom-kz active' : 'nom-kz'} title={item.kz?.notesText || undefined}>{item.kz ? <>{item.kz.rateText}<small>{item.kz.status}</small></> : <span className="dir-none">{kzSource ? '—' : ''}</span>}</td>
                  <td className="nom-vat">{item.vatPercent ? `${item.vatPercent}%` : <span className="dir-none">—</span>}</td>
                  <td className="nom-ad">{item.antiDumping?.length ? item.antiDumping.map((ref) => <AntiDumpingCell key={ref.number} measure={ref} />) : <span className="dir-none">—</span>}</td>
                  <td className="nom-link">{item.detailsUrl && <a href={item.detailsUrl} target="_blank" rel="noreferrer" title={`Код ${item.code} на ${source?.source ?? 'сайте источника'}: примечания, документы, меры`}>Подробнее</a>}</td>
                  <td className="nom-state">
                    {item.inDirectory ? <span className="nom-in">Есть</span>
                      : canAdd ? <button type="button" className="nom-add" onClick={() => setDraft(directoryDraft(item, source, kzSource))}>Добавить</button>
                      : <span className="dir-none">Нет</span>}
                  </td>
                </tr>
              })}
          </tbody>
        </table>
        {items.length < total && <div className="nom-more">
          <span>Показано {formatDisplayNumber(items.length)} из {formatDisplayNumber(total)}</span>
          <button type="button" className="dir-transfer-button" onClick={() => void loadMore()} disabled={loadingMore}>{loadingMore ? 'Загружаем…' : `Показать ещё ${Math.min(pageSize, total - items.length)}`}</button>
        </div>}
      </div>
      <p className="dir-foot">«Пошлина ЕТТ» — Единый таможенный тариф ЕАЭС. «Ставка РК» — ставка ниже ЕТТ по обязательствам Казахстана в ВТО (<a href="https://eec.eaeunion.org/comission/department/catr/ttr/stavki.php" target="_blank" rel="noreferrer">перечень РК</a>); «Добавить» берёт её, если она сегодня действует. НДС 16% — основная ставка Казахстана; 5% для лекарств и медицинских изделий или освобождение задаются в справочнике вручную.</p>
    </section>
    {draft && <TNVEDEntryPanel draft={draft} canVerify={admin} onClose={() => setDraft(null)} onSaved={() => { setDraft(null); setRefreshKey((value) => value + 1) }} />}
  </main>
}

// Антидемпинг кода в таблице: ставка для прочих производителей и страны; мера на часть товаров
// позиции — «возможно», временно не применяемая — серым. Нажатие открывает решение ЕЭК.
function AntiDumpingCell({ measure }: { measure: AntiDumpingRef }) {
  const rates = [...new Set(measure.countries.map((country) => measure.rates[country]).filter(Boolean))].sort((a, b) => Number(a) - Number(b))
  const value = rates.length <= 1 ? percentText(rates[0] ?? '') : `${percentText(rates[0])}–${percentText(rates[rates.length - 1])}`
  const title = [
    `${measure.number} «${measure.shortName}»: ${measure.countries.map((country) => `${countryNames[country] ?? country} ${percentText(measure.rates[country] ?? '')}`).join(', ')}`,
    measure.endDate && `действует по ${formatRuDate(measure.endDate)}`,
    !measure.exact && 'мера на часть товаров позиции — сверьте описание товара с решением ЕЭК',
    measure.suspended && 'временно не применяется',
  ].filter(Boolean).join(' · ')
  const className = measure.suspended ? 'nom-ad-chip off' : measure.exact ? 'nom-ad-chip' : 'nom-ad-chip maybe'
  const body = <><b>{measure.exact ? value : 'возможно'}</b><small>{measure.suspended ? 'не применяется' : measure.countryNames}</small></>
  return measure.documentUrl
    ? <a className={className} href={measure.documentUrl} target="_blank" rel="noreferrer" title={title}>{body}</a>
    : <span className={className} title={title}>{body}</span>
}
