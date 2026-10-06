import { useState } from 'react'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { moneyText, weightText } from '@/shared/lib/rounding'
import { GoodsCalculationResult, ItpSaved } from '@/shared/types/goods'
import { useSavedSnapshot } from '../../hooks/useSavedSnapshot'
import { calculatorInputValue, parseCalculatorNumber, weightInputValue } from './resultValues'

// «Сверка по ИТП» (свёрнута по умолчанию): ИТП × общий нетто = USD, × курс = ₸. Только для сверки,
// на расчёт не влияет. Нетто и курс заполняются из результата, их можно поправить. Введённое
// хранится в смете (saved / onSave), пока задан ИТП.
export function ItpCalculator({ result, saved, onSave }: { result: GoodsCalculationResult; saved?: ItpSaved; onSave: (value: ItpSaved | undefined) => void }) {
  const defaultNetKg = () => {
    const total = result.items.reduce((sum, item) => {
      if (item.excluded) return sum
      return sum + (parseCalculatorNumber(item.netKg) ?? 0)
    }, 0)
    return total > 0 ? weightText(total, true) : ''
  }
  const [itsValue, setItsValue] = useState(() => saved?.value ?? '')
  const [itsNetKg, setItsNetKg] = useState(() => saved?.netKg ?? defaultNetKg())
  const [itsRate, setItsRate] = useState(() => saved?.rate ?? calculatorInputValue(result.usdKzt))
  useSavedSnapshot(saved, itsValue.trim() ? { value: itsValue, netKg: itsNetKg, rate: itsRate } : undefined, onSave, (value) => {
    setItsValue(value?.value ?? '')
    setItsNetKg(value?.netKg ?? defaultNetKg())
    setItsRate(value?.rate ?? calculatorInputValue(result.usdKzt))
  })

  const its = parseCalculatorNumber(itsValue)
  const calculatorNetKg = parseCalculatorNumber(itsNetKg)
  const exchangeRate = parseCalculatorNumber(itsRate)
  const itsUsd = its !== null && its > 0 && calculatorNetKg !== null && calculatorNetKg > 0
    ? its * calculatorNetKg
    : null
  const itsKzt = itsUsd !== null && exchangeRate !== null && exchangeRate > 0
    ? itsUsd * exchangeRate
    : null

  // Окошко под кнопкой в полосе итогов; Esc закрывает и возвращает фокус на кнопку.
  return (
    <details className="rs-itp" onKeyDown={(event) => {
      if (event.key !== 'Escape' || !event.currentTarget.open) return
      event.currentTarget.open = false
      event.currentTarget.querySelector('summary')?.focus()
    }}>
      <summary>Сверка по ИТП</summary>
      <div className="rs-itp-pop" role="group" aria-label="Сверка по ИТП">
      <p className="rs-itp-note">ИТП × нетто × курс — только для сверки, на расчёт не влияет</p>
      <div className="rs-its-grid">
        <label>ИТП, USD за 1 кг<input inputMode="decimal" value={itsValue} onChange={(event) => setItsValue(event.target.value.replace(',', '.'))} onBlur={() => setItsValue(calculatorInputValue(itsValue))} placeholder="введите" /></label>
        <span className="rs-op">×</span>
        <label>Нетто всего, кг<input inputMode="decimal" value={itsNetKg} onChange={(event) => setItsNetKg(event.target.value.replace(',', '.'))} onBlur={() => setItsNetKg(weightInputValue(itsNetKg))} /></label>
        <span className="rs-op">×</span>
        <label>Курс, ₸ за 1 USD<input inputMode="decimal" value={itsRate} onChange={(event) => setItsRate(event.target.value.replace(',', '.'))} onBlur={() => setItsRate(calculatorInputValue(itsRate))} /></label>
        <span className="rs-op">=</span>
        <div className="rs-total rs-its-total">
          <strong>{itsKzt !== null ? `${formatRuNumber(moneyText(itsKzt))} ₸` : '—'}</strong>
          <small>{itsUsd !== null ? `≈ ${formatRuNumber(moneyText(itsUsd))} USD` : 'введите ИТП'}</small>
        </div>
      </div>
      </div>
    </details>
  )
}
