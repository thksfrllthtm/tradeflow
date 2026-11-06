import { useEffect, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatRuDate } from '@/shared/lib/dates'
import { AntiDumpingImport, AntiDumpingMeasure, antiDumpingApi, countryNames, formatMeasureCode, percentText } from './antiDumping'

type Props = { admin: boolean; onPickCode: (code: string) => void; onRefreshed: () => void; onLoaded: (count: number) => void }

// Панель «Антидемпинг ЕАЭС» на странице номенклатуры: все действующие меры из реестра ЕЭК —
// товар, страны и ставки для прочих производителей, коды (нажатие ищет код в номенклатуре),
// сроки и решение ЕЭК. Администратор обновляет меры с сайта ЕЭК.
export function AntiDumpingPanel({ admin, onPickCode, onRefreshed, onLoaded }: Props) {
  const [measures, setMeasures] = useState<AntiDumpingMeasure[]>([])
  const [source, setSource] = useState<AntiDumpingImport | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [open, setOpen] = useState<string | null>(null)

  useEffect(() => {
    let active = true
    antiDumpingApi.list().then((list) => { if (active) { setMeasures(list.measures); setSource(list.import); onLoaded(list.measures.length) } })
      .catch((reason) => { if (active) setError(errorMessage(reason)) }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
    // Загружается один раз при открытии панели.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function refresh() {
    setBusy(true); setError(''); setNotice('')
    try {
      const list = await antiDumpingApi.refresh()
      setMeasures(list.measures); setSource(list.import); onLoaded(list.measures.length)
      setNotice(`Обновлено из реестра ЕЭК: действующих мер — ${list.measures.length}.`)
      onRefreshed()
    } catch (reason) { setError(errorMessage(reason)) } finally { setBusy(false) }
  }

  return <section className="dump-panel" aria-label="Антидемпинговые меры ЕАЭС">
    <header className="dump-head">
      <div>
        <h2>Антидемпинговые меры ЕАЭС <span className="badge error">{measures.length}</span></h2>
        <p>
          Пошлина сверх ЕТТ, % от таможенной стоимости, для товара из указанной страны происхождения. В смете начисляется ставка «для прочих производителей» по полю «Страна».
          {source && <> Источник: <a href={source.registryUrl} target="_blank" rel="noreferrer">реестр ЕЭК</a> · {source.source === 'eec' ? <>обновлено <time dateTime={source.importedAt}>{formatRuDate(source.importedAt.slice(0, 10))}</time>{source.importedByName ? `, ${source.importedByName}` : ''}</> : <>данные из файла программы (реестр на <time dateTime="2026-10-02">02.10.2026</time>)</>}</>}
        </p>
      </div>
      {admin && <button type="button" className="secondary" onClick={() => void refresh()} disabled={busy}>{busy ? 'Обновляем…' : 'Обновить из реестра ЕЭК'}</button>}
    </header>
    {notice && <p className="dir-transfer-ok" role="status">{notice}</p>}
    {error && <p className="dir-transfer-error" role="alert">{error}</p>}
    {loading ? <p className="dump-empty">Загрузка…</p> : measures.length === 0 ? <p className="dump-empty">Мер нет.</p> : <div className="dump-table-wrap"><table className="sheet dump-table">
      <thead><tr><th>Мера</th><th>Товар</th><th>Страна → ставка для прочих производителей</th><th>Коды ТН ВЭД</th><th>Действует</th><th aria-label="Решение" /></tr></thead>
      <tbody>{measures.map((measure) => {
        const expanded = open === measure.number
        const codes = measure.codes.filter((code) => code.length === 10 || !measure.codes.some((other) => other.length > code.length && other.startsWith(code)))
        return <tr key={measure.number} className={measure.suspended ? 'dump-suspended' : undefined}>
          <td className="dump-num">{measure.number}</td>
          <td className="dump-name">
            <b>{measure.shortName}</b>
            {measure.suspended && <i className="badge dump-flag">временно не применяется</i>}
            {measure.needsCheck && <small className="dump-check">{measure.needsCheck}</small>}
            <button type="button" className="button-link dump-more" onClick={() => setOpen(expanded ? null : measure.number)} aria-expanded={expanded}>{expanded ? 'Скрыть ставки по производителям' : 'Ставки по производителям'}</button>
            {expanded && <pre className="dump-rate-text">{measure.rateText}</pre>}
          </td>
          <td className="dump-rates">{measure.countries.map((country) => <span key={country}><em>{countryNames[country] ?? country}</em><b>{percentText(measure.rates[country] ?? '')}</b></span>)}</td>
          <td className="dump-codes">{codes.slice(0, expanded ? codes.length : 6).map((code) => <button key={code} type="button" onClick={() => onPickCode(code)} title={code.length === 10 ? 'Найти код в номенклатуре' : 'Товарная позиция: мера на часть её товаров — найти коды позиции'} className={code.length === 10 ? '' : 'heading'}>{formatMeasureCode(code)}</button>)}
            {!expanded && codes.length > 6 && <button type="button" className="dump-codes-more" onClick={() => setOpen(measure.number)}>ещё {codes.length - 6}</button>}</td>
          <td className="dump-dates">{measure.startDate && <time dateTime={measure.startDate.slice(0, 10)}>{formatRuDate(measure.startDate)}</time>}<br />по {measure.endDate && <time dateTime={measure.endDate.slice(0, 10)}>{formatRuDate(measure.endDate)}</time>}</td>
          <td className="dump-doc">{measure.documentUrl && <a href={measure.documentUrl} target="_blank" rel="noreferrer">Решение ЕЭК</a>}</td>
        </tr>
      })}</tbody>
    </table></div>}
  </section>
}
