import { KeyboardEvent as ReactKeyboardEvent, useEffect, useMemo, useRef, useState } from 'react'
import './AntiDumpingChooser.css'

// Вариант ставки антидемпинга с сервера (реестр ЕЭК): страна, производитель или вид товара, ставка.
export type AntiDumpingOption = { key: string; measure: string; shortName: string; country: string; countryName: string; label: string; percent: string; others: boolean }
// Антидемпинг строки в снимке результата расчёта.
export type AntiDumpingInfo = { status?: string; number?: string; percent?: string; text?: string; selected?: string; options?: AntiDumpingOption[]; documentUrl?: string }

export const NO_ANTIDUMPING = 'none'

const percent = (value?: string) => value ? `${value.replace('.', ',')}%` : ''

// on — начислен, off — не начисляется, ask — сотруднику нужно выбрать или проверить вариант.
export function antiDumpingTone(info?: AntiDumpingInfo): 'on' | 'off' | 'ask' | '' {
  if (!info?.options?.length) return ''
  const status = info.status ?? ''
  return status === 'applied' || status === 'chosen' ? 'on' : status === 'declined' || status === 'other-country' ? 'off' : 'ask'
}

type Props = { info: AntiDumpingInfo; choice?: string; disabled: boolean; onChoose: (key: string) => void }

// Кнопка под пошлиной строки: все варианты антидемпинга по мерам, под которые попадает код —
// страна, производитель, ставка; «Не начислять» и «Как по стране». Выбор пересчитывает строку.
export function AntiDumpingChooser({ info, choice, disabled, onChoose }: Props) {
  const [anchor, setAnchor] = useState<DOMRect | null>(null)
  const [query, setQuery] = useState('')
  const menuRef = useRef<HTMLDivElement>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const options = info.options ?? []

  useEffect(() => {
    if (!anchor) return
    const close = (event: Event) => {
      const target = event.target as Node
      if (menuRef.current?.contains(target) || buttonRef.current?.contains(target)) return
      setAnchor(null)
    }
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') { setAnchor(null); buttonRef.current?.focus() } }
    document.addEventListener('mousedown', close)
    document.addEventListener('scroll', close, true)
    window.addEventListener('resize', close)
    document.addEventListener('keydown', escape)
    return () => { document.removeEventListener('mousedown', close); document.removeEventListener('scroll', close, true); window.removeEventListener('resize', close); document.removeEventListener('keydown', escape) }
  }, [anchor])

  const groups = useMemo(() => {
    const words = query.trim().toLowerCase()
    const byMeasure = new Map<string, AntiDumpingOption[]>()
    for (const option of options) {
      if (words && !`${option.countryName} ${option.label} ${option.measure} ${option.shortName}`.toLowerCase().includes(words)) continue
      byMeasure.set(option.measure, [...(byMeasure.get(option.measure) ?? []), option])
    }
    return [...byMeasure.values()]
  }, [options, query])

  // Открытое меню: фокус — на выбранный вариант (или первый), если нет поиска.
  useEffect(() => {
    if (!anchor || options.length > 8) return
    const menu = menuRef.current
    ;(menu?.querySelector<HTMLButtonElement>('button[aria-pressed="true"]') ?? menu?.querySelector<HTMLButtonElement>('button'))?.focus()
  }, [anchor, options.length])

  if (options.length === 0) return null
  const status = info.status ?? ''
  // Короткая подпись не расширяет колонку пошлины; полная — в подсказке и для экранного диктора.
  const [label, fullLabel] = status === 'applied' || status === 'chosen' ? [`АД ${percent(info.percent)}`, `антидемпинг ${percent(info.percent)}`]
    : status === 'declined' ? ['АД: нет', 'антидемпинг не начисляется']
      : status === 'other-country' ? ['АД: не для страны', 'антидемпинг не для этой страны']
        : status === 'stale' ? ['АД: выбрать заново', 'антидемпинг: выбрать ставку заново']
          : status === 'possible' ? ['АД: возможно', 'антидемпинг возможен — проверьте']
            : ['АД: выбрать', 'антидемпинг: выбрать ставку']
  const tone = antiDumpingTone(info)
  const selected = choice || info.selected || ''

  function pick(key: string) {
    setAnchor(null)
    setQuery('')
    buttonRef.current?.focus()
    if (key !== (choice ?? '')) onChoose(key)
  }

  // Стрелки вверх и вниз — по вариантам и кнопкам меню.
  function moveFocus(event: ReactKeyboardEvent<HTMLDivElement>) {
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
    const buttons = Array.from(menuRef.current?.querySelectorAll<HTMLButtonElement>('button') ?? [])
    if (buttons.length === 0) return
    event.preventDefault()
    const at = buttons.indexOf(document.activeElement as HTMLButtonElement)
    const next = event.key === 'ArrowDown' ? (at + 1) % buttons.length : (at <= 0 ? buttons.length - 1 : at - 1)
    buttons[next].focus()
  }

  const width = 380
  const left = anchor ? Math.max(8, Math.min(anchor.left, window.innerWidth - width - 8)) : 0
  const below = anchor ? window.innerHeight - anchor.bottom > 320 : true
  return <>
    <button ref={buttonRef} type="button" className={`ad-choose ${tone}`} disabled={disabled} title={info.text || fullLabel} aria-label={fullLabel} aria-haspopup="dialog" aria-expanded={Boolean(anchor)}
      onClick={(event) => setAnchor(anchor ? null : event.currentTarget.getBoundingClientRect())}>{label}<svg width="9" height="9" viewBox="0 0 10 10" aria-hidden="true"><path d="M2.5 4l2.5 2.5L7.5 4" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" /></svg></button>
    {anchor && <div ref={menuRef} className="ad-menu" role="dialog" aria-label="Варианты антидемпинга" onKeyDown={moveFocus}
      style={{ left, width, ...(below ? { top: anchor.bottom + 4 } : { bottom: window.innerHeight - anchor.top + 4 }) }}>
      {info.text && <p className="ad-menu-text">{info.text}</p>}
      {options.length > 8 && <input className="ad-menu-search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Страна или производитель" aria-label="Найти вариант" autoFocus />}
      <div className="ad-menu-list">
        {groups.map((group) => <section key={group[0].measure}>
          <h4>{group[0].measure} «{group[0].shortName}»</h4>
          {group.map((option) => <button key={option.key} type="button" aria-pressed={selected === option.key} className={selected === option.key ? 'active' : undefined} onClick={() => pick(option.key)} title={option.label}>
            <span><em>{option.countryName}</em>{option.label}</span><b>{percent(option.percent)}</b>
          </button>)}
        </section>)}
        {groups.length === 0 && <p className="ad-menu-empty">Ничего не найдено</p>}
      </div>
      <footer>
        <button type="button" aria-pressed={selected === NO_ANTIDUMPING} className={selected === NO_ANTIDUMPING ? 'active' : undefined} onClick={() => pick(NO_ANTIDUMPING)}>Не начислять</button>
        {choice && <button type="button" onClick={() => pick('')} title="Ставку выберет программа по полю «Страна» сметы">Как по стране</button>}
      </footer>
    </div>}
  </>
}
