import { useState } from 'react'
import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'
import { weightText } from '@/shared/lib/rounding'
import { BudgetControls, BudgetNote, BudgetStatus } from '../BudgetTool/BudgetTool'
import { BudgetDistributionState } from '../../hooks/useBudgetDistribution'
import { CalculationErrors } from './CalculationErrors'
import { ResultSummary } from './ResultSummary'
import { display, displayNumber, hasAntiDumping, itemDisplayName, parseCalculatorNumber, rowTotals } from './resultValues'
import '../../goods.css'
import './ResultStep.css'

type Props = {
  result: GoodsCalculationResult
  items: GoodsItemInput[]
  loading: boolean
  saveDraft: () => void
  exportExcel: () => void
  onBackToReview: () => void
  onPriceChange: (index: number, value: string) => void
  onPriceBlur: (index: number, value: string) => void
  onBulkPriceDecrease: (value: string, indexes?: number[]) => void
  onRestoreInitial: () => void
  onUndoBulkPriceDecrease: () => void
  canUndoBulkPriceDecrease: boolean
  budget: BudgetDistributionState
}

const positive = (value: unknown) => (parseCalculatorNumber(value as string) ?? 0) > 0

// Этап 3: итоги сверху, одна панель инструментов цен и компактная таблица, которая
// помещается по ширине: код — под названием, места и брутто — подписями под количеством и нетто.
export function ResultStep({
  result,
  items,
  loading,
  saveDraft,
  exportExcel,
  onBackToReview,
  onPriceChange,
  onPriceBlur,
  onBulkPriceDecrease,
  onRestoreInitial,
  onUndoBulkPriceDecrease,
  canUndoBulkPriceDecrease,
  budget,
}: Props) {
  const [bulkPriceDecrease, setBulkPriceDecrease] = useState('')
  const [selectedRows, setSelectedRows] = useState<number[]>([])
  // Цена, которую сейчас вводят: пока поле в фокусе, показываем ровно её (в том числе пустую),
  // иначе стёртое поле сразу заполнилось бы рассчитанной ценой. Пересчёт — при выходе из поля.
  const [draftPrices, setDraftPrices] = useState<Record<number, string>>({})
  const validBulkPriceDecrease = /^\d+(\.\d+)?$/.test(bulkPriceDecrease) && Number(bulkPriceDecrease) > 0
  const allRowsSelected = result.items.length > 0 && selectedRows.length === result.items.length
  const checkTotals = rowTotals(result)
  const antiDumping = hasAntiDumping(result)
  // Колонка доставки нужна, только если доставка есть.
  const delivery = positive(result.totals.totalDeliveryUsd) || result.items.some((item) => positive(item.deliveryUsd))
  const applyTo = selectedRows.length > 0 ? selectedRows : undefined

  function toggleSelectedRow(index: number) {
    setSelectedRows((current) => current.includes(index)
      ? current.filter((selected) => selected !== index)
      : [...current, index])
  }

  return (
    <div className="goods-step-page rs-step">
      <CalculationErrors result={result} onBackToReview={onBackToReview} />

      <ResultSummary result={result} loading={loading} saveDraft={saveDraft} exportExcel={exportExcel} />

      <section className="rv-card">
        <div className="rs-toolbar">
          <div className="rs-tool-group" aria-label="Подбор цен под бюджет">
            <BudgetControls budget={budget} disabled={loading} />
          </div>
          <div className="rs-tool-group rs-price-tool" aria-label="Правка цен">
            {selectedRows.length > 0
              ? <span className="rs-selected">Выбрано: <b>{selectedRows.length}</b> <button type="button" className="button-link" onClick={() => setSelectedRows([])} disabled={loading}>снять</button></span>
              : null}
            <label className="rs-decrease" title="Вычесть сумму из цены единицы у всех строк или у выбранных галочкой">
              <span>Снизить цену на</span>
              <span className="rs-joined">
                <input inputMode="decimal" value={bulkPriceDecrease} onChange={(event) => setBulkPriceDecrease(event.target.value.replace(',', '.'))} placeholder="0.00" aria-label="Сколько вычесть из цены единицы, USD" />
                <i>USD</i>
                <button type="button" onClick={(event) => { event.preventDefault(); onBulkPriceDecrease(bulkPriceDecrease, applyTo); setBulkPriceDecrease('') }} disabled={!validBulkPriceDecrease || loading}>{applyTo ? 'у выбранных' : 'у всех'}</button>
              </span>
            </label>
            <span className="rs-tool-sep" aria-hidden="true" />
            <button type="button" className="rs-quiet" onClick={onUndoBulkPriceDecrease} disabled={!canUndoBulkPriceDecrease || loading} title="Вернуть цены до последнего вычета или подбора">↶ Отменить</button>
            <button type="button" className="rs-quiet" onClick={onRestoreInitial} disabled={loading} title="Вернуть исходные цены всех строк (из Excel, по ИТП)">Сбросить цены</button>
          </div>
        </div>
        <BudgetStatus budget={budget} />

        <div className="rv-grid-wrap rs-table-wrap">
          <table className="rs-table">
            <thead>
              <tr>
                <th className="rs-check"><input type="checkbox" checked={allRowsSelected} onChange={(event) => setSelectedRows(event.target.checked ? result.items.map((_, index) => index) : [])} disabled={loading} aria-label="Выбрать все строки" /></th>
                <th className="rs-num">№</th>
                <th className="rs-name">Товар <small>· код ТН ВЭД</small></th>
                <th title="Количество; под ним — места × штук в месте">Кол-во, шт</th>
                <th title="Вес нетто; под ним — брутто">Нетто, кг</th>
                <th title="Цену единицы можно менять прямо в таблице">Цена ед., USD</th>
                <th>Стоимость, USD</th>
                <th title="Таможенная стоимость">ТС, ₸</th>
                <th>Пошлина, ₸</th>
                {antiDumping && <th>Антидемпинг, ₸</th>}
                <th>НДС, ₸</th>
                <th>Сбор, ₸</th>
                {delivery && <th>Доставка, USD</th>}
                <th className="rs-status" aria-label="Статус" />
              </tr>
            </thead>
            <tbody>
              {result.items.map((item, index) => {
                const source = items[index]
                // The value typed by the user must have priority over the
                // calculated pair price. Otherwise the input immediately
                // jumps back to the old value after every keystroke.
                const price = draftPrices[index] ?? (item.pricePerPair || item.pairPrice || source?.pricePerPair || '')
                const sourceRow = index + 1
                const issues = [...(item.errors ?? []), ...(item.warnings ?? [])]
                const name = itemDisplayName(item, source)
                const ready = item.status === 'calculated'
                const currency = item.goodsCostCurrency || item.costCurrency || 'USD'
                // Антидемпинг из реестра ЕЭК (сервер кладёт его в снимок строки при расчёте).
                const antiDumpingInfo = item.directorySnapshot?.antiDumping as { status?: string; number?: string; percent?: string; text?: string } | undefined
                const antiDumpingNote = antiDumpingInfo && (antiDumpingInfo.status === 'possible' || antiDumpingInfo.status === 'origin') ? antiDumpingInfo : undefined

                return (
                  <tr key={`${item.productId}-${index}`} className={[selectedRows.includes(index) ? 'rv-selected' : '', item.excluded ? 'rv-excluded' : ''].filter(Boolean).join(' ') || undefined}>
                    <td className="rs-check"><input type="checkbox" checked={selectedRows.includes(index)} onChange={() => toggleSelectedRow(index)} disabled={loading} aria-label={`Выбрать строку ${sourceRow}`} /></td>
                    <td className="rs-num">{sourceRow}</td>
                    <td className="rs-name" title={name}>
                      <span className="rs-name-text">{name}</span>
                      <code>{display(item.code || source?.code)}</code>
                    </td>
                    <td>{displayNumber(item.pairs)}{(item.places || item.pairsPerPlace) && <small className="rs-sub">{displayNumber(item.places)} × {displayNumber(item.pairsPerPlace)}</small>}</td>
                    <td>{displayNumber(item.netKg)}{item.grossKg && <small className="rs-sub">брутто {displayNumber(item.grossKg)}</small>}</td>
                    <td>
                      <span className="price-field">
                      <input
                        className="rs-price"
                        inputMode="decimal"
                        value={price}
                        onChange={(event) => { const value = event.target.value.replace(',', '.'); setDraftPrices((current) => ({ ...current, [index]: value })); onPriceChange(index, value) }}
                        onBlur={() => {
                          const value = draftPrices[index]
                          if (value === undefined) return
                          setDraftPrices(({ [index]: _done, ...rest }) => rest)
                          // Пустое поле — убрать цену: строка считается без неё (по ИТП или с пометкой «Укажите цену»).
                          onPriceBlur(index, value.trim())
                        }}
                        onKeyDown={(event) => {
                          if (event.key === 'Enter') event.currentTarget.blur()
                        }}
                        aria-label={`Цена за единицу, строка ${sourceRow}`}
                      />
                      <BudgetNote note={budget.notes[index]} />
                      </span>
                    </td>
                    <td>{displayNumber(item.goodsCost || item.costInput)}{currency !== 'USD' && <small> {currency}</small>}</td>
                    <td>{displayNumber(item.customsValueKzt)}</td>
                    <td>{displayNumber(item.dutyKzt)}{antiDumpingNote && <small className="rs-sub rs-ad-note" title={antiDumpingNote.text}>{antiDumpingNote.status === 'origin' ? 'антидемпинг: укажите страну' : 'антидемпинг: проверить'}</small>}</td>
                    {antiDumping && <td title={antiDumpingInfo?.text}>{displayNumber(item.antiDumpingKzt)}{antiDumpingInfo?.status === 'applied' && <small className="rs-sub">{antiDumpingInfo.number} · {String(antiDumpingInfo.percent ?? '').replace('.', ',')}%</small>}</td>}
                    <td>{displayNumber(item.vatKzt)}</td>
                    <td>{displayNumber(item.feeKzt)}</td>
                    {delivery && <td>{displayNumber(item.deliveryUsd)}</td>}
                    <td className="rs-status">
                      <span className={`rs-dot ${item.excluded ? 'off' : ready ? 'ok' : item.errors?.length ? 'bad' : 'warn'}`} title={item.excluded ? 'Исключена' : ready ? 'Рассчитано' : issues.join('\n') || 'Проверить'} aria-label={ready ? 'Рассчитано' : 'Проверить'}>{ready ? '✓' : '!'}</span>
                    </td>
                  </tr>
                )
              })}
            </tbody>
            <tfoot>
              <tr>
                <td className="rs-check" />
                <td className="rs-num" />
                <td className="rs-name">Итого</td>
                <td>{displayNumber(checkTotals.quantity)}<small className="rs-sub">мест {displayNumber(checkTotals.places)}</small></td>
                <td>{displayNumber(weightText(checkTotals.netKg))}<small className="rs-sub">брутто {displayNumber(weightText(checkTotals.grossKg))}</small></td>
                <td />
                <td>{displayNumber(result.totals.totalCostUsd)}</td>
                <td>{displayNumber(result.totals.customsValueKzt)}</td>
                <td>{displayNumber(result.totals.dutyKzt)}</td>
                {antiDumping && <td>{displayNumber(result.totals.antiDumpingKzt)}</td>}
                <td>{displayNumber(result.totals.vatKzt)}</td>
                <td>{displayNumber(result.totals.feeKzt)}</td>
                {delivery && <td>{displayNumber(result.totals.totalDeliveryUsd)}</td>}
                <td className="rs-status" />
              </tr>
            </tfoot>
          </table>
        </div>

      </section>
    </div>
  )
}
