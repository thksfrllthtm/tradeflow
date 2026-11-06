import { Fragment, useEffect, useState } from 'react'
import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatDisplayNumber } from '@/shared/lib/formatDisplayNumber'
import { User } from '@/shared/types/user'
import { formatRuDate } from '@/shared/lib/dates'
import { SearchIcon } from '@/shared/components/icons'
import { KzImport } from '../kzList/kzListApi'
import { useRiskIndicatorsPanel } from '../riskIndicators/RiskIndicatorsPanel'
import { nomenclatureApi } from './nomenclatureApi'
import { useCodeCheck } from './CodeCheck'
import { AntiDumpingPanel } from './AntiDumpingPanel'
import { countryNames, formatMeasureCode, percentText } from './antiDumping'
import '@/shared/styles/list-page.css'
import '../components/DirectoryTransfer.css'
import './nomenclature.css'
import { PageHead } from '@/shared/components/PageHead/PageHead'
import { AntiDumpingRef, NomenclatureCode, NomenclatureImport } from '@/shared/tnved/nomenclature'
import { nameParts } from '@/shared/tnved/codes'

const pageSize = 100

// Полная номенклатура ТН ВЭД ЕАЭС: поиск по коду или словам, пошлина ЕТТ, ставка РК, НДС,
// антидемпинг и СУР. Загружает и обновляет все эти данные администратор на странице «Обновление данных».
export function NomenclaturePage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [query, setQuery] = useState('')
  const search = useDebouncedValue(query.trim(), 300)
  const [items, setItems] = useState<NomenclatureCode[]>([])
  const [total, setTotal] = useState(0)
  const [source, setSource] = useState<NomenclatureImport | null>(null)
  const [kzSource, setKzSource] = useState<KzImport | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [error, setError] = useState('')
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
  }, [search])

  async function loadMore() {
    setLoadingMore(true)
    try {
      const page = await nomenclatureApi.list({ q: search, offset: items.length, limit: pageSize })
      setItems((current) => [...current, ...page.items]); setTotal(page.total)
    } catch (reason) { setError(errorMessage(reason)) } finally { setLoadingMore(false) }
  }

  // Проверка вставленной колонки кодов; с главной открывается сразу (?check=1).
  // Панели в шапке переключаются: открыта только одна.
  function closePanels() { codeCheck.close(); setAntiDumpingOpen(false); riskIndicators.close() }
  const codeCheck = useCodeCheck({ initialOpen: new URLSearchParams(window.location.search).has('check'), onOpen: closePanels })
  const riskIndicators = useRiskIndicatorsPanel({ onOpen: closePanels })
  const empty = !loading && total === 0
  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="nomenclature" onLogout={onLogout} />
    <section className="directory-page nom-page">
      <PageHead title="Номенклатура ТН ВЭД ЕАЭС"
        subtitle={source ? <>Источник: {source.source}{source.actualDate && <> · данные на <time dateTime={source.actualDate.slice(0, 10)}>{formatRuDate(source.actualDate)}</time></>} · загружено <time dateTime={source.importedAt}>{formatRuDate(source.importedAt.slice(0, 10))}</time>{source.importedByName && `, ${source.importedByName}`}</> : 'Все коды со ставками ввозной пошлины Единого таможенного тарифа.'}
        actions={<>
          {codeCheck.button}
          <button type="button" className={antiDumpingOpen ? 'secondary active' : 'secondary'} onClick={() => { if (!antiDumpingOpen) closePanels(); setAntiDumpingOpen(!antiDumpingOpen) }} aria-expanded={antiDumpingOpen}>Антидемпинг ЕАЭС{antiDumpingCount !== null && ` · ${antiDumpingCount}`}</button>
          {riskIndicators.button}
        </>} />

      {codeCheck.panel}
      {antiDumpingOpen && <AntiDumpingPanel onLoaded={setAntiDumpingCount}
        onPickCode={(code) => { setQuery(formatMeasureCode(code)); window.scrollTo({ top: 0, behavior: 'smooth' }) }} />}
      {riskIndicators.panel}
      {error && <p className="dir-transfer-error nom-note" role="alert">{error}<button type="button" onClick={() => setError('')} aria-label="Закрыть">×</button></p>}

      {/* Как «База решений» и этапы сметы: карточка до низа окна, в ней поиск, таблица и «Показать ещё». */}
      <section className="nom-card" aria-label="Коды ТН ВЭД">
      <div className="nom-filterbar">
        <label className="dir-search">
          <SearchIcon />
          <input type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Начало кода (4015 19) или слова из наименования (перчатки резиновые)" aria-label="Поиск по номенклатуре" autoFocus />
        </label>
        {!loading && source && <span className="nom-found">{search ? `Найдено: ${formatDisplayNumber(total)}` : `Всего: ${formatDisplayNumber(total)}`}</span>}
      </div>

      <div className={loading && items.length > 0 ? 'dir-table-wrap dir-loading' : 'dir-table-wrap'}>
        <table className="sheet sheet-rownum dir-table nom-table">
          <thead><tr><th className="c-code">Код ТН ВЭД</th><th>Наименование</th><th className="nom-rate">Пошлина ЕТТ</th><th className="nom-kz" title="Ставка ниже ЕТТ по обязательствам Казахстана в ВТО">Ставка РК</th><th className="nom-vat">НДС</th><th className="nom-ad" title="Антидемпинговая пошлина ЕАЭС сверх ЕТТ, % для прочих производителей">Антидемпинг</th><th className="nom-sir" title="Стоимостной индикатор риска КГД для Китая (CN), USD за 1 кг; из нескольких профилей — больший">СУР, USD/кг</th><th className="nom-link" aria-label="Подробнее" /></tr></thead>
          <tbody>
            {items.length === 0 ? <tr><td colSpan={8} className="dir-empty">{loading ? 'Загрузка…'
              : !source ? 'Номенклатура ещё не загружена: администратор загружает её на странице «Обновление данных».'
              : empty ? 'Ничего не найдено. Проверьте код или попробуйте другие слова.' : ''}</td></tr>
              : items.map((item, index) => {
                const parts = nameParts(item.name)
                // Список отсортирован по коду: перед первым кодом товарной позиции — строка-заголовок.
                const heading = item.code.slice(0, 4)
                const headingRow = heading !== items[index - 1]?.code.slice(0, 4) && <tr className="nom-heading sheet-section">
                  <td colSpan={8}>Товарная позиция <b>{heading}</b></td>
                </tr>
                return <Fragment key={item.code}>{headingRow}<tr>
                  <td className="c-code"><CopyCode code={item.code} /></td>
                  <td className="nom-name" title={parts.join(' ')}>
                    {parts.length > 1 && <small>{parts.slice(0, -1).join(' ')} </small>}<span>{parts[parts.length - 1]}</span>
                  </td>
                  <td className="nom-rate">{item.tariffText || <span className="dir-none">не указана</span>}</td>
                  <td className={item.kz?.active ? 'nom-kz active' : 'nom-kz'} title={item.kz?.notesText || undefined}>{item.kz ? <>{item.kz.rateText}<small> · {item.kz.status}</small></> : <span className="dir-none">{kzSource ? '—' : ''}</span>}</td>
                  <td className="nom-vat">{item.vatPercent ? `${item.vatPercent}%` : <span className="dir-none">—</span>}</td>
                  <td className="nom-ad">{item.antiDumping?.length ? item.antiDumping.map((ref) => <AntiDumpingCell key={ref.number} measure={ref} />) : <span className="dir-none">—</span>}</td>
                  {/* Число и файл профиля СУР, из которого оно взято («24_прил CN»); полностью — в подсказке. */}
                  <td className="nom-sir" title={item.sir ? [item.sir.file && `Файл: ${item.sir.file}`, `Профиль ${item.sir.profile}`].filter(Boolean).join(' · ') : undefined}>
                    {item.sir ? <>{formatDisplayNumber(item.sir.priceUsd)}{item.sir.file && <small> · {item.sir.file.replace(/\.xlsx?$/i, '')}</small>}</> : <span className="dir-none">—</span>}
                  </td>
                  <td className="nom-link">{item.detailsUrl && <a href={item.detailsUrl} target="_blank" rel="noreferrer" title={`Код ${item.code} на ${source?.source ?? 'сайте источника'}: примечания, документы, меры`} aria-label={`Подробнее о коде ${item.code}`}>↗</a>}</td>
                </tr></Fragment>
              })}
          </tbody>
        </table>
        {items.length < total && <div className="nom-more">
          <span>Показано {formatDisplayNumber(items.length)} из {formatDisplayNumber(total)}</span>
          <button type="button" className="secondary" onClick={() => void loadMore()} disabled={loadingMore}>{loadingMore ? 'Загрузка…' : `Показать ещё ${Math.min(pageSize, total - items.length)}`}</button>
        </div>}
      </div>
      </section>
      <p className="nom-disclaimer">
        <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"><circle cx="8" cy="8" r="6.3" /><path d="M8 7.3v3.6" /><circle cx="8" cy="5" r=".4" fill="currentColor" /></svg>
        <span>«Пошлина ЕТТ» — Единый таможенный тариф ЕАЭС. «Ставка РК» — ставка ниже ЕТТ по обязательствам Казахстана в ВТО (<a href="https://eec.eaeunion.org/comission/department/catr/ttr/stavki.php" target="_blank" rel="noreferrer">перечень РК</a>). НДС 16% — основная ставка Казахстана.</span>
      </p>
    </section>
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
