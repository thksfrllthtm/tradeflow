import { Dispatch, RefObject, SetStateAction, useEffect, useRef, useState } from 'react'
import { GoodsCalculationResult, GoodsItemInput, GoodsParams } from '@/shared/types/goods'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { ReviewRow } from '../ReviewRow/ReviewRow'
import { reviewStatus, rowMissing } from '../ReviewRow/reviewStatus'
import { formatWeight } from '../../lib/netWeight'
import { ReviewCheck } from '../../lib/reviewCheck'
import { reviewTotals } from './reviewTotals'
import { useStableCallback } from '@/shared/hooks/useStableCallback'
import { AgentRowStatus } from '../../agent/agentApi'
import { storageFailed } from '../../lib/storageNotice'
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
  tableWrapRef: RefObject<HTMLDivElement | null>
  topScrollRef: RefObject<HTMLDivElement | null>
  includedCount: number
  allDirectorySelected: boolean
  setItems: Dispatch<SetStateAction<GoodsItemInput[]>>
  setSelectedForDirectory: Dispatch<SetStateAction<string[]>>
  setParamsOpen: (value: boolean) => void
  setParam: (field: keyof GoodsParams, value: string) => void
  loadUsdRate: () => void
  syncHorizontalScroll: (source: 'top' | 'body') => void
  openDirectoryPicker: (ids: string[]) => void
  clearRowDirectory: (id: string) => void
  setItem: (id: string, field: keyof GoodsItemInput, value: string | boolean | undefined) => void
  removeRow: (id: string) => void
  // Последняя удалённая строка: её можно вернуть на место.
  removedRow: { rowNumber: number; name: string } | null
  restoreRemovedRow: () => void
  dismissRemovedRow: () => void
  openRowPhotos: (id: string, index: number) => void
  addManualPhoto: (id: string, event: React.ChangeEvent<HTMLInputElement>) => void
  resetAutomaticNet: (id: string) => void
  addEmptyRow: () => void
  calculate: () => void
  saveDraft: () => void
  clearAll: () => void
  // Подбор кода ИИ: состояние по строкам, открыть чат строки, подобрать для всех строк без кода.
  agentStatuses: Record<string, AgentRowStatus>
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

export function ReviewStep({ params, items, result, reviewCheck, selectedForDirectory, paramsOpen, loading, usdRateLoading, usdRateNotice, storageNotice, deliverySummary, feeSummary, tableWrapRef, topScrollRef, includedCount, allDirectorySelected, setItems, setSelectedForDirectory, setParamsOpen, setParam, loadUsdRate, syncHorizontalScroll, openDirectoryPicker, clearRowDirectory, setItem, removeRow, removedRow, restoreRemovedRow, dismissRemovedRow, openRowPhotos, addManualPhoto, resetAutomaticNet, addEmptyRow, calculate, saveDraft, clearAll, agentStatuses, agentBatch, openAgent, startAgentBatch }: Props) {
  const [filter, setFilter] = useState<Filter>('all')
  const [density, setDensity] = useState<Density>(storedDensity)
  const lastCheckedRef = useRef<string | null>(null)
  const excludedCount = items.length - includedCount
  const failed = storageFailed(storageNotice)
  const setSelectedExcluded = (excluded: boolean) => setItems((current) => current.map((item) => selectedForDirectory.includes(item.id ?? '') ? { ...item, excluded } : item))

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
  const visible = rows.filter((row) => filter === 'all' || (filter === 'attention' ? row.status.attention : filter === 'noCode' ? row.status.noCode : row.status.withErrors))
  const visibleIds = visible.map((row) => row.id)
  const allVisibleSelected = visibleIds.length > 0 && visibleIds.every((id) => selectedForDirectory.includes(id))
  const someVisibleSelected = visibleIds.some((id) => selectedForDirectory.includes(id))
  // Откуда ошибки: актуальный расчёт или прошлый (строки с тех пор менялись).
  const pastCheck = !result && reviewCheck && Object.keys(reviewCheck.rows).length > 0 ? reviewCheck : null

  // Стабильные обработчики для memo-строк: ввод в одной строке не перерисовывает остальные.
  const selectedIds = new Set(selectedForDirectory)
  const onSelect = useStableCallback((id: string, checked: boolean, range: boolean) => {
    const last = lastCheckedRef.current
    lastCheckedRef.current = id
    // Shift+щелчок — диапазон показанных строк от прошлой отмеченной до этой.
    const from = last ? visibleIds.indexOf(last) : -1
    const to = visibleIds.indexOf(id)
    if (range && from >= 0 && to >= 0 && from !== to) {
      const span = visibleIds.slice(Math.min(from, to), Math.max(from, to) + 1)
      setSelectedForDirectory((current) => checked ? [...new Set([...current, ...span])] : current.filter((selected) => !span.includes(selected)))
      return
    }
    setSelectedForDirectory((current) => checked ? [...current, id] : current.filter((selected) => selected !== id))
  })
  const onOpenPicker = useStableCallback((id: string) => openDirectoryPicker([id]))
  const onClearDirectory = useStableCallback(clearRowDirectory)
  const onChange = useStableCallback(setItem)
  const onRemove = useStableCallback(removeRow)
  const onPhotos = useStableCallback(openRowPhotos)
  const onUpload = useStableCallback(addManualPhoto)
  const onResetNet = useStableCallback(resetAutomaticNet)
  const onAgent = useStableCallback(openAgent)
  const agentPending = items.filter((item) => item.id && !item.excluded && !item.code && (!agentStatuses[item.id] || agentStatuses[item.id].status === 'ready')).length
  const agentStarted = counts.noCode - agentPending
  const ownPackageCount = items.filter((item) => item.packageWeightKg !== params.packageWeightKg).length
  const totals = reviewTotals(items, params.packageWeightKg)
  const total = (value: number) => formatRuNumber(formatWeight(value).replace(/\s/g, ''))

  function chooseDensity(value: Density) {
    setDensity(value)
    try { localStorage.setItem(DENSITY_KEY, value) } catch { /* без памяти — выбор на эту сессию */ }
  }

  // Следующая строка, требующая внимания, после строки с курсором (по кругу): прокрутить и
  // поставить курсор в название.
  function nextProblem() {
    const problems = visible.filter((row) => row.status.attention).map((row) => row.id)
    if (problems.length === 0) return
    const currentId = (document.activeElement?.closest('tr[data-row-id]') as HTMLElement | null)?.dataset.rowId ?? ''
    const at = visibleIds.indexOf(currentId)
    const next = problems.find((id) => visibleIds.indexOf(id) > at) ?? problems[0]
    const row = document.querySelector<HTMLElement>(`tr[data-row-id="${CSS.escape(next)}"]`)
    row?.scrollIntoView({ block: 'center' })
    row?.querySelector<HTMLInputElement>('input.rv-name')?.focus({ preventScroll: true })
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
      <summary>Параметры расчёта <span className="goods-params-summary">курс {params.usdKzt ? formatRuNumber(params.usdKzt) : '—'} · доставка: {deliverySummary} · сбор: {feeSummary}{params.customsValueMode === 'sir' && ` · ТС по СИР${Number(params.sirDiscountPercent) > 0 ? ` −${params.sirDiscountPercent}%` : ''}`}</span></summary>
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
          <div className="rv-field" title="Сбор = МРП × множитель, делится поровну между строками"><span>Сбор: МРП × множитель</span><span className="rv-joined">
            <input className="rv-w-md" inputMode="decimal" value={params.mrp} onChange={(event) => setParam('mrp', event.target.value.replace(',', '.'))} placeholder="МРП" aria-label="МРП, KZT" />
            <span className="rv-joined-sep">×</span>
            <input className="rv-w-sm" inputMode="decimal" value={params.feeMultiplier} onChange={(event) => setParam('feeMultiplier', event.target.value.replace(',', '.'))} placeholder="6" aria-label="Множитель сбора" />
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
          <button type="button" className="rv-bar-button" onClick={() => setSelectedExcluded(true)}>Исключить из расчёта</button>
          <button type="button" className="rv-bar-button" onClick={() => setSelectedExcluded(false)}>Включить</button>
          <button type="button" className="button-link" onClick={() => setSelectedForDirectory([])}>Снять выбор</button>
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

      <div className="goods-table-top-scroll" ref={topScrollRef} onScroll={() => syncHorizontalScroll('top')} aria-hidden="true"><div /></div>
      <div className="rv-grid-wrap" ref={tableWrapRef} onScroll={() => syncHorizontalScroll('body')}>
        <table className="rv-grid">
          <caption className="rv-sr">Строки сметы: Enter, ↑ ↓ — переход по строкам, F8 — следующая проблема</caption>
          <thead><tr>
            <th className="rv-check"><input type="checkbox" checked={allVisibleSelected} ref={(input) => { if (input) input.indeterminate = someVisibleSelected && !allVisibleSelected }} onChange={() => setSelectedForDirectory((current) => allVisibleSelected ? current.filter((id) => !visibleIds.includes(id)) : [...new Set([...current, ...visibleIds])])} aria-label="Выбрать все показанные строки" title={allDirectorySelected ? 'Выбраны все строки' : 'Выбрать все показанные строки'} /></th>
            <th className="rv-num" scope="col">№</th>
            <th className="rv-product" scope="col"><span className="rv-product-head">Товар<span className="rv-th-hint">код ТН ВЭД · подбор</span></span></th>
            <th className="rv-qty" scope="col" title="Количество мест">Места</th>
            <th className="rv-qty" scope="col" title="Количество товара в одном месте">Шт. в месте</th>
            <th className="rv-qty" scope="col" title="Количество товара. Пустое из трёх полей рассчитается: места × штук в месте">Кол-во, шт</th>
            <th className="rv-qty" scope="col">Брутто, кг</th>
            <th className="rv-net" scope="col" title="Пустое — рассчитается: брутто − места × вес упаковки">Нетто, кг</th>
            <th className="rv-include" scope="col" title="Учитывать строку в расчёте">В расчёте</th>
            <th className="rv-remove-col" scope="col"><span className="rv-sr">Удалить</span></th>
          </tr></thead>
          <tbody>
            {visible.map(({ item, index, id }) => <ReviewRow key={id || index} item={item} rowNumber={index + 1} resultItem={result?.items[index]} pastCheck={result ? undefined : reviewCheck?.rows[id]} selected={selectedIds.has(id)} onSelect={onSelect} onOpenPicker={onOpenPicker} onClearDirectory={onClearDirectory} onChange={onChange} onRemove={onRemove} onPhotos={onPhotos} onUpload={onUpload} onResetNet={onResetNet} defaultPackageWeight={params.packageWeightKg} agentStatus={agentStatuses[id]} onAgent={onAgent} />)}
            {items.length === 0 && <tr><td colSpan={10} className="goods-empty"><h3>Строк пока нет</h3><p>Загрузите XLSX или добавьте строку вручную.</p></td></tr>}
            {items.length > 0 && visible.length === 0 && <tr className="rv-empty-filter"><td colSpan={10}>В этом фильтре строк нет. <button type="button" className="button-link" onClick={() => setFilter('all')}>Показать все строки</button></td></tr>}
          </tbody>
          {items.length > 0 && <tfoot><tr>
            <td className="rv-check" />
            <td className="rv-num" />
            <td className="rv-product">Итого <small>по строкам в расчёте{filter !== 'all' ? ', не только показанным' : ''}</small></td>
            <td className="rv-qty">{total(totals.places)}</td>
            <td />
            <td className="rv-qty">{total(totals.quantity)}</td>
            <td className="rv-qty">{total(totals.grossKg)}</td>
            <td className="rv-net">{total(totals.netKg)}</td>
            <td colSpan={2} />
          </tr></tfoot>}
        </table>
      </div>
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
