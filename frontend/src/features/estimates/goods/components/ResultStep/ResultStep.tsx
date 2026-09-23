import { useState } from 'react'
import { GlovesCalculationResult, GlovesItemInput } from '../../../shared/types'
import { CalculationErrors } from './CalculationErrors'
import { ResultSummary } from './ResultSummary'
import { display, displayNumber, hasAntiDumping, itemDisplayName, rowTotals } from './resultValues'
import '../../../shared/styles/calculatorCommon.css'
import '../../styles/calculator.css'
import './ResultStep.css'

type Props = {
  result: GlovesCalculationResult
  items: GlovesItemInput[]
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
}

// Этап 3: ошибки расчёта, итоги и таблица результата с временной правкой цены единицы.
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
}: Props) {
  const [bulkPriceDecrease, setBulkPriceDecrease] = useState('')
  const [selectedRows, setSelectedRows] = useState<number[]>([])
  const validBulkPriceDecrease = /^\d+(\.\d+)?$/.test(bulkPriceDecrease) && Number(bulkPriceDecrease) > 0
  const allRowsSelected = result.items.length > 0 && selectedRows.length === result.items.length
  const checkTotals = rowTotals(result)
  const antiDumping = hasAntiDumping(result)

  function toggleSelectedRow(index: number) {
    setSelectedRows((current) => current.includes(index)
      ? current.filter((selected) => selected !== index)
      : [...current, index])
  }

  const applyTo = selectedRows.length > 0 ? selectedRows : undefined

  return (
    <div className="gloves-step-page rs-step">
      <CalculationErrors result={result} onBackToReview={onBackToReview} />

      <ResultSummary result={result} />

      <section className="rv-card">
        <div className="rv-bar">
          {selectedRows.length > 0
            ? <><strong>Выбрано: {selectedRows.length}</strong><button type="button" className="button-link" onClick={() => setSelectedRows([])} disabled={loading}>Снять выбор</button></>
            : <span>Строк: <b>{result.items.length}</b> · цену единицы можно менять прямо в таблице</span>}
          <div className="rs-price-tool">
            <label>Цена ед. −<input inputMode="decimal" value={bulkPriceDecrease} onChange={(event) => setBulkPriceDecrease(event.target.value.replace(',', '.'))} placeholder="0.01" aria-label="Сколько вычесть из цены единицы" /></label>
            <button type="button" className="secondary" onClick={() => { onBulkPriceDecrease(bulkPriceDecrease, applyTo); setBulkPriceDecrease('') }} disabled={!validBulkPriceDecrease || loading}>{applyTo ? `Вычесть у выбранных (${selectedRows.length})` : 'Вычесть у всех'}</button>
            <button type="button" className="button-link" onClick={onUndoBulkPriceDecrease} disabled={!canUndoBulkPriceDecrease || loading} title="Отменить последний вычет">↶ Отменить</button>
            <button type="button" className="button-link" onClick={onRestoreInitial} disabled={loading} title="Вернуть исходные цены всех строк">Сбросить цены</button>
          </div>
        </div>

        <div className="rv-grid-wrap rs-table-wrap">
          <table className="rs-table">
            <thead>
              <tr>
                <th className="rs-check"><input type="checkbox" checked={allRowsSelected} onChange={(event) => setSelectedRows(event.target.checked ? result.items.map((_, index) => index) : [])} disabled={loading} aria-label="Выбрать все строки" /></th>
                <th className="rs-num">№</th>
                <th className="rs-name">Товар</th>
                <th className="rs-text">Код ТН ВЭД</th>
                <th>Места</th>
                <th title="Количество товара в одном месте">Шт. в месте</th>
                <th>Кол-во, шт</th>
                <th>Брутто, кг</th>
                <th>Нетто, кг</th>
                <th>Цена ед.</th>
                <th>Стоимость</th>
                <th title="Таможенная стоимость">ТС, ₸</th>
                <th>Пошлина, ₸</th>
                {antiDumping && <th>Антидемпинг, ₸</th>}
                <th>НДС, ₸</th>
                <th>Сбор, ₸</th>
                <th>Доставка, USD</th>
                <th className="rs-text">Статус</th>
              </tr>
            </thead>
            <tbody>
              {result.items.map((item, index) => {
                const source = items[index]

                // The value typed by the user must have priority over the
                // calculated pair price. Otherwise the input immediately
                // jumps back to the old value after every keystroke.
                const price = item.pricePerPair || item.pairPrice || source?.pricePerPair || ''
                const sourceRow = index + 1
                const issues = [...(item.errors ?? []), ...(item.warnings ?? [])]
                const name = itemDisplayName(item, source)

                return (
                  <tr key={`${item.productId}-${index}`} className={[selectedRows.includes(index) ? 'rv-selected' : '', item.excluded ? 'rv-excluded' : ''].filter(Boolean).join(' ') || undefined}>
                    <td className="rs-check"><input type="checkbox" checked={selectedRows.includes(index)} onChange={() => toggleSelectedRow(index)} disabled={loading} aria-label={`Выбрать строку ${sourceRow}`} /></td>
                    <td className="rs-num">{sourceRow}</td>
                    <td className="rs-name" title={name}>{name}</td>
                    <td className="rs-text"><code>{display(item.code || source?.code)}</code></td>
                    <td>{displayNumber(item.places)}</td>
                    <td>{displayNumber(item.pairsPerPlace)}</td>
                    <td>{displayNumber(item.pairs)}</td>
                    <td>{displayNumber(item.grossKg)}</td>
                    <td>{displayNumber(item.netKg)}</td>
                    <td>
                      <input
                        className="rs-price"
                        inputMode="decimal"
                        value={price}
                        onChange={(event) => onPriceChange(index, event.target.value.replace(',', '.'))}
                        onBlur={(event) => onPriceBlur(index, event.currentTarget.value)}
                        onKeyDown={(event) => {
                          if (event.key === 'Enter') event.currentTarget.blur()
                        }}
                        aria-label={`Цена за единицу, строка ${sourceRow}`}
                      />
                    </td>
                    <td>{displayNumber(item.goodsCost || item.costInput)} <small>{item.goodsCostCurrency || item.costCurrency || ''}</small></td>
                    <td>{displayNumber(item.customsValueKzt)}</td>
                    <td>{displayNumber(item.dutyKzt)}</td>
                    {antiDumping && <td>{displayNumber(item.antiDumpingKzt)}</td>}
                    <td>{displayNumber(item.vatKzt)}</td>
                    <td>{displayNumber(item.feeKzt)}</td>
                    <td>{displayNumber(item.deliveryUsd)}</td>
                    <td className="rs-text">
                      <span className={`rv-badge ${item.status === 'calculated' ? 'ok' : item.errors?.length ? 'bad' : 'warn'}`} title={issues.join('\n') || undefined}>
                        {item.status === 'calculated' ? 'Готово' : 'Проверить'}
                      </span>
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
                <td className="rs-text" />
                <td>{displayNumber(checkTotals.places)}</td>
                <td />
                <td>{displayNumber(checkTotals.quantity)}</td>
                <td>{displayNumber(Number(checkTotals.grossKg.toFixed(3)))}</td>
                <td>{displayNumber(Number(checkTotals.netKg.toFixed(3)))}</td>
                <td />
                <td>{displayNumber(result.totals.totalCostUsd)} <small>USD</small></td>
                <td>{displayNumber(result.totals.customsValueKzt)}</td>
                <td>{displayNumber(result.totals.dutyKzt)}</td>
                {antiDumping && <td>{displayNumber(result.totals.antiDumpingKzt)}</td>}
                <td>{displayNumber(result.totals.vatKzt)}</td>
                <td>{displayNumber(result.totals.feeKzt)}</td>
                <td>{displayNumber(result.totals.totalDeliveryUsd)}</td>
                <td className="rs-text" />
              </tr>
            </tfoot>
          </table>
        </div>

        <div className="rv-actions">
          <span className="rv-actions-note">Стоимость товара и доставка — отдельные расходы, в растаможку не входят</span>
          <button type="button" className="btn-save-draft" onClick={saveDraft} disabled={loading}>Сохранить результат</button>
          <button type="button" className="btn-calculate" onClick={exportExcel} disabled={loading}>{loading ? 'Подождите…' : 'Экспорт Excel'}</button>
        </div>
      </section>
    </div>
  )
}
