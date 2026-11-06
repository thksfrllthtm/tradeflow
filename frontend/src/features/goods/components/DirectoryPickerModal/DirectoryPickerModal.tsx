import { KeyboardEvent, useEffect, useRef, useState } from 'react'
import { ProductDirectoryEntry } from '@/features/directories/products/types'
import { GoodsItemInput } from '@/shared/types/goods'
import { photosOf } from '@/shared/lib/itemPhotos'
import { SearchIcon } from '@/shared/components/icons'
import { Modal } from '@/shared/components/Modal/Modal'
import '@/shared/styles/list-page.css'
import '@/shared/styles/verification-status.css'
import '../../goods.css'
import './DirectoryPickerModal.css'
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
  // Строки сметы и id строк, для которых выбирается товар или код.
  items: GoodsItemInput[]
  targetIds: string[]
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
export function DirectoryPickerModal({ items, targetIds, mode, query, entries, counts, moreCodes, loading, onClose, onModeChange, onQueryChange, onSearchNow, onShowAll, onPick, onPreview }: Props) {
  const rows = items.filter((item) => targetIds.includes(item.id ?? ''))
  const multipleRows = rows.length > 1
  // № — как в таблице.
  const rowNumber = rows.length === 1 ? items.indexOf(rows[0]) + 1 : 0
  const title = rows.length === 1 ? `Код для строки ${rowNumber}` : multipleRows ? `Код для ${rows.length} выбранных строк` : 'Выбор кода ТН ВЭД'
  // Наименование товара строки — одной строкой под заголовком.
  const subtitle = rows.length === 1 ? rows[0].name || rows[0].supplierName || '' : ''
  const target = rows.length === 1 ? `К строке ${rowNumber}` : `К ${rows.length} выбранным строкам`
  // Общий код выбранных строк; у строк разные коды — блока «Сейчас в строке» нет.
  const codes = [...new Set(rows.map((row) => row.code ?? ''))]
  const currentCode = codes.length === 1 ? codes[0] : ''
  const photos = rows.flatMap((row) => photosOf(row).map((photo) => ({ photo, row })))
  const [active, setActive] = useState(-1)
  const [current, setCurrent] = useState<NomenclatureCode | null>(null)
  const listRef = useRef<HTMLDivElement | null>(null)
  const searchRef = useRef<HTMLInputElement | null>(null)
  const modeInfo = modes.find((option) => option.value === mode)!
  const other = modes.find((option) => option.value !== mode)!
  const picked = entries[active]

  useEffect(() => setActive(-1), [entries])
  useEffect(() => { listRef.current?.querySelectorAll('.dp-item')[active]?.scrollIntoView({ block: 'nearest' }) }, [active])
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
  const close = onClose
  const photoThumbs = photos.length > 0 && <div className="dp-photos">
    {photos.slice(0, 4).map(({ photo, row }, index) => <button type="button" key={`${row.id}-${index}`} onClick={() => onPreview([photo])} title={multipleRows ? `Строка ${row.sourceRow || 'новая'} — открыть фото` : 'Открыть фото'}><img src={photo} alt={`Фото строки ${row.sourceRow || 'новой'}`} /></button>)}
    {photos.length > 4 && <span>+{photos.length - 4}</span>}
  </div>
  const footer = <>
    <span className="dp-target">{target}</span>
    <span className="dp-keys"><kbd>↑</kbd><kbd>↓</kbd> выбрать · <kbd>Enter</kbd> применить</span>
    <button type="button" className="secondary" onClick={onClose}>Отмена</button>
    <button type="button" onClick={() => picked && onPick(picked)} disabled={!picked}>{picked ? `Применить ${formatCode(picked.code)}` : 'Применить'}</button>
  </>
  return <Modal title={title} subtitle={subtitle && <span title={subtitle}>{subtitle}</span>} headExtra={photoThumbs} footer={footer} onClose={close} width={880} className="dp-modal">
    <div className="dp-controls">
      <div className="dir-seg dp-tabs" role="tablist">
        {modes.map((option) => <button type="button" role="tab" aria-selected={mode === option.value} key={option.value} className={mode === option.value ? 'active' : ''} onClick={() => onModeChange(option.value)}>{option.label}</button>)}
      </div>
      <label className="dir-search dp-search">
        <SearchIcon />
        <input ref={searchRef} autoFocus value={query} onChange={(event) => onQueryChange(event.target.value)} onKeyDown={onSearchKeyDown} placeholder={modeInfo.placeholder} aria-label="Поиск" aria-controls="dp-list" />
        {loading && <span className="goods-spinner dp-spinner" aria-label="Ищем" />}
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
      {entries.length === 0 ? <div className="dir-empty dp-empty">
        <p>{loading ? 'Ищем…' : query.trim() ? `Ничего не найдено по «${query.trim()}».` : 'Активных записей нет.'}</p>
        {!loading && counts[other.value] > 0 && <button type="button" className="secondary" onClick={() => onModeChange(other.value)}>Посмотреть в «{other.label}»</button>}
        {!loading && query.trim() !== '' && <button type="button" className="button-link" onClick={() => onShowAll()}>Сбросить поиск</button>}
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
  </Modal>
}
