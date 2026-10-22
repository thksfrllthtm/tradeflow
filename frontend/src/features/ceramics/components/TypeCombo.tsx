import { KeyboardEvent, useEffect, useId, useRef, useState } from 'react'

// «Керамическая» есть у всех типов — в списке её не повторяем, остаётся то, чем типы отличаются.
export const shortType = (name: string) => {
  const rest = name.replace(/^керамическ\S*\s+/i, '')
  return rest === name ? name : rest.charAt(0).toUpperCase() + rest.slice(1)
}

type Props = {
  value: string
  names: string[]
  onChange: (name: string) => void
  // Варианты ИИ (доля 0–1): идут первыми, с меткой «ИИ 85%».
  hints?: Map<string, number>
  // Сколько марок у типа — серым справа.
  counts?: Map<string, number>
  // Пункт «без типа» первым (например, «Все типы» в фильтре); выбор даёт ''.
  emptyLabel?: string
  placeholder?: string
  label: string
}

type Option = { value: string; text: string }

// Тип товара: поле с поиском и свой список вместо <select> (тот рисует браузер, его не оформить).
// Пока ничего не введено — сначала варианты ИИ, потом все типы; при вводе — типы, где есть все слова
// запроса. Стрелки выбирают, Enter подставляет, Esc закрывает. Список — position: fixed у поля, иначе
// его обрезала бы прокрутка таблицы; при прокрутке страницы закрывается.
export function TypeCombo({ value, names, onChange, hints, counts, emptyLabel, placeholder = 'Выберите или начните вводить', label }: Props) {
  const id = useId()
  const input = useRef<HTMLInputElement>(null)
  const list = useRef<HTMLDivElement>(null)
  const [query, setQuery] = useState<string | null>(null)
  const [place, setPlace] = useState<{ left: number; top: number; width: number; height: number } | null>(null)
  const [active, setActive] = useState(-1)
  const words = (query ?? '').toLowerCase().split(/\s+/).filter(Boolean)
  const found = words.length
    ? names.filter((name) => words.every((word) => name.toLowerCase().includes(word)))
    : [...names].sort((a, b) => (hints?.get(b) ?? -1) - (hints?.get(a) ?? -1))
  const options: Option[] = [
    ...(emptyLabel && !words.length ? [{ value: '', text: emptyLabel }] : []),
    ...found.map((name) => ({ value: name, text: shortType(name) })),
  ]
  // Черта под вариантами ИИ — если за ними есть другие типы.
  const hinted = words.length ? 0 : found.filter((name) => hints?.has(name)).length
  const lastHint = hinted ? options.length - found.length + hinted - 1 : -1

  function open() {
    const rect = input.current?.getBoundingClientRect()
    if (rect) setPlace({ left: rect.left, top: rect.bottom + 4, width: Math.max(rect.width, 280), height: Math.max(160, Math.min(360, window.innerHeight - rect.bottom - 16)) })
  }
  function close() { setPlace(null); setQuery(null); setActive(-1) }
  function choose(next: string) { onChange(next); close(); input.current?.blur() }

  useEffect(() => {
    if (!place) return
    const scrolled = (event: Event) => { if (!list.current?.contains(event.target as Node)) close() }
    window.addEventListener('scroll', scrolled, true)
    window.addEventListener('resize', close)
    return () => { window.removeEventListener('scroll', scrolled, true); window.removeEventListener('resize', close) }
  }, [place])
  useEffect(() => { if (active >= 0) document.getElementById(`${id}-${active}`)?.scrollIntoView({ block: 'nearest' }) }, [active, id])

  function keys(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      if (!place) open()
      const step = event.key === 'ArrowDown' ? 1 : -1
      setActive((index) => options.length === 0 ? -1 : index < 0 ? (step > 0 ? 0 : options.length - 1) : (index + step + options.length) % options.length)
    } else if (event.key === 'Enter') {
      event.preventDefault()
      if (place && options[active]) choose(options[active].value)
    } else if (event.key === 'Escape' && place) {
      event.preventDefault()
      close()
    }
  }

  const shown = value ? shortType(value) : emptyLabel ?? ''
  return <div className={place ? 'cer-combo open' : 'cer-combo'}>
    <input ref={input} role="combobox" aria-expanded={Boolean(place)} aria-controls={`${id}-list`} aria-autocomplete="list" aria-label={label}
      aria-activedescendant={place && active >= 0 ? `${id}-${active}` : undefined} autoComplete="off" title={value || undefined}
      value={query ?? shown} placeholder={placeholder}
      onChange={(event) => { setQuery(event.target.value); setActive(0); if (!place) open() }}
      onFocus={(event) => { event.target.select(); open() }} onClick={() => { if (!place) open() }} onBlur={close} onKeyDown={keys} />
    <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><path d="M3 4.5l3 3 3-3" /></svg>
    {place && <div ref={list} id={`${id}-list`} className="cer-combo-list" role="listbox" aria-label={label}
      style={{ left: place.left, top: place.top, width: place.width, maxHeight: place.height }}>
      {options.length === 0 && <p className="cer-combo-empty">Нет типов со словами «{query?.trim()}»</p>}
      {options.map((option, index) => {
        const hint = hints?.get(option.value)
        const count = counts?.get(option.value)
        return <div key={option.value || '—'} id={`${id}-${index}`} role="option" aria-selected={option.value === value}
          className={`${index === active ? 'active' : ''}${index === lastHint && lastHint < options.length - 1 ? ' last-hint' : ''}${option.value ? '' : ' cer-combo-all'}`}
          onMouseDown={(event) => event.preventDefault()} onMouseEnter={() => setActive(index)} onClick={() => choose(option.value)}>
          <span>{option.text}</span>
          {hint !== undefined ? <small className="cer-combo-hint">ИИ {Math.round(hint * 100)}%</small> : count !== undefined && <small>{count}</small>}
        </div>
      })}
    </div>}
  </div>
}
