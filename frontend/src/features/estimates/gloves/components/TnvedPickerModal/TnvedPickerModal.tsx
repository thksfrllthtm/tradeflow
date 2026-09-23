import { TNVEDDirectoryEntry } from '@/features/directories/tnved/types'
import { directoryStatusLabel, rateText } from '../../../shared/lib/tnvedRateText'
import { GlovesItemInput } from '../../../shared/types'
import '@/features/directories/verificationStatus.css'
import '../../../shared/styles/pickerModal.css'

type Props = {
  title: string
  photos: { photo: string; row: GlovesItemInput }[]
  multipleRows: boolean
  query: string
  entries: TNVEDDirectoryEntry[]
  loading: boolean
  onClose: () => void
  onQueryChange: (value: string) => void
  onSearchNow: () => void
  onShowAll: () => void
  onPick: (entry: TNVEDDirectoryEntry) => void
  onPreview: (photo: string) => void
}

// Окно выбора карточки ТН ВЭД (категория «Перчатки») для одной или нескольких строк.
export function TnvedPickerModal({ title, photos, multipleRows, query, entries, loading, onClose, onQueryChange, onSearchNow, onShowAll, onPick, onPreview }: Props) {
  return <div className="picker-modal-backdrop" role="presentation" onMouseDown={onClose}>
    <div className="directory-picker-modal" role="dialog" aria-modal="true" aria-labelledby="gloves-picker-title" onMouseDown={(event) => event.stopPropagation()}>
      <div className="modal-head">
        <div><p className="eyebrow">ТН ВЭД · ПЕРЧАТКИ</p><h3 id="gloves-picker-title" title={title}>{title}</h3></div>
        <button type="button" className="icon-button" onClick={onClose} aria-label="Закрыть">×</button>
      </div>
      {photos.length > 0 && <div className="picker-photo-strip">
        {photos.slice(0, 8).map(({ photo, row }, index) => <button type="button" className="picker-photo" key={`${row.id}-${index}`} onClick={() => onPreview(photo)}><img src={photo} alt={`Фото строки ${row.sourceRow || 'новой'}`} />{multipleRows && <small>Строка {row.sourceRow || 'новая'}</small>}</button>)}
        {photos.length > 8 && <span className="picker-photo-more">+{photos.length - 8}</span>}
      </div>}
      <div className="directory-search-bar">
        <input autoFocus value={query} onChange={(event) => onQueryChange(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') onSearchNow() }} placeholder="Поиск по коду, названию или синониму" />
        <button type="button" onClick={onSearchNow}>Найти</button>
      </div>
      <div className="directory-picker-list">
        {loading ? <p className="muted">Ищем…</p>
          : entries.length === 0 ? <div className="picker-empty"><p className="muted">Подходящих активных записей нет.</p>{query.trim() !== '' && <button type="button" className="secondary" onClick={onShowAll}>Показать все записи</button>}</div>
          : entries.map((entry) => <button type="button" className="directory-picker-item" key={entry.id} onClick={() => onPick(entry)}>
            <strong>{entry.name}</strong>
            <span>{entry.nameZh || '—'} · <code>{entry.code}</code></span>
            <small>ИТС: {entry.companyCoefficient || 'не указан'} · Пошлина: {rateText(entry.duty)} · НДС: {rateText(entry.vat)}</small>
            <span className={`directory-status ${entry.verificationStatus}`}>{directoryStatusLabel(entry)}</span>
          </button>)}
      </div>
      <p className="picker-footnote">Код, ИТС и ставки копируются в выбранные строки как снимок. Это предварительные данные операторского справочника, а не официальное подтверждение.</p>
    </div>
  </div>
}
