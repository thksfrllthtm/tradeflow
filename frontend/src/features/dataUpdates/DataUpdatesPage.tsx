import { ChangeEvent, ReactNode, useEffect, useState } from 'react'
import { AppHeader } from '@/shared/components/AppHeader/AppHeader'
import { PageHead } from '@/shared/components/PageHead/PageHead'
import { request } from '@/shared/api/client'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatDisplayNumber } from '@/shared/lib/formatDisplayNumber'
import { formatRuDate } from '@/shared/lib/dates'
import { plural } from '@/shared/lib/plural'
import { User } from '@/shared/types/user'
import { NomenclatureImport } from '@/shared/tnved/nomenclature'
import { formatCode } from '@/shared/tnved/codes'
import { TrademarkImport, TrademarkSummary, trademarksApi } from '@/shared/trademarks/trademarks'
import { DirectoryImportResult } from '@/features/directories/common/directoryTransfer'
import { NomenclatureChange, NomenclatureUpload, nomenclatureApi } from '@/features/directories/nomenclature/nomenclatureApi'
import { AntiDumpingImport, antiDumpingApi } from '@/features/directories/nomenclature/antiDumping'
import { KzImport, KzImportResult } from '@/features/directories/kzList/kzListApi'
import { kzImportText, useKzListUpload } from '@/features/directories/kzList/KzListPanel'
import { useRiskIndicatorsUpload } from '@/features/directories/riskIndicators/RiskIndicatorsPanel'
import '@/shared/styles/list-page.css'
import '@/features/directories/components/DirectoryTransfer.css'
import '@/features/directories/kzList/KzListPanel.css'
import './DataUpdatesPage.css'

// Откуда скачивать данные. ponytail: у КГД нет постоянной ссылки на файлы СУР и ТРОИС — ведём на главную сайта.
const links = {
  nomenclature: 'https://www.tws.by/tws/tnved/download',
  kzList: 'https://eec.eaeunion.org/comission/department/catr/ttr/stavki.php',
  antiDumping: 'https://remedies.eaeunion.org/dimd/ru/security/actions',
  kgd: 'https://kgd.gov.kz/',
}

// Результат последнего обновления набора — сервер хранит ответ загрузки (data_update_results). Для антидемпинга —
// что изменилось в реестре ЕЭК. СУР не хранится: дата и объём каждого профиля и так видны в таблице профилей.
type MeasureRef = { number: string; shortName: string }
type MeasureChanges = { measureCount: number; added: MeasureRef[]; removed: MeasureRef[]; changed: MeasureRef[] }
type Results = { nomenclature: NomenclatureUpload; kz: KzImportResult; antidumping: MeasureChanges; trademarks: TrademarkImport }
type Dataset = keyof Results
type Last = { [K in Dataset]?: { result: Results[K]; updatedAt: string; updatedByName: string } }
// Сервер хранит ответ загрузки номенклатуры как есть — { import, changes }; страница работает с плоским видом, как nomenclatureApi.upload.
type Saved = Omit<Last, 'nomenclature'> & { nomenclature?: { result: { import: NomenclatureImport; changes?: NomenclatureChange[] }; updatedAt: string; updatedByName: string } }
const loadSaved = () => request<Saved>('/api/data-updates').then(({ nomenclature, ...rest }): Last =>
  nomenclature ? { ...rest, nomenclature: { ...nomenclature, result: { ...nomenclature.result.import, changes: nomenclature.result.changes ?? [] } } } : rest)

const codes = (count: number) => `${formatDisplayNumber(count)} ${plural(count, 'код', 'кода', 'кодов')}`
const changeLabels = { added: 'Новый', removed: 'Удалён', changed: 'Изменён' }
const changeTones = { added: 'ok', removed: 'error', changed: 'warn' }

// «04.11.2026, admin · подробности» или «Ещё не загружено».
function updated(at?: string, by?: string, details?: ReactNode) {
  if (!at) return 'Ещё не загружено.'
  return <><time dateTime={at}>{formatRuDate(at.slice(0, 10))}</time>{by && `, ${by}`}{details && <> · {details}</>}</>
}

// Строка набора данных: название и дата обновления, источник, кнопка; под строкой — сообщения и подробности.
function Row({ title, meta, source, sourceTitle, action, children }: { title: string; meta: ReactNode; source: string; sourceTitle: string; action: ReactNode; children?: ReactNode }) {
  return <li className="du-row">
    <div className="du-main"><b>{title}</b><span className="du-meta">{meta}</span></div>
    <a className="du-source" href={source} target="_blank" rel="noreferrer" title={sourceTitle}>{new URL(source).hostname.replace(/^www\./, '')} ↗</a>
    <div className="du-action">{action}</div>
    {children && <div className="du-extra">{children}</div>}
  </li>
}

function FileButton({ busy, label, accept, title, onChange }: { busy: string; label: string; accept: string; title: string; onChange: (event: ChangeEvent<HTMLInputElement>) => void }) {
  return <label className={busy ? 'kz-file disabled' : 'kz-file'} title={title}>
    {busy || label}
    <input type="file" accept={accept} onChange={onChange} disabled={busy !== ''} />
  </label>
}

// Строка изменения: у изменённого кода — что было и что стало (только то, что поменялось).
function ChangeRow({ change }: { change: NomenclatureChange }) {
  const nameChanged = change.kind === 'changed' && change.oldName !== change.name
  const rateChanged = change.kind === 'changed' && change.oldTariffText !== change.tariffText
  return <li className="nom-change">
    <span className={`badge ${changeTones[change.kind]} nom-change-kind`}>{changeLabels[change.kind]}</span>
    <code>{formatCode(change.code)}</code>
    <span className="nom-change-text">
      {change.kind === 'changed'
        ? <>{nameChanged ? <><s>{change.oldName}</s> → {change.name}</> : change.name}{rateChanged && <small>Ставка: <s>{change.oldTariffText || '—'}</s> → {change.tariffText || '—'}</small>}</>
        : <>{change.kind === 'added' ? change.name : change.oldName}<small>Ставка: {(change.kind === 'added' ? change.tariffText : change.oldTariffText) || '—'}</small></>}
    </span>
  </li>
}

const ok = (text: string) => text && <p className="dir-transfer-ok" role="status">{text}</p>
const fail = (text: string) => text && <p className="dir-transfer-error" role="alert">{text}</p>

function NomenclatureResult({ result }: { result: NomenclatureUpload }) {
  const total = result.addedCount + result.removedCount + result.changedCount
  return <>
    {ok(`Загружено ${codes(result.codeCount)}: новых ${formatDisplayNumber(result.addedCount)}, удалено ${formatDisplayNumber(result.removedCount)}, изменились наименование или ставка — ${formatDisplayNumber(result.changedCount)}.`)}
    {result.changes.length > 0 && <ul className="nom-changes">
      {result.changes.map((change) => <ChangeRow key={change.code} change={change} />)}
      {result.changes.length < total && <li className="nom-change-more">Показаны первые {formatDisplayNumber(result.changes.length)} из {formatDisplayNumber(total)}</li>}
    </ul>}
  </>
}

function AntiDumpingResult({ result }: { result: MeasureChanges }) {
  const rows = (['added', 'removed', 'changed'] as const).flatMap((kind) => result[kind].map((measure) => ({ kind, measure })))
  return <>
    {ok(`Действующих мер — ${result.measureCount}: новых ${result.added.length}, снято ${result.removed.length}, изменились ${result.changed.length}.`)}
    {rows.length > 0 && <ul className="nom-changes">{rows.map(({ kind, measure }) => <li key={`${kind}-${measure.number}`} className="nom-change">
      <span className={`badge ${changeTones[kind]} nom-change-kind`}>{{ added: 'Новая', removed: 'Снята', changed: 'Изменена' }[kind]}</span>
      <code>{measure.number}</code>
      <span className="nom-change-text">{measure.shortName}</span>
    </li>)}</ul>}
  </>
}

function trademarksText(result: TrademarkImport) {
  const notes = [
    `${formatDisplayNumber(result.excluded)} исключённых пропущено`,
    result.badLogos.length > 0 ? `логотипы не прочитались: ${result.badLogos.join(', ')}` : '',
    result.skipped.length > 0 ? `пропущены строки — ${result.skipped.join('; ')}` : '',
  ].filter(Boolean)
  return `Загружено ${formatDisplayNumber(result.summary.total)} действующих записей, из них ${formatDisplayNumber(result.summary.logos)} с логотипом; ${notes.join('; ')}. Логотипы без надписи ИИ подпишет в фоне.`
}

function takeFile(event: ChangeEvent<HTMLInputElement>) {
  const file = event.target.files?.[0]
  event.target.value = ''
  return file
}

// Обновление данных из внешних источников в одном месте: номенклатура, перечень РК, антидемпинг, СУР, ТРОИС.
// Только администратор: загрузки и результаты сервер закрывает requireAdmin.
export function DataUpdatesPage({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [nomenclature, setNomenclature] = useState<NomenclatureImport | null>(null)
  const [kz, setKz] = useState<KzImport | null>(null)
  const [antiDumping, setAntiDumping] = useState<AntiDumpingImport | null>(null)
  const [trademarks, setTrademarks] = useState<TrademarkSummary | null>(null)
  const [last, setLast] = useState<Last>({})
  const [open, setOpen] = useState<Partial<Record<Dataset, boolean>>>({})
  const [error, setError] = useState('')

  const [nomBusy, setNomBusy] = useState('')
  const [nomError, setNomError] = useState('')
  const [nomRowErrors, setNomRowErrors] = useState<DirectoryImportResult['errors']>([])
  const [adBusy, setAdBusy] = useState(false)
  const [adError, setAdError] = useState('')
  const [tmBusy, setTmBusy] = useState('')
  const [tmError, setTmError] = useState('')

  // Новый результат заменяет прежний и сразу раскрывается.
  function remember<K extends Dataset>(dataset: K, result: Results[K]) {
    setLast((current) => ({ ...current, [dataset]: { result, updatedAt: new Date().toISOString(), updatedByName: user.name } }))
    setOpen((current) => ({ ...current, [dataset]: true }))
  }

  const loadNomenclature = () => nomenclatureApi.list({ q: '', offset: 0, limit: 1 })
    .then((page) => { setNomenclature(page.import); setKz(page.kzImport) })
  const kzUpload = useKzListUpload({ onImported: (result) => {
    remember('kz', result)
    void loadNomenclature().catch((reason) => setError(errorMessage(reason)))
  } })
  const sir = useRiskIndicatorsUpload()

  useEffect(() => {
    Promise.all([
      loadNomenclature(),
      antiDumpingApi.list().then((list) => setAntiDumping(list.import)),
      trademarksApi.list('', false, 0).then((body) => setTrademarks(body.summary)),
      loadSaved().then(setLast),
    ]).catch((reason) => setError(errorMessage(reason)))
  }, [])

  async function uploadNomenclature(event: ChangeEvent<HTMLInputElement>) {
    const file = takeFile(event)
    if (!file) return
    setNomBusy('Загружаем…'); setNomError(''); setNomRowErrors([])
    try {
      const result = await nomenclatureApi.upload(file)
      setNomenclature({ ...result, importedByName: result.importedByName || user.name }); remember('nomenclature', result)
    } catch (reason) {
      setNomError(errorMessage(reason))
      setNomRowErrors((reason as { rows?: DirectoryImportResult['errors'] }).rows ?? [])
    } finally { setNomBusy('') }
  }

  async function refreshAntiDumping() {
    setAdBusy(true); setAdError('')
    try {
      await antiDumpingApi.refresh()
      // Что изменилось, считает и сохраняет сервер — берём оттуда.
      const [list, saved] = await Promise.all([antiDumpingApi.list(), loadSaved()])
      setAntiDumping(list.import)
      if (saved.antidumping) { setLast((current) => ({ ...current, antidumping: saved.antidumping })); setOpen((current) => ({ ...current, antidumping: true })) }
    } catch (reason) { setAdError(errorMessage(reason)) } finally { setAdBusy(false) }
  }

  async function uploadTrademarks(event: ChangeEvent<HTMLInputElement>) {
    const file = takeFile(event)
    if (!file) return
    setTmBusy('Загружаем…'); setTmError('')
    try {
      const result = await trademarksApi.upload(file)
      setTrademarks(result.summary); remember('trademarks', result)
    } catch (reason) { setTmError(errorMessage(reason)) } finally { setTmBusy('') }
  }

  // Ссылка в строке: раскрыть или скрыть результат последнего обновления.
  const toggle = (dataset: Dataset) => last[dataset] && <>{' · '}<button type="button" className="button-link" onClick={() => setOpen((current) => ({ ...current, [dataset]: !current[dataset] }))} aria-expanded={!!open[dataset]}>{open[dataset] ? 'Скрыть изменения' : 'Что изменилось'}</button></>
  const shown = <K extends Dataset>(dataset: K) => open[dataset] ? last[dataset]?.result : undefined

  const nomResult = shown('nomenclature')
  const kzResult = shown('kz')
  const adResult = shown('antidumping')
  const tmResult = shown('trademarks')
  return <main className="app-shell directory-shell">
    <AppHeader user={user} section="dataUpdates" onLogout={onLogout} />
    <section className="directory-page du-page">
      <PageHead title="Обновление данных" subtitle="Когда обновлялись справочники, что изменилось в последний раз, где взять свежий файл и загрузка." />
      {fail(error)}

      <ul className="du-list">
        <Row title="Номенклатура ТН ВЭД ЕАЭС" source={links.nomenclature} sourceTitle="Excel номенклатуры на tws.by"
          meta={<>{updated(nomenclature?.importedAt, nomenclature?.importedByName, nomenclature && codes(nomenclature.codeCount))}{toggle('nomenclature')}</>}
          action={<FileButton busy={nomBusy} label="Загрузить .xlsx" accept=".xlsx" title="Excel с tws.by: заменяет номенклатуру целиком, покажет изменённые коды" onChange={(event) => void uploadNomenclature(event)} />}>
          {nomResult && <NomenclatureResult result={nomResult} />}
          {fail(nomError)}
          {nomRowErrors.length > 0 && <ul className="dir-transfer-errors">{nomRowErrors.map((item) => <li key={`${item.row}-${item.message}`}><b>Строка {item.row}</b> {item.message}</li>)}</ul>}
        </Row>

        <Row title="Перечень РК — ставки ниже ЕТТ" source={links.kzList} sourceTitle="Сайт ЕЭК: «Применение ставок, отличных от ставок ЕТТ»"
          meta={<>{updated(kz?.importedAt, kz?.importedByName, kz && codes(kz.rowCount))}{toggle('kz')}</>}
          action={kzUpload.action}>
          {kzResult && ok(kzImportText(kzResult))}
          {kzUpload.notes}
        </Row>

        <Row title="Антидемпинговые меры ЕАЭС" source={antiDumping?.registryUrl || links.antiDumping} sourceTitle="Реестр мер ЕЭК"
          meta={<>{antiDumping?.source === 'seed' ? 'С сайта ЕЭК ещё не обновлялось' : updated(antiDumping?.importedAt, antiDumping?.importedByName, antiDumping && `мер: ${antiDumping.measureCount}`)}{toggle('antidumping')}</>}
          action={<button type="button" className="secondary" onClick={() => void refreshAntiDumping()} disabled={adBusy}>{adBusy ? 'Обновляем…' : 'Обновить с сайта ЕЭК'}</button>}>
          {adResult && <AntiDumpingResult result={adResult} />}
          {fail(adError)}
        </Row>

        <Row title="СУР — индикаторы риска КГД" source={links.kgd} sourceTitle="Сайт КГД: файлы «24_прил CN», «25_прил CN», «26_прил CN»"
          meta={sir.meta} action={sir.action}>
          {sir.extra}
        </Row>

        <Row title="ТРОИС — реестр интеллектуальной собственности" source={links.kgd} sourceTitle="Сайт КГД: файл реестра"
          meta={<>{updated(trademarks?.importedAt || undefined, trademarks?.importedByName, trademarks && trademarks.total > 0 && `записей: ${formatDisplayNumber(trademarks.total)}`)}{toggle('trademarks')}</>}
          action={<FileButton busy={tmBusy} label="Загрузить файл" accept=".xls,.xlsx" title="Файл КГД как есть (.xls), без пересохранения — так логотипы сохраняются в исходном качестве. Заменяет реестр целиком" onChange={(event) => void uploadTrademarks(event)} />}>
          {tmResult && ok(trademarksText(tmResult))}
          {fail(tmError)}
        </Row>
      </ul>
    </section>
  </main>
}
