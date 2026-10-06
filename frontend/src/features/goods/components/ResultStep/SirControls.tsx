import { ReactNode, useEffect, useState } from 'react'
import { SirParams, SirWeight } from '@/shared/types/goods'
import './SirControls.css'

const QUICK_DISCOUNTS = ['0', '50', '60', '70']

// «−60» и «60 %» — одно и то же: снижение индикатора на 60 %, то есть 40 % от СИР.
const discountValue = (text: string) => text.replace(',', '.').replace(/[−-]/g, '').replace('%', '').trim()
const validDiscount = (text: string) => text === '' || (/^\d+(\.\d+)?$/.test(text) && Number(text) < 100)
const sameDiscount = (a: string, b: string) => Number(a || 0) === Number(b || 0)

// Вес и снижение СИР: в полосе сметы и в окне сравнения.
export function SirOptions({ weight, discount, disabled, onChange }: { weight: SirWeight; discount: string; disabled: boolean; onChange: (patch: SirParams) => void }) {
  // Своё снижение — в поле рядом с кнопками; пока выбрана кнопка, поле пустое.
  const custom = !QUICK_DISCOUNTS.some((value) => sameDiscount(value, discount))
  const [customText, setCustomText] = useState(custom ? discount : '')
  useEffect(() => setCustomText(custom ? discount : ''), [discount, custom])
  const typedDiscount = discountValue(customText)
  const discountOk = validDiscount(typedDiscount)
  const resetCustom = () => setCustomText(custom ? discount : '')

  function applyDiscount() {
    if (!discountOk || typedDiscount === '') { resetCustom(); return }
    if (!sameDiscount(typedDiscount, discount)) onChange({ sirDiscountPercent: typedDiscount })
  }

  return <>
    <div className="rs-seg" role="radiogroup" aria-label="Вес для СИР">
      <button type="button" role="radio" aria-checked={weight !== 'gross'} className={weight !== 'gross' ? 'active' : undefined} onClick={() => onChange({ sirWeight: 'net' })} disabled={disabled}>Нетто</button>
      <button type="button" role="radio" aria-checked={weight === 'gross'} className={weight === 'gross' ? 'active' : undefined} onClick={() => onChange({ sirWeight: 'gross' })} disabled={disabled}>Брутто</button>
    </div>
    <span className="sir-sep" aria-hidden="true" />
    <span className="sir-field">Снижение</span>
    <div className="rs-seg" role="radiogroup" aria-label="Снижение индикатора">
      {QUICK_DISCOUNTS.map((value) => {
        const active = sameDiscount(value, discount)
        return <button key={value} type="button" role="radio" aria-checked={active} className={active ? 'active' : undefined} onClick={() => onChange({ sirDiscountPercent: value === '0' ? '' : value })} disabled={disabled}
          title={value === '0' ? 'Индикатор без снижения' : `Берётся ${100 - Number(value)} % индикатора`}>{value === '0' ? '0' : `−${value}`}</button>
      })}
    </div>
    <label className={['rs-joined', 'sir-custom', custom ? 'active' : '', discountOk ? '' : 'invalid'].filter(Boolean).join(' ')} title="Своё снижение, %: Enter — применить">
      <input inputMode="decimal" value={customText} onChange={(event) => setCustomText(event.target.value)} onBlur={applyDiscount}
        onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); applyDiscount() } else if (event.key === 'Escape') resetCustom() }}
        placeholder="своё" disabled={disabled} aria-label="Своё снижение индикатора, %" aria-invalid={!discountOk} />
      <i>%</i>
    </label>
  </>
}

type Props = {
  sir: SirParams
  // Строки, посчитанные по СИР, и включённые строки сметы.
  sirRows: number
  totalRows: number
  disabled: boolean
  onChange: (patch: SirParams) => void
  onCompare: () => void
  // По инвойсу: подбор под бюджет — после переключателя, откат цен — справа. По СИР они не нужны.
  invoiceTools: ReactNode
  priceTools: ReactNode
}

// Полоса инструментов результата. Таможенная стоимость сметы: по инвойсу (обычно) или по стоимостному индикатору риска КГД —
// вес нетто или брутто × СИР × (1 − снижение %). Строки без индикатора считаются по инвойсу.
export function SirControls({ sir, sirRows, totalRows, disabled, onChange, onCompare, invoiceTools, priceTools }: Props) {
  const mode = sir.customsValueMode === 'sir' ? 'sir' : 'invoice'
  const country = sir.sirCountry || 'CN'
  const [countryText, setCountryText] = useState(country)
  useEffect(() => setCountryText(country), [country])
  const countryOk = /^([A-Za-z]{2}|00)$/.test(countryText.trim())

  function applyCountry() {
    const value = countryText.trim().toUpperCase()
    if (!countryOk) { setCountryText(country); return }
    if (value !== country) onChange({ sirCountry: value })
  }

  return <div className={mode === 'sir' ? 'sir-bar on' : 'sir-bar'} role="group" aria-label="Таможенная стоимость">
    <span className="sir-label">Таможенная стоимость</span>
    <div className="rs-seg" role="radiogroup" aria-label="Как считать таможенную стоимость">
      <button type="button" role="radio" aria-checked={mode === 'invoice'} className={mode === 'invoice' ? 'active' : undefined} onClick={() => onChange({ customsValueMode: 'invoice' })} disabled={disabled} title="ТС = стоимость по инвойсу + доставка">По инвойсу</button>
      <button type="button" role="radio" aria-checked={mode === 'sir'} className={mode === 'sir' ? 'active' : undefined} onClick={() => onChange({ customsValueMode: 'sir' })} disabled={disabled} title="ТС = вес × стоимостной индикатор риска КГД (USD за 1 кг) × (1 − снижение) + доставка">По СИР</button>
    </div>
    {mode === 'invoice' && <>
      <span className="sir-sep" aria-hidden="true" />
      {invoiceTools}
    </>}
    {mode === 'sir' && <>
      <span className="sir-sep" aria-hidden="true" />
      <SirOptions weight={sir.sirWeight === 'gross' ? 'gross' : 'net'} discount={sir.sirDiscountPercent ?? ''} disabled={disabled} onChange={onChange} />
      <span className="sir-sep" aria-hidden="true" />
      <label className="sir-field" title="Колонка страны в файлах профилей риска; файлы «…_прил CN» — для Китая">
        Страна
        <input className={countryOk ? 'sir-country' : 'sir-country invalid'} value={countryText} maxLength={2} onChange={(event) => setCountryText(event.target.value.toUpperCase())} onBlur={applyCountry}
          onKeyDown={(event) => { if (event.key === 'Enter') { event.preventDefault(); applyCountry() } else if (event.key === 'Escape') setCountryText(country) }}
          disabled={disabled} aria-label="Страна для СИР, двухбуквенный код" aria-invalid={!countryOk} />
      </label>
    </>}
    <span className="sir-end">
      {mode === 'sir' && <span className="sir-count" role="status" title="Цены инвойса на таможенную стоимость не влияют; строки без индикатора — по инвойсу">по СИР <b>{sirRows}</b> из {totalRows}{sirRows < totalRows && <span> · остальные по инвойсу</span>}</span>}
      {mode === 'invoice' && priceTools}
      <button type="button" className="sir-compare" onClick={onCompare} disabled={disabled} title="Сравнить инвойс и СИР: итоги и каждая строка рядом">Сравнить с СИР</button>
    </span>
  </div>
}
