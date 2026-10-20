import { KeyboardEvent, useEffect, useRef, useState } from 'react'
import { ProductDirectoryEntry } from '@/features/directories/products/types'
import { GoodsItemInput } from '@/shared/types/goods'
import './pickerModal.css'
import './DirectoryPickerModal.css'
import '@/shared/styles/verification-status.css'
import { NomenclatureCode } from '@/shared/tnved/nomenclature'
import { formatCode, nameParts } from '@/shared/tnved/codes'
import { lookupNomenclature } from '../../lib/nomenclatureRates'

// Товар — карточка справочника товаров (название, ИТС, код); код — строка номенклатуры ТН ВЭД
// ЕАЭС. nomenclature — ставки кода из номенклатуры (null — кода в номенклатуре нет).
export type DirectoryPickerEntry =
  | { kind: 'product'; key: string; code: string; product: ProductDirectoryEntry; nomenclature: NomenclatureCode | null }
  | { kind: 'code'; key: string; code: string; nomenclature: NomenclatureCode }
export type DirectoryPickerMode = 'products' | 'tnved'

type Props = {
  title: string
  // Наименование товара строки — одной строкой под заголовком.
  subtitle: string
  // «К строке 6» / «К 3 выбранным строкам».
  target: string
  // Код, который сейчас стоит в строке (пусто — кода нет или у строк разные коды).
  currentCode: string
  photos: { photo: string; row: GoodsItemInput }[]
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
  { value: 'tnved', label: 'Номенклатура ТН ВЭД', placeholder: 'Начало кода (4015 19) или слова из наименования', hint: 'Подставятся код, пошлина ЕТТ и НДС; название строки останется из Excel.' },
  { value: 'products', label: 'Справочник товаров', placeholder: 'Название товара, синоним или код', hint: 'Название, код и ИТС — из карточки товара; пошлина и НДС — из номенклатуры.' },
]

// Окно выбора товара или кода ТН ВЭД для одной или нескольких строк сметы.
// Поиск по мере ввода; ↑ ↓ выбирают строку, Enter или «Применить» применяет, двойной щелчок — сразу, Esc закрывает.
export function DirectoryPickerModal({ title, subtitle, target, currentCode, photos, multipleRows, mode, query, entries, counts, moreCodes, loading, onClose, onModeChange, onQueryChange, onSearchNow, onShowAll, onPick, onPreview }: Props) {
  const [active, setActive] = useState(-1)
  const [current, setCurrent] = useState<NomenclatureCode | null>(null)
  const listRef = useRef<HTMLDivElement | null>(null)
  const searchRef = useRef<HTMLInputElement | null>(null)
  const modeInfo = modes.find((option) => option.value === mode)!
  const other = modes.find((option) => option.value !== mode)!
  const picked = entries[active]

  useEffect(() => setActive(-1), [entries])
  useEffect(() => { listRef.current?.querySelectorAll('.dp-item')[active]?.scrollIntoView({ block: 'nearest' }) }, [active])
  useEffect(() => {
    // Esc при открытом просмотре фото закрывает только фото.
    const onKeyDown = (event: globalThis.KeyboardEvent) => { if (event.key === 'Escape' && !document.querySelector('.image-lightbox')) onClose() }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose])
  // Наименование и ставки кода строки — для блока «Сейчас в строке».
  useEffect(() => {
    if (!currentCode) return
    let active = true
    void lookupNomenclature([currentCode]).then((found) => { if (active) setCurrent(found?.byCode.get(currentCode) ?? null) }).catch(() => undefined)
    return () => { active = false }
  }, [currentCode])
  // Курсор в конце заполненного поиска, а не выделение всего текста.
  useEffect(() => { const input = searchRef.current; if (input) input.setSelectionRange(input.value.length, input.value.length) }, [])

  function onSearchKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown') { event.preventDefault(); setActive((index) => Math.min(index + 1, entries.length - 1)) }
    if (event.key === 'ArrowUp') { event.preventDefault(); setActive((index) => Math.max(index - 1, 0)) }
    if (event.key === 'Enter') { if (picked) onPick(picked); else onSearchNow() }
  }

  const currentParts = current ? nameParts(current.name) : []
  return <div className="picker-modal-backdrop" role="presentation" onMouseDown={onClose}>
    <div className="directory-picker-modal dp-modal" role="dialog" aria-modal="true" aria-labelledby="dp-title" onMouseDown={(event) => event.stopPropagation()}>
      <header className="dp-head">
        <div className="dp-title">
          <h3 id="dp-title">{title}</h3>
          {subtitle && <p title={subtitle}>{subtitle}</p>}
        </div>
        {photos.length > 0 && <div className="dp-photos">
          {photos.slice(0, 4).map(({ photo, row }, index) => <button type="button" key={`${row.id}-${index}`} onClick={() => onPreview([photo])} title={multipleRows ? `Строка ${row.sourceRow || 'новая'} — открыть фото` : 'Открыть фото'}><img src={photo} alt={`Фото строки ${row.sourceRow || 'новой'}`} /></button>)}
          {photos.length > 4 && <span>+{photos.length - 4}</span>}
        </div>}
        <button type="button" className="dp-close" onClick={onClose} aria-label="Закрыть" title="Закрыть (Esc)">
          <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round"><path d="M4 4l8 8M12 4l-8 8" /></svg>
        </button>
      </header>

      <div className="dp-controls">
        <div className="dp-tabs" role="tablist">
          {modes.map((option) => <button type="button" role="tab" aria-selected={mode === option.value} key={option.value} className={mode === option.value ? 'active' : ''} onClick={() => onModeChange(option.value)}>{option.label}</button>)}
        </div>
        <label className="dp-search-wrap">
          <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"><circle cx="7" cy="7" r="4.6" /><path d="M10.4 10.4L14 14" /></svg>
          <input ref={searchRef} className="dp-search" autoFocus value={query} onChange={(event) => onQueryChange(event.target.value)} onKeyDown={onSearchKeyDown} placeholder={modeInfo.placeholder} aria-label="Поиск" aria-controls="dp-list" />
          {loading && <span className="dp-spinner" aria-label="Ищем" />}
        </label>
        <p className="dp-hint">{modeInfo.hint}</p>
      </div>

      {currentCode && <div className="dp-current">
        <span className="dp-current-label">Сейчас в строке</span>
        <code>{formatCode(currentCode)}</code>
        <span className="dp-current-name" title={current?.name}>{currentParts[currentParts.length - 1] ?? ''}</span>
        {current && <span className="dp-current-rates">Пошлина {current.tariffText || '—'} · НДС {current.vatPercent || '16'}%</span>}
      </div>}

      <div className="dp-table-head" aria-hidden="true"><span>Код</span><span>Наименование</span><span>Пошлина</span><span>НДС</span></div>
      <div id="dp-list" className={loading ? 'dp-list dp-loading' : 'dp-list'} ref={listRef} role="listbox" aria-label={modeInfo.label}>
        {entries.length === 0 ? <div className="dp-empty">
          <p>{loading ? 'Ищем…' : query.trim() ? `Ничего не найдено по «${query.trim()}».` : 'Активных записей нет.'}</p>
          {!loading && counts[other.value] > 0 && <button type="button" className="secondary" onClick={() => onModeChange(other.value)}>Посмотреть в «{other.label}»</button>}
          {!loading && query.trim() !== '' && <button type="button" className="dp-link" onClick={() => onShowAll()}>Сбросить поиск</button>}
        </div> : entries.map((entry, index) => {
          const official = entry.nomenclature
          const product = entry.kind === 'product' ? entry.product : null
          const parts = official ? nameParts(official.name) : []
          const path = product ? [product.nameZh, `ИТС ${product.companyCoefficient || 'не указан'}`].filter(Boolean).join(' · ') : parts.slice(0, -1).join(' › ')
          return <button type="button" role="option" tabIndex={-1} aria-selected={index === active} className={index === active ? 'dp-item active' : 'dp-item'} key={entry.key}
            onClick={() => { setActive(index); searchRef.current?.focus() }} onDoubleClick={() => onPick(entry)}>
            <code>{formatCode(entry.code)}</code>
            <span className="dp-item-text">
              <span className="dp-item-name">{product ? product.name : parts[parts.length - 1]}</span>
              {path && <span className="dp-item-context" title={path}>{path}</span>}
            </span>
            {official
              ? <><span className="dp-item-rate">{official.tariffText || '—'}</span><span className="dp-item-rate">{official.vatPercent || '16'}%</span></>
              : <span className="dp-item-missing"><span className="directory-status needs_review">Нет в номенклатуре</span></span>}
          </button>
        })}
        {mode === 'tnved' && moreCodes > 0 && entries.length > 0 && <p className="dp-more">Показаны первые {entries.length} из {counts.tnved}. Уточните поиск — начало кода или слова из наименования.</p>}
      </div>

      <footer className="dp-foot">
        <span className="dp-target">{target}</span>
        <span className="dp-keys"><kbd>↑</kbd><kbd>↓</kbd> выбрать · <kbd>Enter</kbd> применить</span>
        <button type="button" className="dp-cancel" onClick={onClose}>Отмена</button>
        <button type="button" className="dp-apply" onClick={() => picked && onPick(picked)} disabled={!picked}>{picked ? `Применить ${formatCode(picked.code)}` : 'Применить'}</button>
      </footer>
    </div>
  </div>
}
