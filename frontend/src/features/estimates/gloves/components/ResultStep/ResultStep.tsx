import { useState } from 'react'
import { GlovesCalculationResult, GlovesItemInput } from '../../../shared/types'
import { formatDisplayNumber } from '@/shared/lib/formatDisplayNumber'
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
  const display = (value: string | number | undefined | null) =>
    value === undefined || value === null || value === '' ? '—' : String(value)
  const displayNumber = (value: string | number | undefined | null) => {
    const shown = display(value)
    return shown === '—' ? shown : formatDisplayNumber(shown)
  }

  const customsPaymentsUsd = (() => {
    const kzt = Number(String(result.totals.customsPaymentsKzt || '').replace(',', '.'))
    const rate = Number(String(result.usdKzt || '').replace(',', '.'))
    return Number.isFinite(kzt) && Number.isFinite(rate) && rate > 0
      ? `${formatDisplayNumber((kzt / rate).toFixed(2))} USD`
      : '—'
  })()
  const validBulkPriceDecrease = /^\d+(\.\d+)?$/.test(bulkPriceDecrease) && Number(bulkPriceDecrease) > 0
  const allRowsSelected = result.items.length > 0 && selectedRows.length === result.items.length

  function toggleSelectedRow(index: number) {
    setSelectedRows((current) => current.includes(index)
      ? current.filter((selected) => selected !== index)
      : [...current, index])
  }

  return (
    <div className="gloves-step-page">
      <div className="gloves-step-title">
        <div>
          <p className="eyebrow">ЭТАП 3</p>
          <h2>Результат расчёта</h2>
          <p className="muted">
            Растаможка = таможенный сбор + пошлины + НДС. Стоимость товара и доставка — отдельные расходы, которые в растаможку не входят.
          </p>
        </div>
        <button type="button" className="button-link" onClick={onBackToReview}>
          ← Вернуться к проверке
        </button>
      </div>

      <section className="gloves-summary">
        <div><dt>Растаможка (сбор + пошлина + НДС)</dt><dd>{displayNumber(result.totals.customsPaymentsKzt)} KZT / {customsPaymentsUsd}</dd></div>
        <div><dt>Пошлина</dt><dd>{displayNumber(result.totals.dutyKzt)} KZT</dd></div>
        <div><dt>Антидемпинг</dt><dd>{displayNumber(result.totals.antiDumpingKzt)} KZT</dd></div>
        <div><dt>НДС</dt><dd>{displayNumber(result.totals.vatKzt)} KZT</dd></div>
        <div><dt>Таможенный сбор</dt><dd>{displayNumber(result.totals.feeKzt)} KZT</dd></div>
        <div><dt>Таможенная стоимость</dt><dd>{displayNumber(result.totals.customsValueKzt)} KZT</dd></div>
        <div><dt>Доставка</dt><dd>{displayNumber(result.totals.totalDeliveryUsd)} USD / {displayNumber(result.totals.deliveryKzt)} KZT</dd></div>
        <div><dt>Рассчитано</dt><dd>{displayNumber(result.totals.calculatedCount)} из {displayNumber(result.totals.totalCount)}</dd></div>
      </section>

      <section className="gloves-result-table-shell">
        <div className="gloves-result-table-heading">
          <div>
            <h3>Данные по строкам</h3>
            <p>Все значения, вычитанные из файла, и рассчитанные суммы по каждому товару.</p>
          </div>
          <div className="gloves-bulk-price-tool">
            <label>Вычесть из цены пары
              <input
                inputMode="decimal"
                value={bulkPriceDecrease}
                onChange={(event) => setBulkPriceDecrease(event.target.value.replace(',', '.'))}
                placeholder="Например, 0.01"
                aria-label="Сколько вычесть из цены пары во всех строках"
              />
            </label>
            <button
              type="button"
              onClick={() => {
                onBulkPriceDecrease(bulkPriceDecrease, selectedRows)
                setBulkPriceDecrease('')
              }}
              disabled={!validBulkPriceDecrease || selectedRows.length === 0 || loading}
            >ОК выбранным ({selectedRows.length})</button>
            <button
              type="button"
              className="gloves-result-tool-secondary"
              onClick={() => {
                onBulkPriceDecrease(bulkPriceDecrease)
                setBulkPriceDecrease('')
              }}
              disabled={!validBulkPriceDecrease || loading}
            >ОК всем</button>
            <button
              type="button"
              className="gloves-result-tool-secondary"
              onClick={() => setSelectedRows(result.items.map((_, index) => index))}
              disabled={allRowsSelected || loading}
            >Выбрать все</button>
            <button
              type="button"
              className="gloves-result-tool-secondary"
              onClick={() => setSelectedRows([])}
              disabled={selectedRows.length === 0 || loading}
            >Снять выбор</button>
            <button
              type="button"
              className="gloves-result-tool-secondary"
              onClick={onRestoreInitial}
              disabled={loading}
            >Вернуть все значения</button>
            <button
              type="button"
              className="gloves-result-tool-secondary"
              onClick={onUndoBulkPriceDecrease}
              disabled={!canUndoBulkPriceDecrease || loading}
            >Вернуть до последнего вычета</button>
            <span>{result.items.length} строк</span>
          </div>
        </div>

        <div className="gloves-result-table-scroll">
          <table className="gloves-result-table">
            <thead>
              <tr>
                <th className="result-select-column">
                  <input
                    type="checkbox"
                    checked={allRowsSelected}
                    onChange={(event) => setSelectedRows(event.target.checked ? result.items.map((_, index) => index) : [])}
                    disabled={loading}
                    aria-label="Выбрать все строки"
                  />
                </th>
                <th>Строка</th>
                <th>Товар</th>
                <th>Код ТН ВЭД</th>
                <th>Места</th>
                <th>Пар в месте</th>
                <th>Пары</th>
                <th>Брутто, кг</th>
                <th>Нетто, кг</th>
                <th>ИТС</th>
                <th>Цена пары</th>
                <th>Стоимость</th>
                <th>ТС, KZT</th>
                <th>Пошлина</th>
                <th>Антидемпинг</th>
                <th>НДС</th>
                <th>Сбор</th>
                <th>Доставка, USD</th>
                <th>Статус</th>
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

                return (
                  <tr key={`${item.productId}-${index}`} className={selectedRows.includes(index) ? 'result-row-selected' : undefined}>
                    <td className="result-select-column">
                      <input
                        type="checkbox"
                        checked={selectedRows.includes(index)}
                        onChange={() => toggleSelectedRow(index)}
                        disabled={loading}
                        aria-label={`Выбрать строку ${sourceRow}`}
                      />
                    </td>
                    <td>{displayNumber(sourceRow)}</td>
                    <td className="result-product-name">{display(item.standardName || source?.name)}</td>
                    <td><code>{display(item.code || source?.code)}</code></td>
                    <td>{displayNumber(item.places)}</td>
                    <td>{displayNumber(item.pairsPerPlace)}</td>
                    <td>{displayNumber(item.pairs)}</td>
                    <td>{displayNumber(item.grossKg)}</td>
                    <td>{displayNumber(item.netKg)}</td>
                    <td>{displayNumber(item.coefficient)}</td>
                    <td>
                      <input
                        className="gloves-result-price-input"
                        inputMode="decimal"
                        value={price}
                        onChange={(event) => onPriceChange(index, event.target.value.replace(',', '.'))}
                        onBlur={(event) => onPriceBlur(index, event.currentTarget.value)}
                        onKeyDown={(event) => {
                          if (event.key === 'Enter') event.currentTarget.blur()
                        }}
                        aria-label={`Цена за пару, строка ${sourceRow}`}
                      />
                    </td>
                    <td>{displayNumber(item.goodsCost || item.costInput)} {item.goodsCostCurrency || item.costCurrency || ''}</td>
                    <td>{displayNumber(item.customsValueKzt)}</td>
                    <td>{displayNumber(item.dutyKzt)}</td>
                    <td>{displayNumber(item.antiDumpingKzt)}</td>
                    <td>{displayNumber(item.vatKzt)}</td>
                    <td>{displayNumber(item.feeKzt)}</td>
                    <td>{displayNumber(item.deliveryUsd)}</td>
                    <td>
                      <span className={`result-status ${item.status === 'calculated' ? 'result-status-ok' : 'result-status-review'}`}>
                        {item.status === 'calculated' ? 'Готово' : 'Проверить'}
                      </span>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </section>

      {result.totals.blockingErrors?.length ? (
        <div className="banner error">
          <strong>Нужно исправить:</strong>
          <ul>{result.totals.blockingErrors.map((value, index) => <li key={`${value}-${index}`}>{value}</li>)}</ul>
        </div>
      ) : null}

      <div className="gloves-actions">
        <button type="button" className="btn-save-draft" onClick={saveDraft} disabled={loading}>Сохранить результат</button>
        <button type="button" className="btn-export" onClick={exportExcel} disabled={loading}>{loading ? 'Подождите…' : 'Экспорт Excel'}</button>
      </div>
    </div>
  )
}
