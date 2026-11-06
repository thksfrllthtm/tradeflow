import { ChangeEvent, Fragment, useEffect, useRef, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { PageHead } from '@/shared/components/PageHead/PageHead'
import { errorMessage } from '@/shared/lib/errorMessage'
import { plural } from '@/shared/lib/plural'
import { User } from '@/shared/types/user'
import { readDeclarationPdf } from './declarationPdf'
import { Check, CheckStatus, Report, RowCheck, exportCheckApi } from './exportCheckApi'
import { exportCheckStore, SavedCheck } from './exportCheckStore'
import { shownValue } from './shownValue'
import '@/shared/styles/list-page.css'
import './ExportCheckPage.css'

type Loaded = SavedCheck['loaded'][number]
type Tab = SavedCheck['tab']

const badgeText: Record<CheckStatus, string> = { ok: '✓ совпадает', error: '✕ расхождение', warn: '! проверьте', skip: 'не сверяется' }
const Badge = ({ status }: { status: CheckStatus }) => <span className={`badge ${status === 'skip' ? '' : status}`}>{badgeText[status]}</span>
const pairColumns = ['Штук', 'Нетто, кг', 'Цена, USD', 'Сумма, USD']
// Короткие подписи проверок в карточке. Полное название с сервера остаётся для форматирования чисел
// (shownValue) и для подсказки.
const shortLabel: Record<string, string> = {
  'Количество мест': 'Места',
  'Вес брутто, кг': 'Брутто, кг',
  'Вес нетто, кг': 'Нетто, кг',
  'Общая стоимость, USD': 'Сумма, USD',
  'Номер контракта': 'Контракт',
}

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

  const status = !restored ? 'Загрузка сохранённой сверки…' : checking ? 'Сверяем…' : reading > 0 ? `Читаем PDF: ${reading}…` : !invoice ? 'Загрузите инвойс Excel' : loaded.length === 0 ? 'Загрузите экспортную PDF' : ''

  return <main className="app-shell directory-shell ec-shell">
    <AppHeader user={user} section="exportCheck" onLogout={onLogout} />
    <section className="directory-page ec-page">
      <PageHead title="Сверка экспортной"
        subtitle={!report && <>Загрузите наш инвойс Excel и экспортную от китайской стороны (PDF с текстом, если товаров больше 50 — все части).
          Сверка запустится сама: итоги, получатель и контракт, затем каждая строка — штуки, нетто, цена и сумма.</>}
        actions={(invoice || loaded.length > 0) && <button type="button" className="danger" onClick={reset} title="Убрать файлы и результат. Без этого сверка сохраняется при обновлении страницы и переходах">Очистить сверку</button>} />

      <div className="ec-files">
        <label className={invoice ? 'ec-drop filled' : 'ec-drop'} title={invoice ? `${invoice.name}\nНажмите, чтобы заменить` : undefined}>
          <span className="ec-drop-title">1. Инвойс Excel</span>
          <span className="ec-drop-text">{invoice ? invoice.name : 'Перетащите файл .xlsx / .xls или нажмите'}</span>
          <input type="file" accept=".xlsx,.xls" onChange={onInvoice} aria-label="Инвойс Excel" />
        </label>
        <div className={loaded.length > 0 ? 'ec-drop-group filled' : 'ec-drop-group'}>
          <label className="ec-drop ec-drop-pdf">
            <span className="ec-drop-title">2. Экспортные PDF</span>
            <span className="ec-drop-text">{reading > 0 ? `Читаем ${reading} ${plural(reading, 'файл', 'файла', 'файлов')}…` : loaded.length > 0 ? 'Добавить ещё части' : 'Перетащите один или несколько PDF или нажмите'}</span>
            <input type="file" accept=".pdf,application/pdf" multiple onChange={(event) => void onDeclarations(event)} aria-label="Экспортные PDF" />
          </label>
          {loaded.length > 0 && <ul className="ec-decls" aria-label="Загруженные экспортные">
            {/* Части — плашками в ряд: имя и число товаров; места, брутто и нетто части — в подсказке
                (итоги по всем частям сверяются в карточке ниже). */}
            {loaded.map(({ file, declaration, error: readError }) => <li key={file.name} className={readError ? 'ec-decl error' : 'ec-decl'}
              title={declaration ? `${file.name}\n${declaration.items.length} ${plural(declaration.items.length, 'товар', 'товара', 'товаров')} · мест ${declaration.places || '—'} · брутто ${shownValue('кг', declaration.grossKg)} · нетто ${shownValue('кг', declaration.netKg)}` : file.name}>
              <b>{file.name}</b>
              {declaration && <span>{declaration.items.length} {plural(declaration.items.length, 'товар', 'товара', 'товаров')}</span>}
              {readError && <span>{readError}</span>}
              <button type="button" className="icon-button ghost sm" onClick={() => remove(file.name)} aria-label={`Убрать ${file.name}`} title="Убрать файл">×</button>
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
  // Вердикт и все проверки документа — одной карточкой: итоги и реквизиты ячейками в ряд,
  // чтобы основное место на экране осталось строкам.
  return <div className={checking ? 'ec-report ec-stale' : 'ec-report'}>
    <section className={`ec-summary ${verdict}`} aria-label="Итоги и реквизиты">
      <div className={`ec-verdict ${verdict}`} role="status">
        <span className="ec-verdict-icon" aria-hidden="true">{verdict === 'ok' ? '✓' : verdict === 'warn' ? '!' : '✕'}</span>
        <div>
          <b>{title}{report.warnings > 0 && <span className="ec-verdict-warn"> · {report.warnings} {plural(report.warnings, 'предупреждение', 'предупреждения', 'предупреждений')}</span>}</b>
          <span>Инвойс {report.invoiceNumber || 'без номера'}: {report.invoiceRows} {plural(report.invoiceRows, 'строка', 'строки', 'строк')} · в {files} {plural(files, 'экспортной', 'экспортных', 'экспортных')} {report.declarationItems} {plural(report.declarationItems, 'товар', 'товара', 'товаров')}</span>
          {unread.length > 0 && <span className="ec-verdict-unread">Не прочитан{unread.length > 1 ? 'ы' : ''}: {unread.join(', ')}. Уберите {unread.length > 1 ? 'эти файлы' : 'этот файл'} или загрузите правильный PDF.</span>}
        </div>
      </div>
      <div className="ec-cells">
        {report.totals.map((check) => <CheckCell key={check.label} check={check} />)}
        {report.details.map((check, index) => <CheckCell key={check.label} check={check} first={index === 0} wide />)}
      </div>
    </section>

    {(report.problems.length > 0 || report.notes.length > 0) && <div className="ec-messages">
      {report.problems.length > 0 && <ul className="ec-list error">{report.problems.map((text) => <li key={text}>{text}</li>)}</ul>}
      {report.notes.length > 0 && <ul className="ec-list warn">{report.notes.map((text) => <li key={text}>{text}</li>)}</ul>}
    </div>}

    <div className="ec-rows-head">
      <h2>Строки</h2>
      <div className="dir-seg ec-tabs" role="tablist" aria-label="Какие строки показать">
        {tabs.map(({ value, label }) => <button key={value} type="button" role="tab" aria-selected={tab === value}
          className={`ec-tab ec-tab-${value}${tab === value ? ' active' : ''}`} onClick={() => onTab(value)} disabled={value !== 'all' && counts[value] === 0}>
          {label} <span>{counts[value]}</span>
        </button>)}
      </div>
    </div>
    {rows.length === 0
      ? <p className="dir-empty ec-empty">Нет строк в этом списке.</p>
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

// Одна проверка документа: всегда обе стороны — инвойс и экспортная — подписанными строками, чтобы
// можно было сверить глазами; значения целиком (длинные переносятся). Итог — у названия: ✓ или
// «не сверяется»; расхождение — красная ячейка с пояснением. Реквизиты (wide) — шире чисел.
function CheckCell({ check, first, wide }: { check: Check; first?: boolean; wide?: boolean }) {
  const invoice = shownValue(check.label, check.invoice)
  const declaration = shownValue(check.label, check.declaration)
  const hint = `${check.label}\nИнвойс: ${invoice}\nЭкспортная: ${declaration}${check.note ? `\n${check.note}` : ''}`
  return <div className={`ec-cell ec-${check.status}${first ? ' ec-cell-group' : ''}${wide ? ' ec-cell-wide' : ''}`} title={hint}>
    <span className="ec-cell-label">
      {shortLabel[check.label] ?? check.label}{check.status === 'skip' && ' · не сверяется'}
      {check.status === 'ok' && <span className="ec-cell-mark" aria-label="совпадает">✓</span>}
    </span>
    <span className="ec-cell-side"><i title="Инвойс">И</i><b>{invoice}</b></span>
    <span className="ec-cell-side"><i title="Экспортная">Э</i><b>{declaration}</b></span>
    {check.note && check.status !== 'skip' && <small>{check.note}</small>}
  </div>
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
