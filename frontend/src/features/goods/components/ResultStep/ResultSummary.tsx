import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { formatRuDate } from '@/shared/lib/dates'
import { moneyText } from '@/shared/lib/rounding'
import { GoodsCalculationResult, ItpSaved } from '@/shared/types/goods'
import { CalculatorActivity } from '../../types'
import { storageFailed } from '../../lib/storageNotice'
import { ItpCalculator } from './ItpCalculator'
import { displayNumber, hasAntiDumping, parseCalculatorNumber } from './resultValues'
import './ResultSummary.css'

type SummaryProps = {
  result: GoodsCalculationResult
  loading: boolean
  activity: CalculatorActivity
  storageNotice: string
  delivery: boolean
  saveDraft: () => void
  exportExcel: () => void
  itp?: ItpSaved
  onItpChange: (value: ItpSaved | undefined) => void
}

// Полоса итогов над таблицей: растаможка как формула из слагаемых, справочные суммы, состояние
// сохранения и кнопки. Пока идёт пересчёт, итог приглушён и помечен «пересчёт…».
export function ResultSummary({ result, loading, activity, storageNotice, delivery, saveDraft, exportExcel, itp, onItpChange }: SummaryProps) {
  const customsPaymentsUsd = (() => {
    const kzt = parseCalculatorNumber(result.totals.customsPaymentsKzt) ?? NaN
    const rate = parseCalculatorNumber(result.usdKzt) ?? NaN
    return Number.isFinite(kzt) && Number.isFinite(rate) && rate > 0
      ? `${formatRuNumber(moneyText(kzt / rate))} USD`
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
  const calculating = activity === 'calculate'
  const failed = storageFailed(storageNotice)

  return (
    <section className={calculating ? 'rs-summary calculating' : 'rs-summary'} aria-label="Итоги расчёта" aria-busy={calculating}>
      <div className="rs-sum-total">
        <span className="rs-sum-label">Растаможка к оплате</span>
        <strong aria-live="polite">{displayNumber(result.totals.customsPaymentsKzt)} ₸</strong>
        <span className="rs-sum-sub">≈ {customsPaymentsUsd}{calculating && <em> · пересчёт…</em>}</span>
        {partial && <span className="rs-partial" title="Остальные строки с ошибками: они в итог не вошли">Итог неполный: рассчитано {result.totals.calculatedCount} из {result.totals.totalCount} строк</span>}
      </div>
      <div className="rs-sum-body">
        <dl className="rs-sum-parts">
          {parts.map((part, index) => <div className="rs-part" key={part.label}>
            <span className="rs-op" aria-hidden="true">{index === 0 ? '=' : '+'}</span>
            <dt>{part.label}</dt>
            <dd>{displayNumber(part.value)} <small>₸</small></dd>
          </div>)}
        </dl>
        <dl className="rs-sum-ref">
          <div><dt title="База для расчёта пошлины и НДС">Таможенная стоимость</dt><dd>{displayNumber(result.totals.customsValueKzt)} ₸</dd></div>
          {delivery && <div title="Стоимость товара и доставка — отдельные расходы, в растаможку не входят"><dt>Доставка</dt><dd>{displayNumber(result.totals.totalDeliveryUsd)} USD <small className="rs-wide">· не входит в растаможку</small></dd></div>}
          {result.usdKzt && <div><dt>Курс USD</dt><dd>{displayNumber(result.usdKzt)} ₸{result.assessmentDate && <small> на {formatRuDate(result.assessmentDate)}</small>}</dd></div>}
        </dl>
      </div>
      <div className="rs-sum-actions">
        <div className="rs-sum-meta">
          <p className={failed ? 'rs-save-state failed' : 'rs-save-state'} role={failed ? 'alert' : 'status'} title={storageNotice}>{storageNotice || 'Изменения сохраняются автоматически'}</p>
          <ItpCalculator result={result} saved={itp} onSave={onItpChange} />
        </div>
        <div className="rs-sum-buttons">
          <button type="button" className="rs-save" onClick={saveDraft} disabled={loading} title="Сохранить на сервере сейчас, не дожидаясь автосохранения (Ctrl+S)">{activity === 'save' ? 'Сохраняем…' : 'Сохранить'}</button>
          <button type="button" className="btn-calculate rs-export" onClick={exportExcel} disabled={loading}>{activity === 'export' ? 'Готовим файл…' : 'Экспорт Excel'}</button>
        </div>
      </div>
    </section>
  )
}
