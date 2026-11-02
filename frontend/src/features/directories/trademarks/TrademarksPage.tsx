import { ChangeEvent, useEffect, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatDisplayNumber } from '@/shared/lib/formatDisplayNumber'
import { formatRuDate } from '@/shared/lib/dates'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { shortHolder, TrademarkItem, trademarkLogoUrl, trademarksApi, TrademarkSummary } from '@/shared/trademarks/trademarks'
import { isAdmin, User } from '@/shared/types/user'
import '@/shared/styles/list-page.css'
import '../components/DirectoryTransfer.css'
import '../kzList/KzListPanel.css'
import '../riskIndicators/RiskIndicatorsPanel.css'
import './TrademarksPage.css'

// «ТРОИС» — таможенный реестр объектов интеллектуальной собственности КГД. Список и поиск видят все;
// загрузка файла реестра и подпись логотипов — администратор. Логотипы без текста подписывает ИИ в фоне
// (по ним ищутся бренды в наименованиях), неверную подпись администратор исправляет здесь.
export function TrademarksPage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const admin = isAdmin(user)
  const [summary, setSummary] = useState<TrademarkSummary | null>(null)
  const [items, setItems] = useState<TrademarkItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [query, setQuery] = useState('')
  const [unnamed, setUnnamed] = useState(false)
  const [busy, setBusy] = useState('')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [reload, setReload] = useState(0)
  const search = useDebouncedValue(query, 300)

  useEffect(() => {
    let active = true
    trademarksApi.list(search, unnamed, page).then((body) => {
      if (!active) return
      setSummary(body.summary)
      setTotal(body.total)
      setItems((current) => page > 0 ? [...current, ...body.items] : body.items)
    }).catch((reason) => { if (active) setError(errorMessage(reason)) })
    return () => { active = false }
  }, [search, unnamed, page, reload])

  async function upload(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    setBusy('Загружаем и читаем логотипы…'); setMessage(''); setError('')
    try {
      const result = await trademarksApi.upload(file)
      const notes = [
        `${formatDisplayNumber(result.excluded)} исключённых пропущено`,
        result.badLogos.length > 0 ? `логотипы не прочитались: ${result.badLogos.join(', ')}` : '',
        result.skipped.length > 0 ? `пропущены строки — ${result.skipped.join('; ')}` : '',
      ].filter(Boolean)
      setMessage(`Загружено ${formatDisplayNumber(result.summary.total)} действующих записей, из них ${formatDisplayNumber(result.summary.logos)} с логотипом; ${notes.join('; ')}. Логотипы без надписи ИИ подпишет в фоне.`)
      setPage(0); setReload((value) => value + 1)
    } catch (reason) { setError(errorMessage(reason)) } finally { setBusy('') }
  }

  async function saveLogoName(item: TrademarkItem, name: string) {
    if (name.trim() === item.logoName) return
    setError('')
    try {
      await trademarksApi.setLogoName(item.regNo, name)
      setItems((current) => current.map((other) => other.regNo === item.regNo ? { ...other, logoName: name.trim(), logoNameSource: name.trim() ? 'admin' : '' } : other))
    } catch (reason) { setError(errorMessage(reason)) }
  }

  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="trademarks" onLogout={onLogout} />
    <section className="directory-page tm-page">
      <div className="dir-head">
        <div>
          <h1>ТРОИС — таможенный реестр объектов интеллектуальной собственности</h1>
          <p>{summary?.importedAt
            ? <>Реестр КГД · загружено {formatRuDate(summary.importedAt.slice(0, 10))}{summary.importedByName && `, ${summary.importedByName}`}{summary.logos > 0 && <> · логотипов подписано ИИ {formatDisplayNumber(summary.logosRead)} из {formatDisplayNumber(summary.logos)}</>}</>
            : 'Товарные знаки под защитой таможни РК.'}</p>
        </div>
      </div>
      {admin && <div className="kz-step">
        <span className="kz-step-number">↑</span>
        <div><b>Загрузить реестр</b><p>Файл КГД как есть (.xls) — без пересохранения: так логотипы сохраняются в исходном качестве. Новый файл заменяет реестр целиком, подписи логотипов переносятся.</p></div>
        <label className={busy ? 'kz-file primary disabled' : 'kz-file primary'}>
          {busy || 'Выбрать файл'}
          <input type="file" accept=".xls,.xlsx" onChange={(event) => void upload(event)} disabled={busy !== ''} />
        </label>
      </div>}
      {message && <p className="dir-transfer-ok" role="status">{message}</p>}
      {error && <p className="dir-transfer-error" role="alert">{error}</p>}
      {summary && summary.total === 0 ? <p className="sir-empty">{admin ? 'Реестр ещё не загружен.' : 'Реестр ещё не загружен администратором.'}</p> : <>
        <div className="tm-filters">
          <input type="search" value={query} onChange={(event) => { setQuery(event.target.value); setPage(0) }} placeholder="Знак, номер или правообладатель" aria-label="Поиск по реестру" />
          <label><input type="checkbox" checked={unnamed} onChange={(event) => { setUnnamed(event.target.checked); setPage(0) }} /> Логотипы без надписи</label>
          <span>{formatDisplayNumber(total)} найдено</span>
        </div>
        <div className="dir-table-wrap">
          <table className="sheet dir-table tm-table">
            <thead><tr><th>Знак</th><th>Номер</th><th>Товары</th><th>Правообладатель</th><th>Защита до</th></tr></thead>
            <tbody>{items.map((item) => <tr key={item.regNo}>
              <td className="tm-mark">
                {item.hasLogo && <img src={trademarkLogoUrl(item.regNo)} alt={item.name || item.logoName || 'Логотип'} loading="lazy" />}
                {item.name ? <b>{item.name}</b> : item.hasLogo && (admin
                  ? <input key={item.logoName} defaultValue={item.logoName} placeholder={item.logoRead ? 'ИИ не прочитал — впишите' : 'ждёт ИИ — или впишите'} aria-label={`Надпись логотипа ${item.regNo}`}
                    onBlur={(event) => void saveLogoName(item, event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') event.currentTarget.blur() }} />
                  : <b>{item.logoName || '—'}</b>)}
                {!item.name && item.logoName && <small>{item.logoNameSource === 'admin' ? 'подписано вручную' : 'прочитано ИИ'}</small>}
                {item.logoPicture && <small>рисунок: {item.logoPicture}</small>}
              </td>
              <td>{item.regNo}</td>
              <td className="tm-goods">{item.goods}</td>
              <td>{shortHolder(item.holder)}</td>
              <td>{item.protectedUntil ? formatRuDate(item.protectedUntil) : '—'}</td>
            </tr>)}</tbody>
          </table>
          {items.length < total && <button type="button" className="button-link tm-more" onClick={() => setPage(page + 1)}>Показать ещё</button>}
        </div>
      </>}
    </section>
  </main>
}
