import { KeyboardEvent, useEffect, useRef, useState } from 'react'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { errorMessage } from '@/shared/lib/errorMessage'
import { formatCode, nameParts } from '@/shared/tnved/codes'
import { NomenclatureCode } from '@/shared/tnved/nomenclature'
import { nomenclatureApi } from '../nomenclature/nomenclatureApi'

const shortName = (item: NomenclatureCode) => nameParts(item.name).slice(-1)[0] || 'Без наименования'
const rateList = (item: NomenclatureCode) => [`ЕТТ ${item.tariffText || 'не указана'}`, item.vatPercent && `НДС ${item.vatPercent}%`, item.kz?.active && `РК ${item.kz.rateText}`].filter((rate): rate is string => Boolean(rate))
const rates = (item: NomenclatureCode) => rateList(item).join(' · ')

// Код ТН ВЭД товара — из номенклатуры ТН ВЭД ЕАЭС: поиск по началу кода или словам наименования.
// ↑ ↓ — выбор, Enter — применить, Esc — закрыть список. Под полем — выбранный код со ставками.
export function TnvedPicker({ value, onChange, disabled = false }: { value: string; onChange: (code: string) => void; disabled?: boolean }) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const [matches, setMatches] = useState<NomenclatureCode[]>([])
  // undefined — код ещё ищется в номенклатуре, null — его там нет.
  const [selected, setSelected] = useState<NomenclatureCode | null | undefined>(undefined)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const listRef = useRef<HTMLUListElement | null>(null)
  const search = useDebouncedValue(query.trim(), 250)

  // Выбранный код: наименование и ставки из номенклатуры; кода в ней нет — selected пустой.
  useEffect(() => {
    if (!value) { setSelected(null); return }
    let active = true
    setSelected(undefined)
    nomenclatureApi.lookup([value]).then((found) => { if (active) setSelected(found.items[0] ?? null) }).catch(() => { if (active) setSelected(null) })
    return () => { active = false }
  }, [value])

  useEffect(() => {
    if (!open || !search) { setMatches([]); return }
    let active = true
    setLoading(true); setError('')
    nomenclatureApi.list({ q: search, offset: 0, limit: 30 })
      .then((page) => { if (active) { setMatches(page.items); setActive(0) } })
      .catch((reason) => { if (active) setError(errorMessage(reason)) })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [search, open])

  useEffect(() => { listRef.current?.children[active]?.scrollIntoView({ block: 'nearest' }) }, [active])

  function pick(item: NomenclatureCode) {
    onChange(item.code)
    setSelected(item)
    setOpen(false)
    setQuery('')
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown') { event.preventDefault(); setOpen(true); setActive((index) => Math.min(index + 1, matches.length - 1)) }
    if (event.key === 'ArrowUp') { event.preventDefault(); setActive((index) => Math.max(index - 1, 0)) }
    if (event.key === 'Enter' && open) { event.preventDefault(); if (matches[active]) pick(matches[active]) }
    if (event.key === 'Escape' && open) { event.preventDefault(); setOpen(false); setQuery('') }
  }

  const empty = loading ? 'Ищем…' : error || (search ? 'Ничего не найдено в номенклатуре ТН ВЭД ЕАЭС.' : 'Начните вводить код (6116 10) или слова наименования.')
  return <div className="tnved-picker">
    <input
      value={open ? query : ''}
      onChange={(event) => { setQuery(event.target.value); setOpen(true) }}
      onFocus={() => setOpen(true)}
      onBlur={() => { setOpen(false); setQuery('') }}
      onKeyDown={onKeyDown}
      placeholder={value ? 'Поиск другого кода…' : 'Начните вводить код или наименование'}
      role="combobox"
      aria-expanded={open}
      aria-label="Код ТН ВЭД"
      disabled={disabled}
    />
    {open && <ul className="tnved-picker-list" ref={listRef} role="listbox">
      {matches.length === 0 ? <li className="tnved-picker-empty">{empty}</li>
        : matches.map((item, index) => <li key={item.code} role="option" aria-selected={item.code === value} className={[index === active ? 'active' : '', item.code === value ? 'selected' : ''].join(' ')} onMouseDown={(event) => { event.preventDefault(); pick(item) }} onMouseEnter={() => setActive(index)}>
          <code>{formatCode(item.code)}</code>
          <span title={nameParts(item.name).join(' ')}>{shortName(item)}</span>
          <small>{rates(item)}</small>
        </li>)}
    </ul>}
    {value && !open && <div className="tnved-picker-card">
      <code>{formatCode(value)}</code>
      {selected === undefined ? null : selected ? <>
        <span className="tnved-picker-name" title={nameParts(selected.name).join(' ')}>{shortName(selected)}</span>
        <span className="tnved-picker-rates">
          {rateList(selected).map((rate) => <i key={rate}>{rate}</i>)}
          {selected.antiDumping?.some((measure) => measure.exact && !measure.suspended) && <i className="warn">есть антидемпинг</i>}
        </span>
      </> : <span className="warn">Кода нет в номенклатуре ТН ВЭД ЕАЭС — проверьте код</span>}
    </div>}
  </div>
}
