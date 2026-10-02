import { formatDisplayNumber } from '@/shared/lib/formatDisplayNumber'
import { GoodsCalculationResult } from '@/shared/types/goods'
import { ItpCalculator } from './ItpCalculator'
import { displayNumber, hasAntiDumping, parseCalculatorNumber } from './resultValues'
import './ResultSummary.css'

// Карточка итогов: растаможка как формула из слагаемых, справочные суммы, кнопки и сверка по ИТП.
type SummaryProps = { result: GoodsCalculationResult; loading: boolean; saveDraft: () => void; exportExcel: () => void }

// Кнопки сохранения и выгрузки — в карточке итогов, чтобы всегда были на виду.
export function ResultSummary({ result, loading, saveDraft, exportExcel }: SummaryProps) {
  const customsPaymentsUsd = (() => {
    const kzt = parseCalculatorNumber(result.totals.customsPaymentsKzt) ?? NaN
    const rate = parseCalculatorNumber(result.usdKzt) ?? NaN
    return Number.isFinite(kzt) && Number.isFinite(rate) && rate > 0
      ? `${formatDisplayNumber((kzt / rate).toFixed(2))} USD`
      : '—'
  })()
  // Слагаемые растаможки — как на бэкенде: пошлина + антидемпинг + НДС + сбор.
  const parts = [
    { label: 'Пошлина', value: result.totals.dutyKzt },
    ...(hasAntiDumping(result) ? [{ label: 'Антидемпинг', value: result.totals.antiDumpingKzt }] : []),
    { label: 'НДС', value: result.totals.vatKzt },
    { label: 'Сбор', value: result.totals.feeKzt },
  ]
  const partial = result.totals.calculatedCount < result.totals.totalCount

  return (
    <section className="rs-summary">
      <div className="rs-block">
        <span className="rs-title">Растаможка к оплате</span>
        <div className="rs-formula">
          <div className="rs-total">
            <strong>{displayNumber(result.totals.customsPaymentsKzt)} ₸</strong>
            <small>≈ {customsPaymentsUsd}</small>
          </div>
          {parts.map((part, index) => <div className="rs-part" key={part.label}>
            <span className="rs-op">{index === 0 ? '=' : '+'}</span>
            <div><span>{part.label}</span><b>{displayNumber(part.value)} <small>₸</small></b></div>
          </div>)}
        </div>
        {partial && <span className="rs-partial">Рассчитано {result.totals.calculatedCount} из {result.totals.totalCount} строк — итог неполный, остальные строки с ошибками</span>}
      </div>
      <dl className="rs-block rs-ref">
        <span className="rs-title">Справочно</span>
        <div><dt title="База для расчёта пошлины и НДС">Таможенная стоимость</dt><dd>{displayNumber(result.totals.customsValueKzt)} <small>₸</small></dd></div>
        <div><dt title="Не входит в растаможку">Доставка</dt><dd>{displayNumber(result.totals.totalDeliveryUsd)} <small>USD</small></dd></div>
        <p className="rs-ref-note">Стоимость товара и доставка — отдельные расходы, в растаможку не входят</p>
      </dl>
      <div className="rs-block rs-actions">
        <button type="button" className="btn-calculate" onClick={exportExcel} disabled={loading}>{loading ? 'Подождите…' : 'Экспорт Excel'}</button>
        <button type="button" className="btn-save-draft" onClick={saveDraft} disabled={loading}>Сохранить результат</button>
      </div>
      <ItpCalculator result={result} />
    </section>
  )
}
