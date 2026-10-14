import { ChangeEvent, Dispatch, SetStateAction, useEffect, useRef, useState } from 'react'
import { GoodsCalculationResult, GoodsItemInput, GoodsParams } from '@/shared/types/goods'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { reviewStatus, rowMissing } from '../ReviewRow/reviewStatus'
import { DEFAULT_PACKAGE_WEIGHT, formatWeight } from '../../lib/netWeight'
import { ReviewCheck } from '../../lib/reviewCheck'
import { reviewTotals } from './reviewTotals'
import { useStableCallback } from '@/shared/hooks/useStableCallback'
import { AgentRowStatus } from '../../agent/agentApi'
import { storageFailed } from '../../lib/storageNotice'
import { RowDecisionState } from '../../hooks/useRowDecisions'
import { emptyItem } from '../../lib/rowActions'
import { rowId } from '../../lib/excelColumns'
import { lookupNomenclature, withNomenclatureRates } from '../../lib/nomenclatureRates'
import { ReviewSheet, RowAction, SheetApi, SheetRowView } from '../ReviewSheet/ReviewSheet'
import { applySheetChanges, SheetChange } from '../ReviewSheet/sheetModel'
import '../../goods.css'
import './ReviewStep.css'

type Props = {
  params: GoodsParams
  items: GoodsItemInput[]
  result: GoodsCalculationResult | null
  // Ошибки последнего расчёта по id строк: показываются, пока нет нового расчёта.
  reviewCheck: ReviewCheck | null
  selectedForDirectory: string[]
  paramsOpen: boolean
  loading: boolean
  usdRateLoading: boolean
  usdRateNotice: string
  storageNotice: string
  deliverySummary: string
  feeSummary: string
  includedCount: number
  // Новые строки сметы из таблицы (правка, вставка, отмена); сбрасывает результат расчёта.
  editItems: (items: GoodsItemInput[]) => void
  setSelectedForDirectory: Dispatch<SetStateAction<string[]>>
  setParamsOpen: (value: boolean) => void
  setParam: (field: keyof GoodsParams, value: string) => void
  loadUsdRate: () => void
  openDirectoryPicker: (ids: string[]) => void
  removeRow: (id: string) => void
  // Последняя удалённая строка: её можно вернуть на место.
  removedRow: { rowNumber: number; name: string } | null
  restoreRemovedRow: () => void
  dismissRemovedRow: () => void
  openRowPhotos: (id: string, index: number) => void
  addManualPhoto: (id: string, event: ChangeEvent<HTMLInputElement>) => void
  addEmptyRow: () => void
  calculate: () => void
  saveDraft: () => void
  clearAll: () => void
  // Подбор кода ИИ: состояние по строкам, открыть чат строки, подобрать для всех строк без кода.
  agentStatuses: Record<string, AgentRowStatus>
  // База решений: состояние кнопки «В базу» строки и сохранение строк.
  rowDecisions: { stateOf: (item: GoodsItemInput) => RowDecisionState; save: (ids: string[]) => Promise<{ created: number; duplicates: number; skipped: number; failed: number }> }
  agentBatch: { done: number; total: number } | null
  openAgent: (id: string) => void
  startAgentBatch: () => void
}

type Filter = 'all' | 'attention' | 'noCode' | 'errors'
type Density = 'full' | 'compact'
const DENSITY_KEY = 'tradeflow.review-density'
// Выбор сотрудника запоминается; пока его нет — на низком экране (ноутбук) компактно.
const storedDensity = (): Density => {
  try {
    const saved = localStorage.getItem(DENSITY_KEY)
    if (saved === 'compact' || saved === 'full') return saved
  } catch { /* без памяти — по высоте окна */ }
  return window.innerHeight < 900 ? 'compact' : 'full'
}

const checkTime = (value: string) => {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}

// Отмена и повтор по строкам сметы (Ctrl+Z / Ctrl+Y в таблице): запоминается каждое изменение
// строк — правка ячеек, вставка, выбор кода, фото, удаление. mergeNext — следующее изменение
// дополняет последний шаг (ставки, подтянутые к введённому коду, не отдельный шаг отмены).
function useItemsHistory(items: GoodsItemInput[], replace: (items: GoodsItemInput[]) => void) {
  const state = useRef({ past: [] as GoodsItemInput[][], future: [] as GoodsItemInput[][], last: items, skip: false, merge: false })
  useEffect(() => {
    const history = state.current
    if (items === history.last) return
    if (!history.skip && !history.merge) {
      history.past.push(history.last)
      if (history.past.length > 100) history.past.shift()
      history.future = []
    }
    history.skip = false
    history.merge = false
    history.last = items
  }, [items])
  return {
    undo() {
      const history = state.current
      const previous = history.past.pop()
      if (!previous) return
      history.future.push(history.last)
      history.skip = true
      replace(previous)
    },
    redo() {
      const history = state.current
      const next = history.future.pop()
      if (!next) return
      history.past.push(history.last)
      history.skip = true
      replace(next)
    },
    mergeNext() { state.current.merge = true },
  }
}

export function ReviewStep({ params, items, result, reviewCheck, selectedForDirectory, paramsOpen, loading, usdRateLoading, usdRateNotice, storageNotice, deliverySummary, feeSummary, includedCount, editItems, setSelectedForDirectory, setParamsOpen, setParam, loadUsdRate, openDirectoryPicker, removeRow, removedRow, restoreRemovedRow, dismissRemovedRow, openRowPhotos, addManualPhoto, addEmptyRow, calculate, saveDraft, clearAll, agentStatuses, rowDecisions, agentBatch, openAgent, startAgentBatch }: Props) {
  const [filter, setFilter] = useState<Filter>('all')
  const [density, setDensity] = useState<Density>(storedDensity)
  const sheetApi = useRef<SheetApi | null>(null)
  const itemsRef = useRef(items)
  itemsRef.current = items
  const history = useItemsHistory(items, editItems)
  const excludedCount = items.length - includedCount
  const failed = storageFailed(storageNotice)
  const setSelectedExcluded = (excluded: boolean) => editItems(items.map((item) => selectedForDirectory.includes(item.id ?? '') ? { ...item, excluded } : item))

  // Статус каждой строки — для счётчиков и фильтра (строки считают его сами, чтобы memo работал).
  const rows = items.map((item, index) => {
    const id = item.id ?? ''
    const status = reviewStatus(item, result?.items[index], result ? undefined : reviewCheck?.rows[id], rowMissing(item, params.packageWeightKg))
    return { item, index, id, status }
  })
  const counts = {
    attention: rows.filter((row) => row.status.attention).length,
    noCode: rows.filter((row) => row.status.noCode).length,
    errors: rows.filter((row) => row.status.withErrors).length,
    changed: rows.filter((row) => row.status.tone === 'changed').length,
  }
  const shownInFilter = (status: (typeof rows)[number]['status']) => filter === 'all' || (filter === 'attention' ? status.attention : filter === 'noCode' ? status.noCode : status.withErrors)
  const visible = rows.filter((row) => shownInFilter(row.status))
  const visibleIds = visible.map((row) => row.id)
  const allVisibleSelected = visibleIds.length > 0 && visibleIds.every((id) => selectedForDirectory.includes(id))
  // Откуда ошибки: актуальный расчёт или прошлый (строки с тех пор менялись).
  const pastCheck = !result && reviewCheck && Object.keys(reviewCheck.rows).length > 0 ? reviewCheck : null

  // В листе все строки сметы по порядку; фильтр их только скрывает.
  const selectedIds = new Set(selectedForDirectory)
  const sheetRows: SheetRowView[] = rows.map(({ item, id, status }) => ({ item, status, visible: shownInFilter(status), selected: selectedIds.has(id), agent: agentStatuses[id], decision: rowDecisions.stateOf(item) }))

  // Правки листа → строки сметы, отметки строк и ставки для введённых кодов.
  function applyChanges(changes: SheetChange[]) {
    const current = itemsRef.current
    const applied = applySheetChanges(current, changes, params.packageWeightKg, (index) => rowId(0, index))
    if (applied.items) editItems(applied.items)
    if (applied.selected.size > 0) {
      setSelectedForDirectory((selected) => {
        const next = new Set(selected)
        applied.selected.forEach((checked, id) => { if (checked) next.add(id); else next.delete(id) })
        return [...next]
      })
    }
    if (applied.codes.size > 0) void loadCodeRates(applied.codes)
  }

  // Ставки введённого кода — пошлина ЕТТ и НДС из номенклатуры, как при выборе кода в справочнике.
  async function loadCodeRates(codes: Map<string, string>) {
    const found = await lookupNomenclature([...codes.values()]).catch(() => null)
    if (!found) return
    let changed = false
    const next = itemsRef.current.map((item) => {
      const code = codes.get(item.id ?? '')
      const nomenclature = code && item.code === code ? found.byCode.get(code) : undefined
      if (!nomenclature) return item
      changed = true
      const snapshot = withNomenclatureRates(nomenclature, found.source, { code })
      return { ...item, standardName: String(snapshot.name ?? ''), directorySnapshot: snapshot }
    })
    if (!changed) return
    history.mergeNext()
    editItems(next)
  }

  // Фото добавляется скрытым полем выбора файла: кнопка «+» в ячейке открывает его для строки.
  const photoInput = useRef<HTMLInputElement>(null)
  const photoTarget = useRef('')
  function rowAction(action: RowAction, index: number) {
    const id = items[index]?.id
    if (!id) return
    if (action === 'photos') openRowPhotos(id, 0)
    else if (action === 'upload') { photoTarget.current = id; photoInput.current?.click() }
    else if (action === 'picker') openDirectoryPicker([id])
    else if (action === 'agent') openAgent(id)
    else if (action === 'decision') void rowDecisions.save([id])
  }

  function insertRow(index: number) {
    const current = itemsRef.current
    editItems([...current.slice(0, index), emptyItem(rowId(0, current.length), params.packageWeightKg), ...current.slice(index)])
  }

  // Одна строка — с сообщением «Вернуть»; несколько — одним шагом, вернуть можно Ctrl+Z.
  function deleteRows(indexes: number[]) {
    const ids = indexes.map((index) => items[index]?.id).filter((id): id is string => Boolean(id))
    if (ids.length === 1) { removeRow(ids[0]); return }
    if (ids.length === 0 || !window.confirm(`Удалить строки: ${ids.length}? Вернуть можно Ctrl+Z.`)) return
    dismissRemovedRow()
    editItems(itemsRef.current.filter((item) => !ids.includes(item.id ?? '')))
    setSelectedForDirectory((current) => current.filter((id) => !ids.includes(id)))
  }

  function toggleAllVisible() {
    setSelectedForDirectory((current) => allVisibleSelected ? current.filter((id) => !visibleIds.includes(id)) : [...new Set([...current, ...visibleIds])])
  }

  // Итог сохранения отмеченных строк в базу решений.
  const [decisionNote, setDecisionNote] = useState('')
  async function saveSelectedDecisions() {
    setDecisionNote('Сохраняем в базу решений…')
    const counts = await rowDecisions.save(selectedForDirectory)
    setDecisionNote([
      `В базу решений: сохранено ${counts.created}`,
      counts.duplicates > 0 && `уже были в этой смете ${counts.duplicates}`,
      counts.skipped > 0 && `без кода или уже в базе ${counts.skipped}`,
      counts.failed > 0 && `ошибка ${counts.failed}`,
    ].filter(Boolean).join(', '))
  }
  const agentPending = items.filter((item) => item.id && !item.excluded && !item.code && (!agentStatuses[item.id] || agentStatuses[item.id].status === 'ready')).length
  const agentStarted = counts.noCode - agentPending
  const ownPackageCount = items.filter((item) => item.packageWeightKg !== params.packageWeightKg).length
  // Колонку упаковки прячем, пока она ни на что не влияет: нетто есть во всех строках расчёта,
  // а общую упаковку не меняли и своей у строк нет. Ввели свою упаковку — колонка появляется.
  const packageUnused = items.every((item) => item.excluded || item.netKg.trim() !== '')
    && params.packageWeightKg === DEFAULT_PACKAGE_WEIGHT && ownPackageCount === 0
  const noFee = params.feeMultiplier.trim() !== '' && Number(params.feeMultiplier) === 0
  const totals = reviewTotals(items, params.packageWeightKg)
  const total = (value: number) => formatRuNumber(formatWeight(value).replace(/\s/g, ''))

  function chooseDensity(value: Density) {
    setDensity(value)
    try { localStorage.setItem(DENSITY_KEY, value) } catch { /* без памяти — выбор на эту сессию */ }
  }

  // Следующая строка, требующая внимания, после строки с курсором (по кругу): прокрутить и
  // поставить курсор в название.
  function nextProblem() {
    const problems = visible.filter((row) => row.status.attention).map((row) => row.index)
    if (problems.length === 0) return
    const at = sheetApi.current?.currentRow() ?? -1
    sheetApi.current?.focusRow(problems.find((index) => index > at) ?? problems[0])
  }

  // Ctrl+S — сохранить сейчас, F8 — следующая строка, требующая внимания.
  const keysRef = useRef({ saveDraft, nextProblem })
  keysRef.current = { saveDraft, nextProblem }
  useEffect(() => {
    const onKey = (event: globalThis.KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === 's') { event.preventDefault(); keysRef.current.saveDraft() }
      else if (event.key === 'F8') { event.preventDefault(); keysRef.current.nextProblem() }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  // Сообщение об удалённой строке исчезает само через 12 секунд.
  const onDismissRemoved = useStableCallback(dismissRemovedRow)
  useEffect(() => {
    if (!removedRow) return
    const timer = window.setTimeout(onDismissRemoved, 12000)
    return () => window.clearTimeout(timer)
  }, [removedRow, onDismissRemoved])

  const segment = (value: Filter, label: string, count: number, hint: string) => (
    <button type="button" role="radio" aria-checked={filter === value} className={filter === value ? 'active' : undefined} onClick={() => setFilter(value)} disabled={count === 0 && filter !== value} title={hint}>{label} <b>{count}</b></button>
  )

  return <div className={`goods-step-page rv-step${density === 'compact' ? ' compact' : ''}`}>
    <details className="goods-params goods-params-top" open={paramsOpen} onToggle={(event) => setParamsOpen(event.currentTarget.open)}>
      <summary>Параметры расчёта <span className="goods-params-summary">курс {params.usdKzt ? formatRuNumber(params.usdKzt) : '—'} · доставка: {deliverySummary} · сбор: {feeSummary}{params.customsValueMode === 'sir' && ` · ТС по СУР${Number(params.sirDiscountPercent) > 0 ? ` −${params.sirDiscountPercent}%` : ''}`}</span></summary>
      <div className="rv-params">
        <div className="rv-params-group">
          <label className="rv-field">Дата расчёта<input className="rv-w-date" type="date" value={params.assessmentDate} onChange={(event) => setParam('assessmentDate', event.target.value)} required /></label>
          <label className="rv-field">Страна<input className="rv-w-country" value={params.origin} onChange={(event) => setParam('origin', event.target.value)} placeholder="Например, Китай" /></label>
        </div>
        <div className="rv-params-group">
          <div className="rv-field"><span>Курс USD → KZT{usdRateNotice && <small title={`Источник: ${usdRateNotice}`}>✓ {usdRateNotice}</small>}</span><span className="rv-joined"><input className="rv-w-md" inputMode="decimal" value={params.usdKzt} onChange={(event) => setParam('usdKzt', event.target.value.replace(',', '.'))} placeholder="обязательно" required aria-label="Курс USD → KZT" /><button type="button" className="rv-rate-button" onClick={() => void loadUsdRate()} disabled={usdRateLoading || !params.assessmentDate} title="Взять официальный курс НБРК на дату расчёта">{usdRateLoading ? '…' : 'НБРК'}</button></span></div>
        </div>
        <div className="rv-params-group">
          <div className="rv-field"><span>{params.deliveryMode === 'wagons' ? 'Доставка: вагоны × тариф' : params.deliveryMode === 'none' ? 'Доставка: не учитывается' : 'Доставка: сумма за партию'}</span><span className="rv-joined">
            <select value={params.deliveryMode} onChange={(event) => setParam('deliveryMode', event.target.value)} aria-label="Режим доставки"><option value="wagons">По вагонам</option><option value="total">Общая сумма</option><option value="none">Без доставки</option></select>
            {params.deliveryMode === 'wagons' ? <>
              <input className="rv-w-sm" inputMode="decimal" value={params.wagons} onChange={(event) => setParam('wagons', event.target.value.replace(',', '.'))} placeholder="ваг." aria-label="Количество вагонов" />
              <span className="rv-joined-sep">×</span>
              <input className="rv-w-md" inputMode="decimal" value={params.wagonTariffUsd} onChange={(event) => setParam('wagonTariffUsd', event.target.value.replace(',', '.'))} placeholder="тариф" aria-label="Тариф за вагон, USD" />
            </> : params.deliveryMode === 'none' ? null : <input className="rv-w-md" inputMode="decimal" value={params.deliveryTotalUsd} onChange={(event) => setParam('deliveryTotalUsd', event.target.value.replace(',', '.'))} placeholder="сумма" aria-label="Общая доставка за партию, USD" />}
            {params.deliveryMode !== 'none' && <span className="rv-joined-unit">USD</span>}
          </span></div>
        </div>
        <div className="rv-params-group">
          <label className="rv-field" title="Вес одной упаковки (на одно место) — для автоматического расчёта нетто. Меняет все строки, кроме тех, где указана своя упаковка"><span>Упаковка, кг{ownPackageCount > 0 && <small title="Строки со своей упаковкой не меняются вместе с общим значением">своя: {ownPackageCount}</small>}</span><input className="rv-w-sm" inputMode="decimal" value={params.packageWeightKg} onChange={(event) => setParam('packageWeightKg', event.target.value.replace(',', '.'))} /></label>
        </div>
        <div className="rv-params-group">
          {/* «Без сбора» — это множитель 0: сбор не считается и не входит в базу НДС. Снятие галочки возвращает множитель 6. */}
          <div className="rv-field" title="Сбор = МРП × множитель, делится поровну между строками"><span>Сбор: МРП × множитель<label className="rv-nofee"><input type="checkbox" checked={noFee} onChange={(event) => setParam('feeMultiplier', event.target.checked ? '0' : '6')} /> без сбора</label></span><span className={noFee ? 'rv-joined off' : 'rv-joined'}>
            <input className="rv-w-md" inputMode="decimal" value={params.mrp} onChange={(event) => setParam('mrp', event.target.value.replace(',', '.'))} placeholder="МРП" aria-label="МРП, KZT" disabled={noFee} />
            <span className="rv-joined-sep">×</span>
            <input className="rv-w-sm" inputMode="decimal" value={params.feeMultiplier} onChange={(event) => setParam('feeMultiplier', event.target.value.replace(',', '.'))} placeholder="6" aria-label="Множитель сбора" disabled={noFee} />
            <span className="rv-joined-unit">₸</span>
          </span></div>
        </div>
      </div>
    </details>
    <section className="rv-card" aria-label="Строки сметы">
      <div className="rv-bar">
        {selectedForDirectory.length > 0 ? <>
          <strong>Выбрано: {selectedForDirectory.length}</strong>
          <button type="button" className="rv-bar-button primary" onClick={() => openDirectoryPicker(selectedForDirectory)}>Товар или код ТН ВЭД</button>
          <button type="button" className="rv-bar-button" onClick={() => void saveSelectedDecisions()} title="Записать отмеченные строки с кодом в базу решений; тот же товар этой сметы записывается один раз">В базу решений</button>
          <button type="button" className="rv-bar-button" onClick={() => setSelectedExcluded(true)}>Исключить из расчёта</button>
          <button type="button" className="rv-bar-button" onClick={() => setSelectedExcluded(false)}>Включить</button>
          <button type="button" className="button-link" onClick={() => { setSelectedForDirectory([]); setDecisionNote('') }}>Снять выбор</button>
          {decisionNote && <span className="rv-bar-note" role="status">{decisionNote}</span>}
        </> : <>
          <span>Строк: <b>{items.length}</b> · в расчёте: <b>{includedCount}</b>{excludedCount > 0 && <> · исключено: <b>{excludedCount}</b></>}</span>
          {agentBatch ? <span className="rv-bar-note">Запуск подбора: {agentBatch.done} из {agentBatch.total}</span>
            : agentPending > 0 && <button type="button" className="rv-bar-button rv-agent-batch" onClick={startAgentBatch} disabled={loading} title={`Подобрать коды для включённых строк без кода, где подбор ещё не запускали${agentStarted > 0 ? ` (ещё ${agentStarted} уже в подборе)` : ''}; результат — в чате строки`}>Подобрать коды ({agentPending}){agentStarted > 0 && <span className="rv-bar-sub"> · ещё {agentStarted} в подборе</span>}</button>}
          <span className="rv-bar-end">
            <button type="button" className="rv-bar-button" onClick={addEmptyRow} disabled={loading}>+ Строка</button>
          </span>
        </>}
      </div>

      <div className="rv-filterbar">
        <div className="rv-seg" role="radiogroup" aria-label="Какие строки показать">
          {segment('all', 'Все', items.length, 'Все строки сметы')}
          {segment('attention', 'Требуют внимания', counts.attention, 'Ошибки и предупреждения расчёта, строки без кода, изменённые после расчёта строки с ошибками')}
          {segment('noCode', 'Без кода', counts.noCode, 'Включённые строки без кода ТН ВЭД')}
          {segment('errors', 'С ошибками', counts.errors, 'Строки с ошибками расчёта, в том числе изменённые, но ещё не пересчитанные')}
        </div>
        <button type="button" className="rv-next" onClick={nextProblem} disabled={counts.attention === 0} title="Следующая строка, требующая внимания (F8)">Следующая проблема <kbd>F8</kbd></button>
        <p className={pastCheck ? 'rv-check-source past' : 'rv-check-source'} role="status" title={result ? 'Статусы строк — по актуальному расчёту' : pastCheck ? 'Строки менялись после расчёта: ошибки — из прошлого расчёта, нужен пересчёт' : 'Расчёта ещё не было: видны только строки без кода и недостающие данные'}>
          {result ? 'Актуальный расчёт'
            : pastCheck ? <>Прошлый расчёт{checkTime(pastCheck.at) && ` ${checkTime(pastCheck.at)}`}{counts.changed > 0 && <> · изменено: <b>{counts.changed}</b></>} · нужен пересчёт</>
              : 'Расчёта ещё не было'}
        </p>
        <div className="rv-seg rv-density" role="radiogroup" aria-label="Плотность таблицы">
          <button type="button" role="radio" aria-checked={density === 'full'} className={density === 'full' ? 'active' : undefined} onClick={() => chooseDensity('full')} title="Фото, название и код в две строки">Подробно</button>
          <button type="button" role="radio" aria-checked={density === 'compact'} className={density === 'compact' ? 'active' : undefined} onClick={() => chooseDensity('compact')} title="Одна строка на товар: больше строк на экране">Компактно</button>
        </div>
      </div>

      {removedRow && <div className="rv-undo" role="status">
        <span>Строка {removedRow.rowNumber}{removedRow.name ? ` «${removedRow.name.length > 60 ? `${removedRow.name.slice(0, 60)}…` : removedRow.name}»` : ''} удалена</span>
        <button type="button" className="button-link" onClick={restoreRemovedRow}>Вернуть</button>
        <button type="button" className="rv-undo-close" onClick={dismissRemovedRow} aria-label="Закрыть сообщение">×</button>
      </div>}

      {items.length === 0 && <p className="rv-empty-filter">Строк пока нет. Загрузите XLSX или нажмите «+ Строка» — в неё можно вставить строки из Excel (Ctrl+V).</p>}
      {items.length > 0 && visible.length === 0 && <p className="rv-empty-filter">В этом фильтре строк нет. <button type="button" className="button-link" onClick={() => setFilter('all')}>Показать все строки</button></p>}
      <ReviewSheet rows={sheetRows} defaultPackageWeight={params.packageWeightKg} hidePackage={packageUnused} compact={density === 'compact'} apiRef={sheetApi}
        onChanges={applyChanges} onAction={rowAction} onSelectAll={toggleAllVisible} onInsertRow={insertRow} onDeleteRows={deleteRows}
        onPickRows={(indexes) => openDirectoryPicker(indexes.map((index) => items[index]?.id ?? '').filter(Boolean))} onUndo={history.undo} onRedo={history.redo} />
      <input ref={photoInput} type="file" accept="image/jpeg,image/png" style={{ display: 'none' }} onChange={(event) => { if (photoTarget.current) addManualPhoto(photoTarget.current, event) }} />
      {items.length > 0 && <p className="rv-totals">
        <b>Итого</b> <small>по строкам в расчёте{filter !== 'all' ? ', не только показанным' : ''}:</small>
        <span>места <b>{total(totals.places)}</b></span>
        <span>кол-во <b>{total(totals.quantity)}</b> шт</span>
        <span>брутто <b>{total(totals.grossKg)}</b> кг</span>
        <span>нетто <b>{total(totals.netKg)}</b> кг</span>
      </p>}
      <div className="rv-actions">
        <span className={failed ? 'goods-storage-error' : 'rv-actions-note'} role={failed ? 'alert' : 'status'} title={storageNotice}>{storageNotice || 'Изменения сохраняются автоматически'}</span>
        <details className="rv-more">
          <summary>Ещё</summary>
          <div className="rv-more-menu">
            <button type="button" className="rv-clear" onClick={(event) => { event.currentTarget.closest('details')?.removeAttribute('open'); clearAll() }} disabled={loading || items.length === 0}>Очистить смету…</button>
          </div>
        </details>
        <button type="button" className="btn-save-draft" onClick={() => void saveDraft()} disabled={loading || items.length === 0} title="Сохранить черновик на сервере сейчас (Ctrl+S)">Сохранить черновик</button>
        <button type="button" className="btn-calculate" onClick={() => void calculate()} disabled={loading || includedCount === 0}>{loading ? 'Считаем…' : 'Рассчитать →'}</button>
      </div>
    </section>
  </div>
}
