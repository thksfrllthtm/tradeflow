import { KeyboardEvent, useEffect, useRef, useState } from 'react'
import { GoodsParams } from '@/shared/types/goods'
import { formatMoney, formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { formatRuDate, todayIso } from '@/shared/lib/dates'
import { moneyText } from '@/shared/lib/rounding'
import { NumberField } from '@/shared/components/NumberField/NumberField'

type Props = {
  params: GoodsParams
  setParam: (field: keyof GoodsParams, value: string) => void
  loadUsdRate: () => void
  usdRateLoading: boolean
  usdRateNotice: string
  // Строки со своей упаковкой: они не меняются вместе с общим значением.
  ownPackageCount: number
}

// Страны для поля «Страна» — частые торговые партнёры; названия по-русски из браузера. Сервер узнаёт страну по
// русскому названию или двухбуквенному коду (antidumping.OriginCountry), поэтому в смету пишется название.
const COUNTRY_CODES = ['CN', 'TR', 'IN', 'VN', 'KR', 'JP', 'TW', 'TH', 'MY', 'ID', 'UZ', 'KG', 'RU', 'BY', 'AM', 'AZ', 'GE', 'UA', 'IR', 'PK', 'BD', 'AE', 'DE', 'IT', 'PL', 'ES', 'FR', 'GB', 'US', 'KZ']
const regionNames = new Intl.DisplayNames(['ru'], { type: 'region' })
const COUNTRIES = COUNTRY_CODES.map((code) => ({ code, name: regionNames.of(code) ?? code }))
const DELIVERY_MODES = [['none', 'Не учитывать'], ['total', 'Сумма'], ['wagons', 'Вагоны']] as const

// «Без сбора» — это множитель 0: сбор не считается и не входит в базу НДС.
const noFee = (multiplier: string) => multiplier.trim() !== '' && Number(multiplier) === 0
const feeTotal = (mrpText: string, multiplierText: string) => {
  const mrp = Number(mrpText.replace(',', '.'))
  const multiplier = Number(multiplierText.replace(',', '.'))
  return mrp > 0 && multiplier > 0 ? moneyText(mrp * multiplier) : ''
}

// Параметры расчёта — кнопка-сводка (дата и курс) в строке сметы; она раскрывает список, где каждый параметр
// правится сразу в своём поле. Выбор (дата, доставка) применяется сразу; текст и числа — при уходе из поля
// или по Enter, Esc отменяет ввод. Щелчок мимо закрывает список, введённое при этом сохраняется.
export function ParamChips({ params, setParam, loadUsdRate, usdRateLoading, usdRateNotice, ownPackageCount }: Props) {
  const [menuOpen, setMenuOpen] = useState(false)
  const [draft, setDraft] = useState<Partial<Record<keyof GoodsParams, string>>>({})
  const menu = useRef<HTMLDivElement>(null)

  const saved = (field: keyof GoodsParams) => String(params[field] ?? '')
  const value = (field: keyof GoodsParams) => draft[field] ?? saved(field)
  const edit = (field: keyof GoodsParams, next: string) => setDraft((current) => ({ ...current, [field]: next }))
  function commit(field: keyof GoodsParams) {
    const next = draft[field]
    if (next === undefined) return
    if (next !== saved(field)) setParam(field, next)
    setDraft(({ [field]: _done, ...rest }) => rest)
  }
  function close(keep: boolean) {
    if (keep) for (const [field, next] of Object.entries(draft) as [keyof GoodsParams, string][]) if (next !== saved(field)) setParam(field, next)
    setDraft({})
    setMenuOpen(false)
    setCountryOpen(false)
  }

  // Страна: свой список вместо datalist (тот рисует браузер, его не оформить). Пока ничего не введено —
  // все страны, при вводе — подходящие по названию или коду. Стрелки выбирают, Enter подставляет.
  const [countryOpen, setCountryOpen] = useState(false)
  const [countryActive, setCountryActive] = useState(-1)
  const countryQuery = (draft.origin ?? '').trim().toLowerCase()
  const countries = draft.origin === undefined ? COUNTRIES
    : COUNTRIES.filter((country) => country.name.toLowerCase().includes(countryQuery) || country.code.toLowerCase() === countryQuery)
  const activeCountry = countryOpen ? countries[countryActive] : undefined
  useEffect(() => { if (activeCountry) document.getElementById(`rv-country-${activeCountry.code}`)?.scrollIntoView({ block: 'nearest' }) }, [activeCountry])
  function chooseCountry(name: string) {
    setDraft(({ origin: _done, ...rest }) => rest)
    if (name !== saved('origin')) setParam('origin', name)
    setCountryOpen(false)
  }
  function countryKeys(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      const step = event.key === 'ArrowDown' ? 1 : -1
      setCountryOpen(true)
      setCountryActive((index) => countries.length === 0 ? -1 : index < 0 ? (step > 0 ? 0 : countries.length - 1) : (index + step + countries.length) % countries.length)
      return
    }
    if (event.key === 'Enter' && activeCountry) { event.preventDefault(); chooseCountry(activeCountry.name); return }
    if (event.key === 'Escape' && countryOpen) { event.preventDefault(); setCountryOpen(false); return }
    if (event.key === 'Enter') setCountryOpen(false)
    fieldKeys('origin')(event)
  }

  useEffect(() => {
    if (!menuOpen) return
    const outside = (event: MouseEvent) => { if (!menu.current?.contains(event.target as Node)) close(true) }
    const escape = (event: globalThis.KeyboardEvent) => { if (event.key === 'Escape' && !event.defaultPrevented) close(false) }
    document.addEventListener('mousedown', outside)
    document.addEventListener('keydown', escape)
    return () => { document.removeEventListener('mousedown', outside); document.removeEventListener('keydown', escape) }
  })

  // Enter — применить поле; Esc в поле с вводом — вернуть сохранённое значение (список остаётся открытым).
  const fieldKeys = (field: keyof GoodsParams) => (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') { event.preventDefault(); commit(field) }
    if (event.key === 'Escape' && draft[field] !== undefined) { event.preventDefault(); setDraft(({ [field]: _skip, ...rest }) => rest) }
  }
  const num = (field: keyof GoodsParams, label: string, unit = '', empty = '—', narrow = false) => <span className={narrow ? 'rv-unit-field narrow' : 'rv-unit-field'}>
    <NumberField value={value(field)} onChange={(next) => edit(field, next)} onBlur={() => commit(field)} onKeyDown={fieldKeys(field)} aria-label={label} placeholder={empty}
      style={{ paddingRight: unit && value(field) ? `${unit.length * 7 + 16}px` : undefined }} />
    {unit && value(field) && <i>{unit}</i>}
  </span>

  const fee = noFee(value('feeMultiplier')) ? 'не начисляется'
    : params.customsFeeKzt ? formatMoney(params.customsFeeKzt, 'KZT')
      : feeTotal(value('mrp'), value('feeMultiplier')) ? formatMoney(feeTotal(value('mrp'), value('feeMultiplier')), 'KZT') : '—'
  const delivery = params.deliveryMode === 'none' ? 'не учитывается'
    : params.deliveryMode === 'total' ? formatMoney(params.deliveryTotalUsd, 'USD')
      : params.deliveryMode === 'wagons' ? `${formatRuNumber(params.wagons) || '—'} ваг. × ${formatMoney(params.wagonTariffUsd, 'USD')}`
        : ''
  // Все параметры видны сразу; пустые дата и курс — красным (без них расчёт не пойдёт), прочие пустые — серым.
  const shown: { label: string; text: string; warn?: boolean; empty?: boolean }[] = [
    { label: 'Дата', text: params.assessmentDate ? formatRuDate(params.assessmentDate) : 'не указана', warn: !params.assessmentDate },
    { label: 'Курс USD', text: params.usdKzt ? formatMoney(params.usdKzt, 'KZT') : 'не указан', warn: !params.usdKzt },
    { label: 'Доставка', text: delivery || 'не указана', empty: !delivery },
    { label: 'Упаковка', text: params.packageWeightKg ? `${formatRuNumber(params.packageWeightKg)} кг/место` : 'не указана', empty: !params.packageWeightKg },
    { label: 'Сбор', text: fee, empty: fee === '—' },
    { label: 'Страна', text: params.origin || 'не указана', empty: !params.origin },
    ...(params.customsValueMode === 'sir' ? [{ label: 'Там. стоимость', text: `по СУР${Number(params.sirDiscountPercent) > 0 ? ` −${params.sirDiscountPercent}%` : ''}` }] : []),
  ]

  return <div className="rv-params" ref={menu}>
    {/* Сводка только показывает значения; список открывает одна кнопка «Изменить параметры». */}
    <div className="rv-params-summary">
      {shown.map((item) => <span key={item.label} className={`rv-params-item${item.warn ? ' warn' : item.empty ? ' empty' : ''}`}><span>{item.label}</span><b>{item.text}</b></span>)}
      <button type="button" className={menuOpen ? 'rv-params-edit open' : 'rv-params-edit'}
        onClick={() => menuOpen ? close(true) : setMenuOpen(true)} aria-expanded={menuOpen} aria-haspopup="true">Изменить параметры
        <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><path d="M3 4.5l3 3 3-3" /></svg>
      </button>
    </div>

    {menuOpen && <div className="rv-params-form" role="group" aria-label="Параметры расчёта">
      <div className="rv-param">
        <span>Дата</span>
        <div className="rv-param-inputs">
          <input type="date" value={params.assessmentDate} onChange={(event) => setParam('assessmentDate', event.target.value)} aria-label="Дата расчёта" />
          <button type="button" className="secondary sm rv-param-link" onClick={() => setParam('assessmentDate', todayIso())} disabled={params.assessmentDate === todayIso()}>Сегодня</button>
        </div>
      </div>

      <div className="rv-param">
        <span>Курс USD</span>
        <div className="rv-param-inputs">
          {num('usdKzt', 'Курс за 1 USD', 'KZT')}
          <button type="button" className={usdRateNotice && params.usdKzt ? 'secondary sm rv-param-link active' : 'secondary sm rv-param-link'} onClick={() => void loadUsdRate()} disabled={usdRateLoading || !params.assessmentDate}
            title={params.assessmentDate ? `Взять курс НБРК на ${formatRuDate(params.assessmentDate)}` : 'Сначала укажите дату расчёта'}>{usdRateLoading ? '…' : 'НБРК'}</button>
        </div>
      </div>

      <div className="rv-param">
        <span>Доставка</span>
        <div className="rv-param-inputs">
          <div className="dir-seg" role="radiogroup" aria-label="Как учитывать доставку">
            {DELIVERY_MODES.map(([mode, label]) => <button key={mode} type="button" role="radio" aria-checked={params.deliveryMode === mode}
              className={params.deliveryMode === mode ? 'active' : undefined} onClick={() => setParam('deliveryMode', mode)}>{label}</button>)}
          </div>
        </div>
        {params.deliveryMode === 'total' && <div className="rv-param-inputs rv-param-more">{num('deliveryTotalUsd', 'Доставка за партию', 'USD')}</div>}
        {params.deliveryMode === 'wagons' && <div className="rv-param-inputs rv-param-more">
          {num('wagons', 'Вагонов', 'ваг.', '—', true)}<span className="rv-param-op">×</span>{num('wagonTariffUsd', 'Тариф за вагон', 'USD')}
        </div>}
      </div>

      <div className="rv-param">
        <span>Упаковка</span>
        <div className="rv-param-inputs">{num('packageWeightKg', 'Вес упаковки на место', 'кг/место', 'не указана')}</div>
        {ownPackageCount > 0 && <p className="rv-param-note">Своя упаковка у {formatRuNumber(String(ownPackageCount))} строк — они не изменятся</p>}
      </div>

      <div className="rv-param">
        <span>Сбор</span>
        <div className="rv-param-inputs">
          {num('mrp', 'МРП', 'KZT')}<span className="rv-param-op">×</span>{num('feeMultiplier', 'Множитель сбора (0 — без сбора)', '', '—', true)}
        </div>
        <p className="rv-param-note total">= <b>{fee}</b></p>
      </div>

      <div className="rv-param">
        <span>Страна</span>
        <div className="rv-param-inputs rv-combo">
          <input role="combobox" aria-expanded={countryOpen} aria-controls="rv-countries" aria-autocomplete="list" aria-activedescendant={activeCountry ? `rv-country-${activeCountry.code}` : undefined}
            value={value('origin')} onChange={(event) => { edit('origin', event.target.value); setCountryOpen(true); setCountryActive(-1) }}
            onFocus={() => setCountryOpen(true)} onClick={() => setCountryOpen(true)} onBlur={() => { commit('origin'); setCountryOpen(false) }} onKeyDown={countryKeys}
            placeholder="не указана" aria-label="Страна происхождения" title="Нужна для антидемпинговых пошлин" autoComplete="off" />
          {countryOpen && countries.length > 0 && <div id="rv-countries" className="rv-combo-list" role="listbox" aria-label="Страны">
            {countries.map((country, index) => <div key={country.code} id={`rv-country-${country.code}`} role="option" aria-selected={country.name === params.origin}
              className={index === countryActive ? 'active' : undefined} onMouseDown={(event) => event.preventDefault()} onMouseEnter={() => setCountryActive(index)}
              onClick={() => chooseCountry(country.name)}><span>{country.name}</span><small>{country.code}</small></div>)}
          </div>}
        </div>
      </div>

      {params.customsValueMode === 'sir' && <div className="rv-param" title="Как считать таможенную стоимость — переключатель на этапе «Результат»">
        <span>Там. стоимость</span>
        <b className="rv-param-static">по СУР{Number(params.sirDiscountPercent) > 0 ? ` −${params.sirDiscountPercent}%` : ''}</b>
      </div>}
    </div>}
  </div>
}
