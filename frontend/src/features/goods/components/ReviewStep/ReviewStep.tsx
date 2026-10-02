import { Dispatch, RefObject, SetStateAction } from 'react'
import { GoodsCalculationResult, GoodsItemInput, GoodsParams } from '@/shared/types/goods'
import { ReviewRow } from '../ReviewRow/ReviewRow'
import { reviewStatus } from '../ReviewRow/reviewStatus'
import { formatWeight } from '../../lib/netWeight'
import { reviewTotals } from './reviewTotals'
import { useStableCallback } from '@/shared/hooks/useStableCallback'
import { AgentRowStatus } from '../../agent/agentApi'
import '../../goods.css'
import './ReviewStep.css'

type Props = {
  params: GoodsParams
  items: GoodsItemInput[]
  result: GoodsCalculationResult | null
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
  setLightbox: (values: string[], index?: number) => void
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

export function ReviewStep({ params, items, result, selectedForDirectory, paramsOpen, loading, usdRateLoading, usdRateNotice, storageNotice, deliverySummary, feeSummary, tableWrapRef, topScrollRef, includedCount, allDirectorySelected, setItems, setSelectedForDirectory, setParamsOpen, setParam, loadUsdRate, syncHorizontalScroll, openDirectoryPicker, clearRowDirectory, setItem, removeRow, setLightbox, addManualPhoto, resetAutomaticNet, addEmptyRow, calculate, saveDraft, clearAll, agentStatuses, agentBatch, openAgent, startAgentBatch }: Props) {
  const excludedCount = items.length - includedCount
  const storageFailed = /не сохранён|недоступен|не удалось|отклонил|ошиб/i.test(storageNotice)
  const setSelectedExcluded = (excluded: boolean) => setItems((current) => current.map((item) => selectedForDirectory.includes(item.id ?? '') ? { ...item, excluded } : item))
  // Стабильные обработчики для memo-строк: ввод в одной строке не перерисовывает остальные.
  const selectedIds = new Set(selectedForDirectory)
  const onSelect = useStableCallback((id: string, checked: boolean) => setSelectedForDirectory((current) => checked ? [...current, id] : current.filter((selected) => selected !== id)))
  const onOpenPicker = useStableCallback((id: string) => openDirectoryPicker([id]))
  const onClearDirectory = useStableCallback(clearRowDirectory)
  const onChange = useStableCallback(setItem)
  const onRemove = useStableCallback(removeRow)
  const onPreview = useStableCallback(setLightbox)
  const onUpload = useStableCallback(addManualPhoto)
  const onResetNet = useStableCallback(resetAutomaticNet)
  const onAgent = useStableCallback(openAgent)
  const agentPending = items.filter((item) => item.id && !item.excluded && !item.code && !agentStatuses[item.id]).length
  const ownPackageCount = items.filter((item) => item.packageWeightKg !== params.packageWeightKg).length
  const withoutCodeCount = items.filter((item) => !item.excluded && !item.code).length
  const errorCount = items.filter((item, index) => reviewStatus(item, result?.items[index]).tone === 'bad').length
  const totals = reviewTotals(items, params.packageWeightKg)
  const total = (value: number) => formatWeight(Number(value.toFixed(3)))
  return <div className="goods-step-page rv-step">
    <details className="goods-params goods-params-top" open={paramsOpen} onToggle={(event) => setParamsOpen(event.currentTarget.open)}>
      <summary>Параметры расчёта <span className="goods-params-summary">курс {params.usdKzt || '—'} · доставка: {deliverySummary} · сбор: {feeSummary}</span></summary>
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
    <section className="rv-card">
      <div className="rv-bar">
        {selectedForDirectory.length > 0 ? <>
          <strong>Выбрано: {selectedForDirectory.length}</strong>
          <button type="button" className="secondary" onClick={() => openDirectoryPicker(selectedForDirectory)}>Товар или код ТН ВЭД</button>
          <button type="button" className="rv-bar-button" onClick={() => setSelectedExcluded(true)}>Исключить из расчёта</button>
          <button type="button" className="rv-bar-button" onClick={() => setSelectedExcluded(false)}>Включить</button>
          <button type="button" className="button-link" onClick={() => setSelectedForDirectory([])}>Снять выбор</button>
        </> : <>
          <span>Строк: <b>{items.length}</b> · в расчёте: <b>{includedCount}</b>{excludedCount > 0 && <> · исключено: <b>{excludedCount}</b></>}</span>
          {withoutCodeCount > 0 && <span className="rv-bar-count warn" title="Выберите код ТН ВЭД — без него строка не рассчитается">без кода: {withoutCodeCount}</span>}
          {errorCount > 0 && <span className="rv-bar-count bad" title="Причина — в подсказке у значка «!» в конце строки">с ошибками: {errorCount}</span>}
          {agentBatch ? <span className="rv-bar-count">ИИ запускает подбор: {agentBatch.done} из {agentBatch.total}</span>
            : agentPending > 0 && <button type="button" className="rv-bar-button rv-agent-batch" onClick={startAgentBatch} disabled={loading} title="ИИ-агент подберёт коды для включённых строк без кода; результат — в чате строки (кнопка «ИИ»)">Подобрать коды ИИ ({agentPending})</button>}
          <span className="rv-bar-end">
            <small className="rv-bar-hint">Enter, ↑ ↓ — переход по строкам</small>
            <button type="button" className="rv-bar-button" onClick={addEmptyRow} disabled={loading}>+ Строка</button>
          </span>
        </>}
      </div>
      <div className="goods-table-top-scroll" ref={topScrollRef} onScroll={() => syncHorizontalScroll('top')} aria-hidden="true"><div /></div>
      <div className="rv-grid-wrap" ref={tableWrapRef} onScroll={() => syncHorizontalScroll('body')}>
        <table className="rv-grid">
          <thead><tr>
            <th className="rv-check"><input type="checkbox" checked={allDirectorySelected} onChange={(event) => setSelectedForDirectory(event.target.checked ? items.map((item) => item.id ?? '') : [])} aria-label="Выбрать все строки" title="Выбрать все строки" /></th>
            <th className="rv-num">№</th>
            <th className="rv-product">Товар <small>· код ТН ВЭД</small></th>
            <th className="rv-qty" title="Количество мест">Места</th>
            <th className="rv-qty" title="Количество товара в одном месте">Шт. в месте</th>
            <th className="rv-qty" title="Количество товара. Пустое из трёх полей рассчитается: места × штук в месте">Кол-во, шт</th>
            <th className="rv-qty">Брутто, кг</th>
            <th className="rv-net" title="Пустое — рассчитается: брутто − места × вес упаковки">Нетто, кг</th>
            <th className="rv-include" title="Учитывать строку в расчёте">В расчёте</th>
            <th className="rv-status" aria-label="Статус" />
            <th className="rv-remove-col" aria-label="Удалить" />
          </tr></thead>
          <tbody>
            {items.map((item, index) => <ReviewRow key={item.id ?? index} item={item} rowNumber={index + 1} resultItem={result?.items[index]} selected={selectedIds.has(item.id ?? '')} onSelect={onSelect} onOpenPicker={onOpenPicker} onClearDirectory={onClearDirectory} onChange={onChange} onRemove={onRemove} onPreview={onPreview} onUpload={onUpload} onResetNet={onResetNet} defaultPackageWeight={params.packageWeightKg} agentStatus={agentStatuses[item.id ?? '']} onAgent={onAgent} />)}
            {items.length === 0 && <tr><td colSpan={11} className="goods-empty"><h3>Строк пока нет</h3><p>Загрузите XLSX или добавьте строку вручную.</p></td></tr>}
          </tbody>
          {items.length > 0 && <tfoot><tr>
            <td className="rv-check" />
            <td className="rv-num" />
            <td className="rv-product">Итого <small>по строкам в расчёте</small></td>
            <td className="rv-qty">{total(totals.places)}</td>
            <td />
            <td className="rv-qty">{total(totals.quantity)}</td>
            <td className="rv-qty">{total(totals.grossKg)}</td>
            <td className="rv-net">{total(totals.netKg)}</td>
            <td colSpan={3} />
          </tr></tfoot>}
        </table>
      </div>
      <div className="rv-actions">
        <span className={storageFailed ? 'goods-storage-error' : 'rv-actions-note'} role={storageFailed ? 'alert' : undefined}>{storageNotice || 'Изменения сохраняются в браузере автоматически'}</span>
        <button type="button" className="rv-clear" onClick={clearAll} disabled={loading || items.length === 0}>Очистить</button>
        <button type="button" className="btn-save-draft" onClick={() => void saveDraft()} disabled={loading || items.length === 0} title="Сохранить черновик на сервере">Сохранить черновик</button>
        <button type="button" className="btn-calculate" onClick={() => void calculate()} disabled={loading || includedCount === 0}>{loading ? 'Считаем…' : 'Рассчитать →'}</button>
      </div>
    </section>
  </div>
}
