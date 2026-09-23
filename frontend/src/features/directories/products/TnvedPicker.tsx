import { KeyboardEvent, useEffect, useRef, useState } from 'react'
import { TNVEDDirectoryEntry } from '../tnved/types'
import { rateLabel } from '../tnved/tnvedLabels'

// Выбор кода ТН ВЭД с поиском по коду, описанию и синонимам (вместо длинного выпадающего списка).
// ↑ ↓ — выбор, Enter — применить, Esc — закрыть список.
export function TnvedPicker({ entries, value, onChange }: { entries: TNVEDDirectoryEntry[]; value?: number; onChange: (id: number | undefined) => void }) {
  const selected = entries.find((entry) => entry.id === value)
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const listRef = useRef<HTMLUListElement | null>(null)
  const needle = query.trim().toLowerCase()
  const matches = needle ? entries.filter((entry) => [entry.code, entry.name, entry.nameZh, ...entry.synonymsRu, ...entry.synonymsZh, ...entry.synonymsEn].join(' ').toLowerCase().includes(needle)) : entries

  useEffect(() => setActive(0), [query])
  useEffect(() => { listRef.current?.children[active]?.scrollIntoView({ block: 'nearest' }) }, [active])

  function pick(entry: TNVEDDirectoryEntry) {
    onChange(entry.id)
    setOpen(false)
    setQuery('')
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown') { event.preventDefault(); setOpen(true); setActive((index) => Math.min(index + 1, matches.length - 1)) }
    if (event.key === 'ArrowUp') { event.preventDefault(); setActive((index) => Math.max(index - 1, 0)) }
    if (event.key === 'Enter' && open) { event.preventDefault(); if (matches[active]) pick(matches[active]) }
    if (event.key === 'Escape' && open) { event.preventDefault(); setOpen(false); setQuery('') }
  }

  return <div className="tnved-picker">
    <input
      value={open ? query : selected ? `${selected.code} · ${selected.name}` : ''}
      onChange={(event) => { setQuery(event.target.value); setOpen(true) }}
      onFocus={() => setOpen(true)}
      onBlur={() => { setOpen(false); setQuery('') }}
      onKeyDown={onKeyDown}
      placeholder={selected ? 'Поиск другого кода…' : 'Начните вводить код или описание'}
      role="combobox"
      aria-expanded={open}
      aria-label="Код ТН ВЭД"
    />
    {open && <ul className="tnved-picker-list" ref={listRef} role="listbox">
      {matches.length === 0 ? <li className="tnved-picker-empty">Ничего не найдено. Добавьте код в справочник ТН ВЭД.</li>
        : matches.map((entry, index) => <li key={entry.id} role="option" aria-selected={entry.id === value} className={[index === active ? 'active' : '', entry.id === value ? 'selected' : ''].join(' ')} onMouseDown={(event) => { event.preventDefault(); pick(entry) }} onMouseEnter={() => setActive(index)}>
          <code>{entry.code}</code>
          <span>{entry.name || 'Без описания'}</span>
          <small>{rateLabel(entry.duty, 'Пошлина')} · НДС {rateLabel(entry.vat, 'НДС')}</small>
        </li>)}
    </ul>}
    {selected && !open && <div className="tnved-picker-card">
      <code>{selected.code}</code>
      <span>Пошлина {rateLabel(selected.duty, 'Пошлина')}</span>
      <span>НДС {rateLabel(selected.vat, 'НДС')}</span>
      {selected.antiDumpingPercent && <span>Антидемпинг {selected.antiDumpingPercent}%</span>}
      {selected.verificationStatus !== 'operator_verified' && <span className="warn">код требует проверки</span>}
    </div>}
  </div>
}
