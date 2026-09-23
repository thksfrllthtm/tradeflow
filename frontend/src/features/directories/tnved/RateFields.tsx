import { useState } from 'react'
import { TNVEDRate } from './types'

// Поля одной ставки (пошлина или НДС): тип и значение в одну строку, источник и период — по раскрытию.
export function RateFields({ label, rate, onChange, vat = false }: { label: string; rate: TNVEDRate; onChange: (value: Partial<TNVEDRate>) => void; vat?: boolean }) {
  const unknown = vat ? 'Не указан' : 'Не указана'
  const typeChanged = (type: TNVEDRate['type']) => onChange({ type, percent: '', amount: '', currency: '', unit: '', basis: '', validFrom: '', validTo: '', checkedAt: '', source: '' })
  const hasPercent = rate.type === 'percentage' || rate.type === 'combined_minimum'
  const hasAmount = rate.type === 'specific' || rate.type === 'combined_minimum'
  const sourceFilled = Boolean(rate.checkedAt || rate.validFrom || rate.validTo || rate.source)
  const [sourceInitiallyOpen] = useState(sourceFilled)
  return <div className="drawer-rate">
    <b>{label}</b>
    <div className="drawer-row">
      <label className="drawer-field"><span>Тип</span><select value={rate.type} onChange={(event) => typeChanged(event.target.value as TNVEDRate['type'])}><option value="unknown">{unknown}</option><option value="percentage">Процентная</option>{!vat && <><option value="specific">За единицу</option><option value="combined_minimum">Процент, но не менее суммы</option></>}{vat && <option value="exempt">Освобождение</option>}</select></label>
      {hasPercent && <label className="drawer-field"><span>Процент</span><span className="drawer-suffix"><input inputMode="decimal" value={rate.percent} onChange={(event) => onChange({ percent: event.target.value.replace(',', '.') })} placeholder="12" /><i>%</i></span></label>}
    </div>
    {hasAmount && <div className="drawer-row">
      <label className="drawer-field"><span>Сумма за единицу</span><input inputMode="decimal" value={rate.amount} onChange={(event) => onChange({ amount: event.target.value.replace(',', '.') })} placeholder="2" /></label>
      <label className="drawer-field"><span>Валюта</span><input value={rate.currency} onChange={(event) => onChange({ currency: event.target.value.toUpperCase() })} placeholder="EUR" maxLength={3} /></label>
      <label className="drawer-field"><span>Единица</span><input value={rate.unit} onChange={(event) => onChange({ unit: event.target.value })} placeholder="пару" /></label>
    </div>}
    {rate.type === 'exempt' && <label className="drawer-field"><span>Основание освобождения</span><textarea rows={2} value={rate.basis ?? ''} onChange={(event) => onChange({ basis: event.target.value })} required /></label>}
    {rate.type !== 'unknown' && <details open={sourceInitiallyOpen || undefined}>
      <summary>Источник и период действия{sourceFilled ? ' · заполнено' : ''}</summary>
      <div>
        <div className="drawer-row">
          <label className="drawer-field"><span>Проверено</span><input type="date" value={rate.checkedAt} onChange={(event) => onChange({ checkedAt: event.target.value })} /></label>
          <label className="drawer-field"><span>Действует с</span><input type="date" value={rate.validFrom} onChange={(event) => onChange({ validFrom: event.target.value })} /></label>
          <label className="drawer-field"><span>по</span><input type="date" value={rate.validTo} onChange={(event) => onChange({ validTo: event.target.value })} /></label>
        </div>
        <label className="drawer-field"><span>Источник</span><input value={rate.source} onChange={(event) => onChange({ source: event.target.value })} placeholder="Текст или ссылка" /></label>
      </div>
    </details>}
  </div>
}
