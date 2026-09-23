import { ChangeEvent } from 'react'
import { GlovesCalculationResult, GlovesItemInput } from '../../../shared/types'
import { formatDisplayNumber } from '@/shared/lib/formatDisplayNumber'
import '../../../shared/styles/calculatorCommon.css'
import './ReviewRow.css'

export type ReviewRowProps = {
  item: GlovesItemInput
  rowNumber: number
  resultItem?: GlovesCalculationResult['items'][number]
  selected: boolean
  onSelected: (checked: boolean) => void
  onOpenPicker: () => void
  onClearDirectory: () => void
  onChange: (id: string, field: keyof GlovesItemInput, value: string | boolean | undefined) => void
  onRemove: (id: string) => void
  onPreview: (value: string) => void
  onUpload: (id: string, event: ChangeEvent<HTMLInputElement>) => void
  onResetNet: () => void
}

function parseDecimal(value: string): number | null {
  const normalized = String(value ?? '').trim().replace(',', '.')
  if (!normalized || !/^[+-]?(\d+(\.\d*)?|\.\d+)$/.test(normalized)) return null
  const parsed = Number(normalized)
  return Number.isFinite(parsed) ? parsed : null
}

function formatWeight(value: number): string {
  return formatDisplayNumber(value.toFixed(3).replace(/\.?0+$/, ''))
}

function autoNetFor(item: GlovesItemInput): number | null {
  const gross = parseDecimal(item.grossKg)
  const places = parseDecimal(item.places)
  const packageWeight = parseDecimal(item.packageWeightKg)
  if (gross === null || places === null || packageWeight === null) return null
  return gross - places * packageWeight
}

function photosOf(item: GlovesItemInput): string[] {
  return item.photoUrls?.length ? item.photoUrls : item.photoUrl ? [item.photoUrl] : []
}

function netHint(item: GlovesItemInput): { className: string; text: string } {
  const gross = parseDecimal(item.grossKg)
  const net = parseDecimal(item.netKg)
  const auto = autoNetFor(item)
  const excel = item.netSource === 'excel'
  const calculated = item.netSource === 'calculated'
  const manual = !excel && !calculated && item.netKg.trim() !== ''

  if (excel || manual || calculated) {
    if (net === null) return { className: 'net-bad', text: 'Нетто — не число' }
    if (gross === null) return { className: 'net-warn', text: 'Нет брутто для проверки' }
    const ok = net > 1e-9 && net <= gross + 1e-9
    const label = excel ? 'из Excel' : calculated ? 'рассчитано' : 'вручную'
    const compare = auto !== null ? ` · авто было бы ${formatWeight(auto)} кг` : ''
    return ok
      ? { className: 'net-ok', text: `✓ ${label} · ${formatWeight(net)} кг${compare}` }
      : { className: 'net-bad', text: `✕ нетто должно быть больше 0 и не больше брутто${compare}` }
  }

  if (auto === null) return { className: 'net-warn', text: 'Укажите брутто и места для авто-нетто' }
  if (gross === null) return { className: 'net-warn', text: `Авто: ${formatWeight(auto)} кг` }
  return auto > 1e-9 && auto <= gross + 1e-9
    ? { className: 'net-ok', text: `✓ авто: ${formatWeight(auto)} кг` }
    : { className: 'net-bad', text: `✕ авто: ${formatWeight(auto)} кг — проверьте брутто и места` }
}

export function ReviewRow({ item, rowNumber, resultItem, selected, onSelected, onOpenPicker, onClearDirectory, onChange, onRemove, onPreview, onUpload, onResetNet }: ReviewRowProps) {
  const id = item.id ?? ''
  const value = (field: keyof GlovesItemInput) => String(item[field] ?? '')
  const photos = photosOf(item)
  const resultError = resultItem?.errors?.length || resultItem?.warnings?.length
  const verified = item.directorySnapshot?.verificationStatus === 'operator_verified'
  const netHintValue = netHint(item)
  const netPlaceholder = 'Авто'
  const fieldInput = (field: keyof GlovesItemInput, placeholder = '') => (
    <input className="review-input" inputMode="decimal" value={value(field)} onChange={(event) => onChange(id, field, event.target.value.replace(',', '.'))} placeholder={placeholder} />
  )
  const statusReady = resultItem?.status === 'calculated' || (Boolean(item.code) && verified)
  const statusLabel = resultItem?.errors?.length ? 'Проверить' : statusReady ? 'Готово' : item.code ? 'Проверить' : 'Нужен код'
  const productCell = (
    <td className="gloves-product-cell review-product-cell">
      <div className="review-product-head">
        <input className="product-name-input" value={value('name')} onChange={(event) => onChange(id, 'name', event.target.value)} placeholder="Название товара" />
        <div className="gloves-row-photos">
          {photos.length ? <button type="button" className="photo-preview-button" onClick={() => onPreview(photos[0])} title="Открыть крупно"><img src={photos[0]} alt="Фото товара" />{photos.length > 1 && <b>+{photos.length - 1}</b>}</button> : <span className="photo-empty">Нет фото</span>}
          <label className="photo-upload-button">+ фото<input type="file" accept="image/jpeg,image/png" onChange={(event) => onUpload(id, event)} /></label>
        </div>
      </div>
      <div className="review-product-code"><strong>{item.code || 'Код ТН ВЭД не выбран'}</strong>{item.code && <button type="button" className="button-link" onClick={onClearDirectory}>Снять</button>}</div>
      <div className="review-product-actions"><button type="button" className="button-link" onClick={onOpenPicker}>{item.code ? 'Изменить карточку ТН ВЭД' : 'Подобрать код'}</button>{item.code && <small className={verified ? 'status-ok' : 'status-warn'}>{verified ? 'Проверено оператором' : 'Требует проверки ставок'}</small>}</div>
    </td>
  )

  return (
    <tr className={[item.excluded ? 'row-excluded' : '', resultError ? 'row-error' : ''].filter(Boolean).join(' ')}>
      <td><input type="checkbox" checked={!item.excluded} onChange={(event) => onChange(id, 'excluded', !event.target.checked)} aria-label="Включить строку" /></td>
      <td><input type="checkbox" checked={selected} onChange={(event) => onSelected(event.target.checked)} aria-label="Выбрать строку для карточки" /></td>
      <td>{rowNumber}</td>
      {productCell}
      <td className="review-edit-cell">{fieldInput('places')}</td>
      <td className="review-edit-cell">{fieldInput('pairsPerPlace')}</td>
      <td className="review-edit-cell">{fieldInput('pairs')}</td>
      <td className="review-edit-cell">{fieldInput('grossKg')}</td>
      <td className="review-net-cell">
        <div className="review-net-editor">
          <div className="review-net-input-wrap">
            {fieldInput('netKg', netPlaceholder)}
            <small className={`net-status ${netHintValue.className}`}>{netHintValue.text}</small>
          </div>
          {item.netKg && item.netSource !== 'excel' && <button type="button" className="net-reset" onClick={onResetNet}>Вернуть авто</button>}
        </div>
      </td>
      <td className="review-edit-cell">{fieldInput('coefficient', 'ИТС')}</td>
      <td><span className={`status-badge ${statusReady ? 'status-calculated' : 'status-review'}`}>{statusLabel}</span></td>
      <td><button type="button" className="row-remove" onClick={() => onRemove(id)} title="Удалить строку">×</button></td>
    </tr>
  )
}
