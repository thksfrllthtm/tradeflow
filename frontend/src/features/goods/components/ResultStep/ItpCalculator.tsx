import { useState } from 'react'
import { NumberField } from '@/shared/components/NumberField/NumberField'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { moneyText, weightText } from '@/shared/lib/rounding'
import { GoodsCalculationResult, ItpSaved } from '@/shared/types/goods'
import { useSavedSnapshot } from '../../hooks/useSavedSnapshot'
import { calculatorInputValue, parseCalculatorNumber, weightInputValue } from './resultValues'

// «Сверка по ИТП» (свёрнута по умолчанию): ИТП × общий нетто = USD, × курс = KZT. Только для сверки,
// на расчёт не влияет. Нетто и курс заполняются из результата, их можно поправить. Введённое
// хранится в смете (saved / onSave), пока задан ИТП.
export function ItpCalculator({ result, saved, onSave, onClose }: { result: GoodsCalculationResult; saved?: ItpSaved; onSave: (value: ItpSaved | undefined) => void; onClose: () => void }) {
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

  // Карточка под полосой итогов, открывается из меню «⋯»; Esc и ✕ закрывают.
  return (
    <section className="rs-itp" aria-label="Сверка по ИТП" onKeyDown={(event) => { if (event.key === 'Escape') onClose() }}>
      <div className="rs-itp-pop">
      <div className="rs-itp-head"><strong>Сверка по ИТП</strong><button type="button" className="rs-icon" onClick={onClose} aria-label="Закрыть сверку по ИТП">✕</button></div>
      <p className="rs-itp-note">ИТП × нетто × курс — только для сверки, на расчёт не влияет</p>
      <div className="rs-its-grid">
        <label>ИТП, USD за 1 кг<NumberField value={itsValue} onChange={setItsValue} onBlur={() => setItsValue(calculatorInputValue(itsValue))} placeholder="введите" /></label>
        <span className="rs-op">×</span>
        <label>Нетто всего, кг<NumberField value={itsNetKg} onChange={setItsNetKg} onBlur={() => setItsNetKg(weightInputValue(itsNetKg))} /></label>
        <span className="rs-op">×</span>
        <label>Курс, KZT за 1 USD<NumberField value={itsRate} onChange={setItsRate} onBlur={() => setItsRate(calculatorInputValue(itsRate))} /></label>
        <span className="rs-op">=</span>
        <div className="rs-total rs-its-total">
          <strong>{itsKzt !== null ? `${formatRuNumber(moneyText(itsKzt))} KZT` : '—'}</strong>
          <small>{itsUsd !== null ? `≈ ${formatRuNumber(moneyText(itsUsd))} USD` : 'введите ИТП'}</small>
        </div>
      </div>
      </div>
    </section>
  )
}
