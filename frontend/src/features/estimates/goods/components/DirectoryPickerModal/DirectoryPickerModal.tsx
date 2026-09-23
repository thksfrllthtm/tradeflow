import { KeyboardEvent, useEffect, useRef, useState } from 'react'
import { ProductDirectoryEntry } from '@/features/directories/products/types'
import { TNVEDDirectoryEntry } from '@/features/directories/tnved/types'
import { directoryStatusLabel, rateText } from '../../../shared/lib/tnvedRateText'
import { GlovesItemInput } from '../../../shared/types'
import '../../../shared/styles/pickerModal.css'
import './DirectoryPickerModal.css'
import '@/features/directories/verificationStatus.css'

export type DirectoryPickerEntry = TNVEDDirectoryEntry & { productDirectory?: ProductDirectoryEntry }
export type DirectoryPickerMode = 'products' | 'tnved'

type Props = {
  title: string
  photos: { photo: string; row: GlovesItemInput }[]
  multipleRows: boolean
  mode: DirectoryPickerMode
  query: string
  entries: DirectoryPickerEntry[]
  counts: Record<DirectoryPickerMode, number>
  loading: boolean
  onClose: () => void
  onModeChange: (mode: DirectoryPickerMode) => void
  onQueryChange: (value: string) => void
  onSearchNow: () => void
  onShowAll: () => void
  onPick: (entry: DirectoryPickerEntry) => void
  onPreview: (photos: string[]) => void
}

const modes: { value: DirectoryPickerMode; label: string; placeholder: string; hint: string }[] = [
  { value: 'products', label: 'Товары', placeholder: 'Название товара, синоним или код', hint: 'Код ТН ВЭД и ИТС подставятся из карточки товара.' },
  { value: 'tnved', label: 'Коды ТН ВЭД', placeholder: 'Код или описание', hint: 'Подставится только код и ставки; цену за единицу можно ввести вручную.' },
]

// Окно выбора товара или кода ТН ВЭД для одной или нескольких строк сметы.
// Поиск по мере ввода; ↑ ↓ выбирают карточку, Enter применяет, Esc закрывает.
export function DirectoryPickerModal({ title, photos, multipleRows, mode, query, entries, counts, loading, onClose, onModeChange, onQueryChange, onSearchNow, onShowAll, onPick, onPreview }: Props) {
  const [active, setActive] = useState(-1)
  const listRef = useRef<HTMLDivElement | null>(null)
  const current = modes.find((option) => option.value === mode)!
  const other = modes.find((option) => option.value !== mode)!

  useEffect(() => setActive(-1), [entries])
  useEffect(() => { listRef.current?.querySelectorAll('.dp-item')[active]?.scrollIntoView({ block: 'nearest' }) }, [active])
  useEffect(() => {
    // Esc при открытом просмотре фото закрывает только фото.
    const onKeyDown = (event: globalThis.KeyboardEvent) => { if (event.key === 'Escape' && !document.querySelector('.image-lightbox')) onClose() }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose])

  function onSearchKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown') { event.preventDefault(); setActive((index) => Math.min(index + 1, entries.length - 1)) }
    if (event.key === 'ArrowUp') { event.preventDefault(); setActive((index) => Math.max(index - 1, 0)) }
    if (event.key === 'Enter') { if (entries[active]) onPick(entries[active]); else onSearchNow() }
  }

  return <div className="picker-modal-backdrop" role="presentation" onMouseDown={onClose}>
    <div className="directory-picker-modal dp-modal" role="dialog" aria-modal="true" aria-labelledby="dp-title" onMouseDown={(event) => event.stopPropagation()}>
      <header className="dp-head">
        <div className="dp-title">
          <h3 id="dp-title" title={title}>{title}</h3>
          {photos.length > 0 && <div className="dp-photos">
            {photos.slice(0, 6).map(({ photo, row }, index) => <button type="button" key={`${row.id}-${index}`} onClick={() => onPreview([photo])} title={multipleRows ? `Строка ${row.sourceRow || 'новая'} — открыть фото` : 'Открыть фото'}><img src={photo} alt={`Фото строки ${row.sourceRow || 'новой'}`} /></button>)}
            {photos.length > 6 && <span>+{photos.length - 6}</span>}
          </div>}
        </div>
        <button type="button" className="dp-close" onClick={onClose} aria-label="Закрыть" title="Закрыть (Esc)">×</button>
      </header>

      <div className="dp-controls">
        <div className="dp-tabs" role="tablist">
          {modes.map((option) => <button type="button" role="tab" aria-selected={mode === option.value} key={option.value} className={mode === option.value ? 'active' : ''} onClick={() => onModeChange(option.value)}>{option.label}<b>{counts[option.value]}</b></button>)}
        </div>
        <input className="dp-search" autoFocus value={query} onChange={(event) => onQueryChange(event.target.value)} onKeyDown={onSearchKeyDown} placeholder={current.placeholder} aria-label="Поиск" />
      </div>
      <p className="dp-hint">{loading ? 'Ищем…' : current.hint}<span>↑ ↓ выбрать · Enter применить · Esc закрыть</span></p>

      <div className={loading ? 'dp-list dp-loading' : 'dp-list'} ref={listRef} role="listbox" aria-label={current.label}>
        {entries.length === 0 ? <div className="dp-empty">
          <p>{loading ? 'Ищем…' : query.trim() ? `Ничего не найдено по «${query.trim()}».` : 'Активных записей нет.'}</p>
          {!loading && counts[other.value] > 0 && <button type="button" className="secondary" onClick={() => onModeChange(other.value)}>Посмотреть в «{other.label}» ({counts[other.value]})</button>}
          {!loading && query.trim() !== '' && <button type="button" className="dp-link" onClick={onShowAll}>Сбросить поиск</button>}
        </div> : entries.map((entry, index) => {
          const zh = entry.productDirectory?.nameZh || entry.nameZh
          const itc = entry.productDirectory?.companyCoefficient || (entry.productDirectory ? 'не указан' : entry.companyCoefficient || 'не указан')
          return <button type="button" role="option" aria-selected={index === active} className={index === active ? 'dp-item active' : 'dp-item'} key={entry.id} onMouseEnter={() => setActive(index)} onClick={() => onPick(entry)}>
            <span className="dp-item-name">{entry.productDirectory?.name || entry.name}</span>
            <span className={`directory-status ${entry.verificationStatus}`}>{directoryStatusLabel(entry)}</span>
            <span className="dp-item-meta">
              <code>{entry.code}</code>
              {zh && <span>{zh}</span>}
              <span>ИТС {itc}</span>
              <span>Пошлина {rateText(entry.duty)}</span>
              <span>НДС {rateText(entry.vat)}</span>
            </span>
          </button>
        })}
      </div>
    </div>
  </div>
}
