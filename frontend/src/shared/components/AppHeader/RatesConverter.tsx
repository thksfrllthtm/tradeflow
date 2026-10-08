import { KeyboardEvent, useEffect, useRef, useState } from 'react'
import { mainRates, TodayRate } from '@/shared/api/todayRates'
import { formatRuDate } from '@/shared/lib/dates'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { decimalText, moneyText } from '@/shared/lib/rounding'

// Сумма в другой валюте по курсам НБРК (тенге за 1 единицу; у KZT — 1). null — не число или нет курса.
export function convert(amount: string, from: string, to: string, rates: TodayRate[]): number | null {
  const value = Number(amount.replace(/\s/g, '').replace(',', '.'))
  const rate = (currency: string) => currency === 'KZT' ? 1 : Number(rates.find((item) => item.currency === currency)?.rate)
  if (amount.trim() === '' || !Number.isFinite(value) || !(rate(from) > 0) || !(rate(to) > 0)) return null
  return value * rate(from) / rate(to)
}

// Сумма для показа: от единицы — копейки; меньше единицы (1 ₸ ≈ 0,0022 USD) — до 6 знаков, не «0,00».
export function amountText(value: number) {
  return formatRuNumber(Math.abs(value) >= 1 || value === 0 ? moneyText(value) : decimalText(value, 6))
}

// Названия валют по-русски — из браузера («доллар США», «китайский юань»).
const names = new Intl.DisplayNames(['ru'], { type: 'currency' })
const nameOf = (code: string) => names.of(code) ?? code
const FREQUENT = ['KZT', 'USD', 'EUR', 'CNY', 'RUB']
const symbol = (code: string) => code === 'KZT' ? '₸' : code

// Выбор валюты: плашка с кодом и названием; список — с поиском по коду и названию, частые валюты сверху,
// у каждой — курс в тенге. ↑ ↓ — выбор, Enter — применить, Esc — закрыть список.
function CurrencySelect({ value, codes, rates, label, onChange }: { value: string; codes: string[]; rates: TodayRate[]; label: string; onChange: (code: string) => void }) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const listRef = useRef<HTMLUListElement>(null)
  const needle = query.trim().toLowerCase()
  const found = codes.filter((code) => !needle || code.toLowerCase().includes(needle) || nameOf(code).toLowerCase().includes(needle))
  const rateOf = (code: string) => code === 'KZT' ? '' : `${formatRuNumber(rates.find((rate) => rate.currency === code)?.rate)} ₸`

  useEffect(() => setActive(0), [query])
  useEffect(() => { listRef.current?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' }) }, [active, open])

  function pick(code: string) { onChange(code); setOpen(false); setQuery('') }
  function onKey(event: KeyboardEvent) {
    if (event.key === 'ArrowDown') { event.preventDefault(); setActive((index) => Math.min(index + 1, found.length - 1)) }
    if (event.key === 'ArrowUp') { event.preventDefault(); setActive((index) => Math.max(index - 1, 0)) }
    if (event.key === 'Enter') { event.preventDefault(); if (found[active]) pick(found[active]) }
    // Esc закрывает только список, окно конвертера остаётся.
    if (event.key === 'Escape') { event.stopPropagation(); setOpen(false); setQuery('') }
  }

  return <div className="cur-select">
    <button type="button" className={open ? 'cur-select-button open' : 'cur-select-button'} onClick={() => setOpen(!open)} aria-haspopup="listbox" aria-expanded={open} aria-label={`${label}: ${value} — ${nameOf(value)}`}>
      <b>{value}</b><span>{nameOf(value)}</span>
      <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><path d="M2.5 4l2.5 2.5L7.5 4" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" /></svg>
    </button>
    {open && <div className="cur-list" onKeyDown={onKey}>
      <input className="cur-search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Код или название: юань, CNY" aria-label="Поиск валюты" autoFocus />
      <ul role="listbox" aria-label={label} ref={listRef}>
        {found.length === 0 && <li className="cur-empty">Нет такой валюты</li>}
        {found.map((code, index) => <li key={code} role="option" aria-selected={index === active}
          className={[code === value ? 'chosen' : '', !needle && code === FREQUENT[FREQUENT.length - 1] ? 'cur-split' : ''].filter(Boolean).join(' ')}
          onMouseDown={(event) => { event.preventDefault(); pick(code) }} onMouseEnter={() => setActive(index)}>
          <b>{code}</b><span>{nameOf(code)}</span><small>{rateOf(code)}</small>
        </li>)}
      </ul>
    </div>}
  </div>
}

// Курс НБРК в шапке (USD и EUR); нажатие открывает конвертер по всем валютам НБРК.
export function RatesConverter({ rates }: { rates: TodayRate[] }) {
  // Сначала тенге и частые валюты, остальные — по алфавиту.
  const codes = [...FREQUENT.filter((code) => code === 'KZT' || rates.some((rate) => rate.currency === code)), ...rates.map((rate) => rate.currency).filter((code) => !FREQUENT.includes(code)).sort()]
  const shown = mainRates(rates)
  const [amount, setAmount] = useState('1')
  const [from, setFrom] = useState('USD')
  const [to, setTo] = useState('KZT')
  const ref = useRef<HTMLDetailsElement>(null)
  // Содержимое окна создаётся при каждом открытии: списки валют закрыты, курсор — в сумме.
  const [open, setOpen] = useState(false)
  const result = convert(amount, from, to, rates)
  const unit = convert('1', from, to, rates)

  // Закрывается щелчком вне окна и клавишей Esc. Путь события, а не target: выбранная строка списка
  // к этому моменту уже убрана со страницы, и клик по ней иначе считался бы щелчком снаружи.
  useEffect(() => {
    const close = (event: Event) => {
      const box = ref.current
      if (!box?.open) return
      if (event instanceof globalThis.KeyboardEvent ? event.key === 'Escape' : !event.composedPath().includes(box)) box.open = false
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', close)
    return () => { document.removeEventListener('mousedown', close); document.removeEventListener('keydown', close) }
  }, [])

  return <details className="app-header-rates" ref={ref} onToggle={(event) => setOpen(event.currentTarget.open)}>
    <summary title={`Курс Национального банка РК на ${formatRuDate(rates[0].date)} — нажмите, чтобы пересчитать сумму`} aria-label="Курс НБРК и конвертер валют">
      {shown.map((rate) => <span key={rate.currency} className="app-header-rate"><small>1 {rate.currency}</small><b>{formatRuNumber(rate.rate)} ₸</b></span>)}
      <span className="app-header-rate-date">НБРК · {formatRuDate(rates[0].date)}</span>
    </summary>
    {open && <div className="app-header-converter" role="group" aria-label="Конвертер валют">
      <strong>Конвертер валют</strong>
      <input className="app-header-converter-amount" inputMode="decimal" value={amount} onChange={(event) => setAmount(event.target.value)} aria-label="Сумма" autoFocus />
      <div className="app-header-converter-row">
        <CurrencySelect value={from} codes={codes} rates={rates} label="Из валюты" onChange={setFrom} />
        <button type="button" className="cur-swap" onClick={() => { setFrom(to); setTo(from) }} title="Поменять местами" aria-label="Поменять валюты местами">⇄</button>
        <CurrencySelect value={to} codes={codes} rates={rates} label="В валюту" onChange={setTo} />
      </div>
      <output className="app-header-converter-result">{result === null ? '—' : `${amountText(result)} ${symbol(to)}`}</output>
      <small>{unit !== null && from !== to && `1 ${symbol(from)} = ${amountText(unit)} ${symbol(to)} · `}по курсу НБРК на {formatRuDate(rates[0].date)}</small>
    </div>}
  </details>
}
