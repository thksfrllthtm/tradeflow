import { KeyboardEvent, useEffect, useRef, useState } from 'react'
import { NomenclatureCode } from '@/features/directories/nomenclature/nomenclatureApi'
import { nameParts } from '@/features/directories/nomenclature/nomenclatureDraft'
import { ProductDirectoryEntry } from '@/features/directories/products/types'
import { GlovesItemInput } from '../../../shared/types'
import '../../../shared/styles/pickerModal.css'
import './DirectoryPickerModal.css'
import '@/features/directories/verificationStatus.css'

// Товар — карточка справочника товаров (название, ИТС, код); код — строка номенклатуры ТН ВЭД
// ЕАЭС. nomenclature — ставки кода из номенклатуры (null — кода в номенклатуре нет).
export type DirectoryPickerEntry =
  | { kind: 'product'; key: string; code: string; product: ProductDirectoryEntry; nomenclature: NomenclatureCode | null }
  | { kind: 'code'; key: string; code: string; nomenclature: NomenclatureCode }
export type DirectoryPickerMode = 'products' | 'tnved'

type Props = {
  title: string
  photos: { photo: string; row: GlovesItemInput }[]
  multipleRows: boolean
  mode: DirectoryPickerMode
  query: string
  entries: DirectoryPickerEntry[]
  counts: Record<DirectoryPickerMode, number>
  // Кодов номенклатуры больше, чем показано.
  moreCodes: number
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
  { value: 'products', label: 'Товары', placeholder: 'Название товара, синоним или код', hint: 'Название, код и ИТС — из карточки товара; пошлина и НДС — из номенклатуры ТН ВЭД ЕАЭС.' },
  { value: 'tnved', label: 'Коды ТН ВЭД', placeholder: 'Начало кода (4015 19) или слова из наименования', hint: 'Номенклатура ТН ВЭД ЕАЭС: подставятся код, пошлина ЕТТ и НДС; название строки останется из Excel.' },
]

// Окно выбора товара или кода ТН ВЭД для одной или нескольких строк сметы.
// Поиск по мере ввода; ↑ ↓ выбирают карточку, Enter применяет, Esc закрывает.
export function DirectoryPickerModal({ title, photos, multipleRows, mode, query, entries, counts, moreCodes, loading, onClose, onModeChange, onQueryChange, onSearchNow, onShowAll, onPick, onPreview }: Props) {
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
          const official = entry.nomenclature
          const product = entry.kind === 'product' ? entry.product : null
          const parts = official ? nameParts(official.name) : []
          return <button type="button" role="option" aria-selected={index === active} className={index === active ? 'dp-item active' : 'dp-item'} key={entry.key} onMouseEnter={() => setActive(index)} onClick={() => onPick(entry)}>
            <span className="dp-item-name">{product ? product.name : parts[parts.length - 1]}</span>
            {!official && <span className="directory-status needs_review">Нет в номенклатуре</span>}
            <span className="dp-item-meta">
              <code>{entry.code}</code>
              {product?.nameZh && <span>{product.nameZh}</span>}
              {!product && parts.length > 1 && <span className="dp-item-context">{parts.slice(0, -1).join(' ')}</span>}
              {product && <span>ИТС {product.companyCoefficient || 'не указан'}</span>}
              {official && <span>Пошлина ЕТТ {official.tariffText || 'не указана'}</span>}
              {official && <span>НДС {official.vatPercent || '16'}%</span>}
            </span>
          </button>
        })}
        {mode === 'tnved' && moreCodes > 0 && entries.length > 0 && <p className="dp-more">Показаны первые {entries.length} из {counts.tnved}. Уточните поиск — начало кода или слова из наименования.</p>}
      </div>
    </div>
  </div>
}
