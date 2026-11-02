import { ChangeEvent, Fragment, useEffect, useRef, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { User } from '@/shared/types/user'
import { Declaration, readDeclarationPdf } from './declarationPdf'
import { Check, CheckStatus, Report, RowCheck, exportCheckApi } from './exportCheckApi'
import { exportCheckStore } from './exportCheckStore'
import '@/shared/styles/list-page.css'
import './ExportCheckPage.css'

type Loaded = { file: File; declaration?: Declaration; error?: string }
type Tab = 'error' | 'warn' | 'all'

const plural = (n: number, one: string, few: string, many: string) => {
  const tail = n % 10, hundred = n % 100
  return hundred >= 11 && hundred <= 14 ? many : tail === 1 ? one : tail >= 2 && tail <= 4 ? few : many
}

// Число для показа: тот же знак после запятой, что и при сравнении на сервере (сумма — 2, цена — 4,
// вес — до 3), и разделители тысяч. Поэтому «280.00» и «280» выглядят одинаково, а расхождение,
// которое нашёл сервер, всегда видно глазами. Не число — как есть.
export function shownValue(label: string, text: string) {
  const value = text.trim()
  if (!/^-?\d+(\.\d+)?$/.test(value)) return value || '—'
  const number = Number(value)
  const lower = label.toLowerCase()
  let fixed = value
  if (lower.includes('сумма') || lower.includes('стоимость')) fixed = number.toFixed(2)
  else if (lower.includes('цена')) fixed = number.toFixed(4)
  else if (lower.includes('кг')) fixed = number.toFixed(3).replace(/\.?0+$/, '')
  return formatRuNumber(fixed)
}

const badgeText: Record<CheckStatus, string> = { ok: '✓ совпадает', error: '✕ расхождение', warn: '! проверьте', skip: 'не сверяется' }
const Badge = ({ status }: { status: CheckStatus }) => <span className={`ec-badge ec-${status}`}>{badgeText[status]}</span>
const pairColumns = ['Штук', 'Нетто, кг', 'Цена, USD', 'Сумма, USD']

// Сверка нашего инвойса с экспортной КНР: инвойс Excel и один или несколько PDF (партия больше 50 товаров
// разбита на файлы). PDF читается в браузере, сравнивает сервер; сверка запускается сама, как только
// загружены оба документа, и повторяется при замене любого файла.
export function ExportCheckPage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [invoice, setInvoice] = useState<File | null>(null)
  const [loaded, setLoaded] = useState<Loaded[]>([])
  const [reading, setReading] = useState(0)
  const [report, setReport] = useState<Report | null>(null)
  const [checking, setChecking] = useState(false)
  const [error, setError] = useState('')
  const [tab, setTab] = useState<Tab>('all')

  const onInvoice = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (file) setInvoice(file)
  }

  const onDeclarations = async (event: ChangeEvent<HTMLInputElement>) => {
    const files = [...(event.target.files ?? [])]
    event.target.value = ''
    if (files.length === 0) return
    setReading((count) => count + files.length)
    for (const file of files) {
      const next: Loaded = { file }
      try { next.declaration = await readDeclarationPdf(file) } catch (reason) {
        // Наши сообщения — по-русски; ошибки pdf.js («Invalid PDF structure») — понятным текстом.
        const message = errorMessage(reason)
        next.error = /[а-яё]/i.test(message) ? message : 'Не удалось открыть: файл повреждён или это не PDF'
      }
      setLoaded((list) => [...list.filter((item) => item.file.name !== file.name), next].sort((a, b) => a.file.name.localeCompare(b.file.name, 'ru', { numeric: true })))
      setReading((count) => count - 1)
    }
  }

  const remove = (name: string) => setLoaded((list) => list.filter((item) => item.file.name !== name))
  const reset = () => {
    setInvoice(null); setLoaded([]); setReport(null); setError(''); setTab('all')
    void exportCheckStore.clear(user.id).catch(() => undefined)
  }

  // Сверка сама: когда есть инвойс и прочитаны все PDF. Ключ — имена и даты файлов.
  const inputKey = invoice && reading === 0 ? [invoice.name, invoice.lastModified, ...loaded.map((item) => `${item.file.name}:${item.file.lastModified}`)].join('|') : ''

  // Восстановление после обновления страницы или перехода из другого раздела. Пока не восстановили,
  // ничего не сохраняем (иначе пустое состояние затрёт сохранённое); восстановленный отчёт не
  // пересчитываем — он уже соответствует этим файлам.
  const [restored, setRestored] = useState(false)
  const restoredKey = useRef('')
  useEffect(() => {
    let current = true
    exportCheckStore.load(user.id)
      .then((saved) => {
        if (!current || !saved) return
        if (saved.invoice && saved.report) restoredKey.current = [saved.invoice.name, saved.invoice.lastModified, ...saved.loaded.map((item) => `${item.file.name}:${item.file.lastModified}`)].join('|')
        setInvoice(saved.invoice); setLoaded(saved.loaded); setReport(saved.report); setTab(saved.tab)
      })
      .catch(() => undefined)
      .finally(() => { if (current) setRestored(true) })
    return () => { current = false }
  }, [user.id])
  useEffect(() => {
    if (!restored || reading > 0 || checking) return
    void exportCheckStore.save(user.id, { invoice, loaded, report, tab }).catch(() => undefined)
  }, [restored, reading, checking, invoice, loaded, report, tab, user.id])

  useEffect(() => {
    if (!restored) return
    if (inputKey && inputKey === restoredKey.current) { restoredKey.current = ''; return }
    restoredKey.current = ''
    const declarations = loaded.flatMap((item) => item.declaration ? [item.declaration] : [])
    if (!inputKey || !invoice || declarations.length === 0) { setReport(null); return }
    let current = true
    setChecking(true)
    setError('')
    exportCheckApi.check(invoice, declarations)
      .then((next) => {
        if (!current) return
        setReport(next)
        setTab(next.rows.some((row) => row.status === 'error') ? 'error' : next.rows.some((row) => row.status === 'warn') ? 'warn' : 'all')
      })
      .catch((reason) => { if (current) { setReport(null); setError(errorMessage(reason)) } })
      .finally(() => { if (current) setChecking(false) })
    return () => { current = false }
    // Повтор только при смене файлов: loaded и invoice входят в inputKey.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [inputKey, restored])

  const status = !restored ? 'Загружаем сохранённую сверку…' : checking ? 'Сверяем…' : reading > 0 ? `Читаем PDF: ${reading}…` : !invoice ? 'Загрузите инвойс Excel' : loaded.length === 0 ? 'Загрузите экспортную PDF' : ''

  return <main className="app-shell directory-shell ec-shell">
    <AppHeader user={user} section="exportCheck" onLogout={onLogout} />
    <section className="ec-page">
      <div className="ec-head">
        <div>
          <h1>Сверка экспортной</h1>
          <p>Загрузите наш инвойс Excel и экспортную от китайской стороны (PDF с текстом, если товаров больше 50 — все части).
            Сверка запустится сама: итоги, получатель и контракт, затем каждая строка — штуки, нетто, цена и сумма.</p>
        </div>
        {(invoice || loaded.length > 0) && <button type="button" className="ec-reset" onClick={reset} title="Убрать файлы и результат. Без этого сверка сохраняется при обновлении страницы и переходах">Очистить сверку</button>}
      </div>

      <div className="ec-files">
        <label className={invoice ? 'ec-drop filled' : 'ec-drop'}>
          <span className="ec-drop-title">1. Инвойс Excel</span>
          <span className="ec-drop-text">{invoice ? invoice.name : 'Перетащите файл .xlsx / .xls или нажмите'}</span>
          {invoice && <span className="ec-drop-hint">Нажмите, чтобы заменить</span>}
          <input type="file" accept=".xlsx,.xls" onChange={onInvoice} aria-label="Инвойс Excel" />
        </label>
        <div className={loaded.length > 0 ? 'ec-drop-group filled' : 'ec-drop-group'}>
          <label className="ec-drop ec-drop-pdf">
            <span className="ec-drop-title">2. Экспортные PDF</span>
            <span className="ec-drop-text">{reading > 0 ? `Читаем ${reading} ${plural(reading, 'файл', 'файла', 'файлов')}…` : loaded.length > 0 ? 'Добавить ещё части' : 'Перетащите один или несколько PDF или нажмите'}</span>
            <input type="file" accept=".pdf,application/pdf" multiple onChange={(event) => void onDeclarations(event)} aria-label="Экспортные PDF" />
          </label>
          {loaded.length > 0 && <ul className="ec-decls" aria-label="Загруженные экспортные">
            {loaded.map(({ file, declaration, error: readError }) => <li key={file.name} className={readError ? 'ec-decl error' : 'ec-decl'}>
              <b title={file.name}>{file.name}</b>
              {declaration && <span>{declaration.items.length} {plural(declaration.items.length, 'товар', 'товара', 'товаров')} · мест {declaration.places || '—'} · брутто {shownValue('кг', declaration.grossKg)} · нетто {shownValue('кг', declaration.netKg)}</span>}
              {readError && <span>{readError}</span>}
              <button type="button" onClick={() => remove(file.name)} aria-label={`Убрать ${file.name}`} title="Убрать файл">×</button>
            </li>)}
          </ul>}
        </div>
      </div>

      {error && <p className="banner error" role="alert">{error}<button type="button" onClick={() => setError('')}>×</button></p>}
      {status && !report && <p className="ec-status-line" role="status">{status}</p>}

      {report && <ReportView report={report} files={loaded.filter((item) => item.declaration).length} unread={loaded.filter((item) => item.error).map((item) => item.file.name)} checking={checking} tab={tab} onTab={setTab} />}
    </section>
  </main>
}

function ReportView({ report, files, unread, checking, tab, onTab }: { report: Report; files: number; unread: string[]; checking: boolean; tab: Tab; onTab: (tab: Tab) => void }) {
  const counts = {
    error: report.rows.filter((row) => row.status === 'error').length,
    warn: report.rows.filter((row) => row.status === 'warn').length,
    all: report.rows.length,
  }
  const rows = tab === 'all' ? report.rows : report.rows.filter((row) => row.status === tab)
  // Непрочитанный PDF — сверка неполная: зелёный вердикт показывать нельзя, даже если всё сошлось.
  const verdict = report.errors > 0 ? 'error' : unread.length > 0 ? 'warn' : 'ok'
  const title = report.errors > 0
    ? `${report.errors} ${plural(report.errors, 'расхождение', 'расхождения', 'расхождений')}`
    : unread.length > 0 ? 'Сверка неполная' : 'Расхождений нет'
  const tabs: { value: Tab; label: string }[] = [
    { value: 'error', label: 'Расхождения' },
    { value: 'warn', label: 'Проверьте' },
    { value: 'all', label: 'Все строки' },
  ]
  return <div className={checking ? 'ec-report ec-stale' : 'ec-report'}>
    <div className={`ec-verdict ${verdict}`} role="status">
      <span className="ec-verdict-icon" aria-hidden="true">{verdict === 'ok' ? '✓' : verdict === 'warn' ? '!' : '✕'}</span>
      <div>
        <b>{title}{report.warnings > 0 && <span className="ec-verdict-warn"> · {report.warnings} {plural(report.warnings, 'предупреждение', 'предупреждения', 'предупреждений')}</span>}</b>
        <span>Инвойс {report.invoiceNumber || 'без номера'}: {report.invoiceRows} {plural(report.invoiceRows, 'строка', 'строки', 'строк')} · в {files} {plural(files, 'экспортной', 'экспортных', 'экспортных')} {report.declarationItems} {plural(report.declarationItems, 'товар', 'товара', 'товаров')}</span>
        {unread.length > 0 && <span className="ec-verdict-unread">Не прочитан{unread.length > 1 ? 'ы' : ''}: {unread.join(', ')}. Уберите {unread.length > 1 ? 'эти файлы' : 'этот файл'} или загрузите правильный PDF.</span>}
      </div>
    </div>

    {(report.problems.length > 0 || report.notes.length > 0) && <div className="ec-messages">
      {report.problems.length > 0 && <ul className="ec-list error">{report.problems.map((text) => <li key={text}>{text}</li>)}</ul>}
      {report.notes.length > 0 && <ul className="ec-list warn">{report.notes.map((text) => <li key={text}>{text}</li>)}</ul>}
    </div>}

    <div className="ec-summary-grid">
      <ChecksTable title="Итоги" checks={report.totals} />
      <ChecksTable title="Реквизиты" checks={report.details} />
    </div>

    <div className="ec-rows-head">
      <h2>Строки</h2>
      <div className="ec-tabs" role="tablist" aria-label="Какие строки показать">
        {tabs.map(({ value, label }) => <button key={value} type="button" role="tab" aria-selected={tab === value}
          className={`ec-tab ec-tab-${value}${tab === value ? ' active' : ''}`} onClick={() => onTab(value)} disabled={value !== 'all' && counts[value] === 0}>
          {label} <span>{counts[value]}</span>
        </button>)}
      </div>
    </div>
    {rows.length === 0
      ? <p className="ec-empty">Нет строк в этом списке.</p>
      : <div className="ec-table-wrap"><table className="ec-table ec-rows">
        {/* По каждому показателю две колонки рядом — из инвойса и из экспортной; совпавшее значение
            экспортной — серым, расхождение — обе ячейки красным. Код ТН ВЭД только для справки. */}
        <colgroup>
          <col className="ec-col-no" /><col className="ec-col-name" />
          {pairColumns.map((label) => <Fragment key={label}><col /><col /></Fragment>)}
          <col className="ec-col-code" /><col className="ec-col-result" />
        </colgroup>
        <thead>
          <tr>
            <th rowSpan={2}>№</th><th rowSpan={2}>Наименование · откуда взято</th>
            {pairColumns.map((label) => <th key={label} colSpan={2} className="ec-pair ec-group">{label}</th>)}
            <th rowSpan={2} className="ec-pair">Код ТН ВЭД<small>инвойс / эксп.</small></th>
            <th rowSpan={2}>Результат</th>
          </tr>
          <tr>{pairColumns.map((label) => <Fragment key={label}><th className="ec-sub ec-pair">Инвойс</th><th className="ec-sub">Экспортная</th></Fragment>)}</tr>
        </thead>
        <tbody>{rows.map((row) => <RowView key={row.no} row={row} showFile={files > 1} />)}</tbody>
      </table></div>}
  </div>
}

function ChecksTable({ title, checks }: { title: string; checks: Check[] }) {
  const isNumber = (text: string) => /^-?\d+(\.\d+)?$/.test(text)
  const numericTable = checks.every((check) => isNumber(check.invoice) || isNumber(check.declaration))
  const align = numericTable ? 'ec-right' : undefined
  return <section className="ec-card" aria-label={title}>
    <h2>{title}</h2>
    <table className="ec-table ec-checks">
      <colgroup><col className="ec-col-label" /><col /><col /><col className="ec-col-verdict" /></colgroup>
      <thead><tr><th>Показатель</th><th className={align}>Инвойс</th><th className={align}>Экспортная</th><th>Результат</th></tr></thead>
      <tbody>{checks.map((check) => {
        const numeric = numericTable && (isNumber(check.invoice) || isNumber(check.declaration))
        return <tr key={check.label} className={`ec-${check.status}`}>
          <td>{check.label}</td>
          <td className={`ec-value ec-${check.status}${numeric ? ' ec-right' : ''}`}>{numeric ? shownValue(check.label, check.invoice) : check.invoice || '—'}</td>
          <td className={`ec-value ec-${check.status}${numeric ? ' ec-right' : ''}`}>{numeric ? shownValue(check.label, check.declaration) : check.declaration || '—'}</td>
          <td><Badge status={check.status} />{check.note && <small>{check.note}</small>}</td>
        </tr>
      })}</tbody>
    </table>
  </section>
}

function RowView({ row, showFile }: { row: RowCheck; showFile: boolean }) {
  const missing = row.checks.length === 1
  // Откуда взяты обе стороны: строка листа Excel и товар в PDF — чтобы расхождение открыть и проверить.
  const source = [
    row.excelRow ? `Excel стр. ${row.excelRow}` : '',
    row.declarationNo ? `PDF${showFile && row.file ? ` ${row.file}` : ''} товар ${row.declarationNo}` : '',
    row.declarationName,
  ].filter(Boolean).join(' · ')
  return <tr className={`ec-${row.status}`}>
    <td className="ec-no">{row.no}</td>
    <td className="ec-name"><span className="ec-name-text" title={row.name}>{row.name}</span>{source && <small title={row.file}>{source}</small>}</td>
    {missing
      ? <td colSpan={8} className="ec-missing ec-pair">{row.checks[0].note}</td>
      : row.checks.map((check) => <Fragment key={check.label}>
        <td className={`ec-pair ec-num ec-${check.status}`} title={`${check.label}, инвойс: ${check.invoice}`}>{shownValue(check.label, check.invoice)}</td>
        <td className={`ec-num ec-decl-side ec-${check.status}`} title={`${check.label}, экспортная: ${check.declaration}`}>{shownValue(check.label, check.declaration)}</td>
      </Fragment>)}
    <td className="ec-pair ec-code" title="Код ТН ВЭД только для справки: китайские коды отличаются от кодов ЕАЭС">
      {row.codeInvoice || '—'}{row.codeDeclaration && <small>{row.codeDeclaration}</small>}
    </td>
    {/* Пояснения по строке — здесь, а не в узких ячейках чисел: какая колонка и что не так. */}
    <td className="ec-result">
      <Badge status={row.status} />
      {!missing && row.checks.filter((check) => check.note).map((check) => <small key={check.label} className={`ec-note ec-${check.status}`}><b>{check.label.split(',')[0]}:</b> {check.note}</small>)}
    </td>
  </tr>
}
