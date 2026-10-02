import { useState } from 'react'
import { formatDisplayNumber } from '@/shared/lib/formatDisplayNumber'
import { moneyText, weightText } from '@/shared/lib/rounding'
import { GoodsCalculationResult } from '@/shared/types/goods'
import { calculatorInputValue, parseCalculatorNumber, weightInputValue } from './resultValues'

// «Сверка по ИТП» (свёрнута по умолчанию): ИТП × общий нетто = USD, × курс = ₸. Только для сверки,
// на расчёт не влияет. Нетто и курс заполняются из результата, их можно поправить.
export function ItpCalculator({ result }: { result: GoodsCalculationResult }) {
  const [itsValue, setItsValue] = useState('')
  const [itsNetKg, setItsNetKg] = useState(() => {
    const total = result.items.reduce((sum, item) => {
      if (item.excluded) return sum
      return sum + (parseCalculatorNumber(item.netKg) ?? 0)
    }, 0)
    return total > 0 ? weightText(total, true) : ''
  })
  const [itsRate, setItsRate] = useState(() => calculatorInputValue(result.usdKzt))

  const its = parseCalculatorNumber(itsValue)
  const calculatorNetKg = parseCalculatorNumber(itsNetKg)
  const exchangeRate = parseCalculatorNumber(itsRate)
  const itsUsd = its !== null && its > 0 && calculatorNetKg !== null && calculatorNetKg > 0
    ? its * calculatorNetKg
    : null
  const itsKzt = itsUsd !== null && exchangeRate !== null && exchangeRate > 0
    ? itsUsd * exchangeRate
    : null

  return (
    <details className="rs-block rs-its">
      <summary className="rs-title">Сверка по ИТП <small>на расчёт не влияет</small></summary>
      <div className="rs-its-grid">
        <label>ИТП, USD за 1 кг<input inputMode="decimal" value={itsValue} onChange={(event) => setItsValue(event.target.value.replace(',', '.'))} onBlur={() => setItsValue(calculatorInputValue(itsValue))} placeholder="введите" /></label>
        <span className="rs-op">×</span>
        <label>Нетто всего, кг<input inputMode="decimal" value={itsNetKg} onChange={(event) => setItsNetKg(event.target.value.replace(',', '.'))} onBlur={() => setItsNetKg(weightInputValue(itsNetKg))} /></label>
        <span className="rs-op">×</span>
        <label>Курс, ₸ за 1 USD<input inputMode="decimal" value={itsRate} onChange={(event) => setItsRate(event.target.value.replace(',', '.'))} onBlur={() => setItsRate(calculatorInputValue(itsRate))} /></label>
        <span className="rs-op">=</span>
        <div className="rs-total rs-its-total">
          <strong>{itsKzt !== null ? `${formatDisplayNumber(moneyText(itsKzt))} ₸` : '—'}</strong>
          <small>{itsUsd !== null ? `≈ ${formatDisplayNumber(moneyText(itsUsd))} USD` : 'введите ИТП'}</small>
        </div>
      </div>
    </details>
  )
}
