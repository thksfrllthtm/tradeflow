import { ChangeEvent, Fragment, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { errorMessage } from '@/shared/lib/errorMessage'
import { User } from '@/shared/types/user'
import { Declaration, readDeclarationPdf } from './declarationPdf'
import { Check, CheckStatus, Report, RowCheck, exportCheckApi } from './exportCheckApi'
import '@/shared/styles/list-page.css'
import './ExportCheckPage.css'

type Loaded = { file: File; declaration?: Declaration; error?: string }

const plural = (n: number, one: string, few: string, many: string) => {
  const tail = n % 10, hundred = n % 100
  return hundred >= 11 && hundred <= 14 ? many : tail === 1 ? one : tail >= 2 && tail <= 4 ? few : many
}

// Сверка нашего инвойса с экспортной декларацией КНР: инвойс Excel и один или несколько PDF деклараций
// (партия больше 50 товаров разбита на файлы). PDF читается в браузере, сравнивает сервер.
export function ExportCheckPage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [invoice, setInvoice] = useState<File | null>(null)
  const [loaded, setLoaded] = useState<Loaded[]>([])
  const [reading, setReading] = useState(0)
  const [report, setReport] = useState<Report | null>(null)
  const [checking, setChecking] = useState(false)
  const [error, setError] = useState('')
  const [onlyProblems, setOnlyProblems] = useState(true)

  const onInvoice = (event: ChangeEvent<HTMLInputElement>) => {
    setInvoice(event.target.files?.[0] ?? null)
    setReport(null)
  }

  const onDeclarations = async (event: ChangeEvent<HTMLInputElement>) => {
    const files = [...(event.target.files ?? [])]
    event.target.value = ''
    if (files.length === 0) return
    setReport(null)
    setReading((count) => count + files.length)
    for (const file of files) {
      const next: Loaded = { file }
      try { next.declaration = await readDeclarationPdf(file) } catch (reason) { next.error = errorMessage(reason) }
      setLoaded((list) => [...list.filter((item) => item.file.name !== file.name), next])
      setReading((count) => count - 1)
    }
  }

  const remove = (name: string) => { setLoaded((list) => list.filter((item) => item.file.name !== name)); setReport(null) }

  const declarations = loaded.flatMap((item) => item.declaration ? [item.declaration] : [])
  const ready = invoice !== null && declarations.length > 0 && reading === 0 && !checking

  const check = async () => {
    if (!invoice) return
    setChecking(true)
    setError('')
    try {
      const next = await exportCheckApi.check(invoice, declarations)
      setReport(next)
      setOnlyProblems(next.rows.some((row) => row.status !== 'ok'))
    } catch (reason) {
      setError(errorMessage(reason))
    } finally {
      setChecking(false)
    }
  }

  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="exportCheck" onLogout={onLogout} />
    <section className="directory-page ec-page">
      <div className="dir-head">
        <div>
          <h1>Сверка экспортной декларации</h1>
          <p>Загрузите наш инвойс Excel и экспортную декларацию от китайской стороны (PDF с текстом). Программа сравнит места, брутто и нетто,
            получателя и номер контракта, а затем каждую строку: штуки, нетто, цену и сумму. Если партия больше 50 товаров, загрузите все части сразу.</p>
        </div>
      </div>
      {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}

      <div className="ec-files">
        <label className={invoice ? 'ec-drop filled' : 'ec-drop'}>
          <span className="ec-drop-title">Инвойс Excel</span>
          <span className="ec-drop-text">{invoice ? invoice.name : 'Выберите файл .xlsx или .xls'}</span>
          <input type="file" accept=".xlsx,.xls" onChange={onInvoice} />
        </label>
        <label className={loaded.length > 0 ? 'ec-drop filled' : 'ec-drop'}>
          <span className="ec-drop-title">Экспортные декларации PDF</span>
          <span className="ec-drop-text">{reading > 0 ? `Читаем ${reading} ${plural(reading, 'файл', 'файла', 'файлов')}…` : loaded.length > 0 ? `${loaded.length} ${plural(loaded.length, 'файл', 'файла', 'файлов')}, можно добавить ещё` : 'Выберите один или несколько файлов'}</span>
          <input type="file" accept=".pdf,application/pdf" multiple onChange={(event) => void onDeclarations(event)} />
        </label>
      </div>

      {loaded.length > 0 && <ul className="ec-decls" aria-label="Загруженные декларации">
        {loaded.map(({ file, declaration, error: readError }) => <li key={file.name} className={readError ? 'ec-decl error' : 'ec-decl'}>
          <b>{file.name}</b>
          {declaration && <span>
            {declaration.billNo && <>№ {declaration.billNo} · </>}{declaration.items.length} {plural(declaration.items.length, 'товар', 'товара', 'товаров')} · мест {declaration.places || '—'} · брутто {declaration.grossKg || '—'} · нетто {declaration.netKg || '—'}
            {declaration.receiver && <> · {declaration.receiver}</>}
          </span>}
          {readError && <span>{readError}</span>}
          <button type="button" onClick={() => remove(file.name)} aria-label={`Убрать ${file.name}`}>×</button>
        </li>)}
      </ul>}

      <div className="ec-actions">
        <button type="button" className="primary" disabled={!ready} onClick={() => void check()}>{checking ? 'Сверяем…' : 'Сверить'}</button>
      </div>

      {report && <ReportView report={report} onlyProblems={onlyProblems} onToggle={setOnlyProblems} />}
    </section>
  </main>
}

function ReportView({ report, onlyProblems, onToggle }: { report: Report; onlyProblems: boolean; onToggle: (value: boolean) => void }) {
  const rows = onlyProblems ? report.rows.filter((row) => row.status !== 'ok') : report.rows
  const clean = report.errors === 0
  return <div className="ec-report">
    <p className={clean ? 'ec-summary ok' : 'ec-summary error'} role="status">
      {clean ? 'Расхождений нет. ' : `${report.errors} ${plural(report.errors, 'расхождение', 'расхождения', 'расхождений')}. `}
      Инвойс {report.invoiceNumber || 'без номера'}: {report.invoiceRows} {plural(report.invoiceRows, 'строка', 'строки', 'строк')}, в декларациях {report.declarationItems} {plural(report.declarationItems, 'товар', 'товара', 'товаров')}
      {report.warnings > 0 && <>, {report.warnings} {plural(report.warnings, 'предупреждение', 'предупреждения', 'предупреждений')}</>}.
    </p>
    {report.problems.length > 0 && <ul className="ec-list error">{report.problems.map((text) => <li key={text}>{text}</li>)}</ul>}
    {report.notes.length > 0 && <ul className="ec-list warn">{report.notes.map((text) => <li key={text}>{text}</li>)}</ul>}

    <ChecksTable title="Итоги" checks={report.totals} />
    <ChecksTable title="Реквизиты" checks={report.details} />

    <div className="ec-rows-head">
      <h2>Строки</h2>
      <span className="ec-rows-count">{report.rows.length} {plural(report.rows.length, 'строка', 'строки', 'строк')}, совпадают {report.rows.filter((row) => row.status === 'ok').length}</span>
      <label className="ec-toggle"><input type="checkbox" checked={onlyProblems} onChange={(event) => onToggle(event.target.checked)} /> Только расхождения</label>
    </div>
    {rows.length === 0
      ? <p className="dir-empty">Все строки совпадают.</p>
      : <div className="dir-table-wrap"><table className="dir-table ec-table ec-rows">
        {/* По каждому показателю две колонки рядом — из инвойса и из декларации, чтобы было видно, что
            именно сравнивалось; вердикт по строке — цветной значок справа. */}
        <thead>
          <tr>
            <th rowSpan={2}>№</th><th rowSpan={2}>Наименование</th>
            {pairColumns.map((label) => <th key={label} colSpan={2} className="ec-pair">{label}</th>)}
            <th rowSpan={2}>Результат</th>
          </tr>
          <tr>{pairColumns.map((label) => <Fragment key={label}><th className="ec-sub ec-pair">Инвойс</th><th className="ec-sub">Декларация</th></Fragment>)}</tr>
        </thead>
        <tbody>{rows.map((row) => <RowView key={row.no} row={row} />)}</tbody>
      </table></div>}
  </div>
}

const pairColumns = ['Штук', 'Нетто, кг', 'Цена, USD', 'Сумма, USD', 'Код ТН ВЭД']
const badgeText: Record<CheckStatus, string> = { ok: '✓ совпадает', error: '✕ расхождение', warn: '! проверьте', skip: 'не сверяется' }
const Badge = ({ status }: { status: CheckStatus }) => <span className={`ec-badge ec-${status}`}>{badgeText[status]}</span>

function ChecksTable({ title, checks }: { title: string; checks: Check[] }) {
  return <>
    <h2>{title}</h2>
    <div className="dir-table-wrap"><table className="dir-table ec-table">
      <thead><tr><th>Показатель</th><th>Инвойс</th><th>Декларация</th><th>Результат</th></tr></thead>
      <tbody>{checks.map((check) => <tr key={check.label} className={`ec-${check.status}`}>
        <td>{check.label}</td>
        <td className={`ec-value ec-${check.status}`}>{check.invoice || '—'}</td>
        <td className={`ec-value ec-${check.status}`}>{check.declaration || '—'}</td>
        <td><Badge status={check.status} />{check.note && <small>{check.note}</small>}</td>
      </tr>)}</tbody>
    </table></div>
  </>
}

function RowView({ row }: { row: RowCheck }) {
  const missing = row.checks.length === 1
  // Откуда взяты обе стороны: строка листа Excel и товар в файле декларации, чтобы расхождение
  // можно было открыть и проверить глазами.
  const source = [
    row.excelRow ? `Excel: строка ${row.excelRow}` : '',
    row.file ? [row.file, row.declarationNo ? `товар ${row.declarationNo}` : '', row.declarationName].filter(Boolean).join(' · ') : '',
  ].filter(Boolean).join(' — ')
  return <tr className={`ec-${row.status}`}>
    <td>{row.no}</td>
    <td className="ec-name">{row.name}{source && <small>{source}</small>}</td>
    {missing
      ? <td colSpan={10} className="ec-missing">{row.checks[0].note}</td>
      : <>
        {row.checks.map((check) => <Fragment key={check.label}>
          <td className={`ec-pair ec-num ec-${check.status}`}>{check.invoice || '—'}</td>
          <td className={`ec-num ec-${check.status}`}>{check.declaration || '—'}{check.note && <small>{check.note}</small>}</td>
        </Fragment>)}
        <td className="ec-pair ec-code">{row.codeInvoice || '—'}</td>
        <td className="ec-code">{row.codeDeclaration || '—'}</td>
      </>}
    <td><Badge status={row.status} /></td>
  </tr>
}
