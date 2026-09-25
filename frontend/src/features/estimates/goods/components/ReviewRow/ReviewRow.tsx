import { ChangeEvent, InputHTMLAttributes, KeyboardEvent, memo } from 'react'
import { GlovesCalculationResult, GlovesItemInput } from '../../../shared/types'
import { autoNetFor, formatWeight, netHint } from '../../../shared/lib/netWeight'
import { photosOf } from '../../../shared/lib/itemPhotos'
import { autoQuantityHint, QuantityField } from '../../../shared/lib/quantities'
import { reviewStatus } from './reviewStatus'
import '../../../shared/styles/calculatorCommon.css'
import './ReviewRow.css'

// Обработчики получают id строки и должны быть стабильными (не пересоздаваться
// на каждый рендер) — тогда memo перерисовывает только изменённую строку.
export type ReviewRowProps = {
  item: GlovesItemInput
  rowNumber: number
  resultItem?: GlovesCalculationResult['items'][number]
  selected: boolean
  onSelect: (id: string, checked: boolean) => void
  onOpenPicker: (id: string) => void
  onClearDirectory: (id: string) => void
  onChange: (id: string, field: keyof GlovesItemInput, value: string | boolean | undefined) => void
  onRemove: (id: string) => void
  onPreview: (values: string[], index?: number) => void
  onUpload: (id: string, event: ChangeEvent<HTMLInputElement>) => void
  onResetNet: (id: string) => void
  // Общая упаковка из параметров расчёта: строка с другим значением считается «со своей упаковкой».
  defaultPackageWeight: string
}

// Spreadsheet-style navigation: Enter / ↓ go to the same column in the next row, Shift+Enter / ↑ go up.
function moveFocus(event: KeyboardEvent<HTMLInputElement>) {
  const step = event.key === 'ArrowDown' || (event.key === 'Enter' && !event.shiftKey) ? 1 : event.key === 'ArrowUp' || (event.key === 'Enter' && event.shiftKey) ? -1 : 0
  const cell = event.currentTarget.closest('td')
  const row = cell?.parentElement
  if (!step || !cell || !row) return
  const target = (step > 0 ? row.nextElementSibling : row.previousElementSibling) as HTMLTableRowElement | null
  const input = target?.cells[cell.cellIndex]?.querySelector<HTMLInputElement>('input.rv-input')
  if (!input) return
  event.preventDefault()
  input.focus()
  input.select()
}

export const ReviewRow = memo(function ReviewRow({ item, rowNumber, resultItem, selected, onSelect, onOpenPicker, onClearDirectory, onChange, onRemove, onPreview, onUpload, onResetNet, defaultPackageWeight }: ReviewRowProps) {
  const id = item.id ?? ''
  const value = (field: keyof GlovesItemInput) => String(item[field] ?? '')
  const photos = photosOf(item)
  const verified = item.directorySnapshot?.verificationStatus === 'operator_verified'
  const ownPackage = item.packageWeightKg !== defaultPackageWeight
  // Пустая своя упаковка на сервере заменяется общей — считаем подсказку так же.
  const netItem = item.packageWeightKg.trim() ? item : { ...item, packageWeightKg: defaultPackageWeight }
  const netHintValue = netHint(netItem)
  const autoNet = autoNetFor(netItem)
  // Упаковка влияет только на авто-нетто; для нетто из Excel или введённого вручную её не показываем.
  const autoNetMode = !item.netKg
  const numberInput = (field: keyof GlovesItemInput, label: string, extra: InputHTMLAttributes<HTMLInputElement> = {}) => (
    <input className="rv-input" inputMode="decimal" placeholder="—" value={value(field)} onChange={(event) => onChange(id, field, event.target.value.replace(',', '.'))} onKeyDown={moveFocus} aria-label={`${label}, строка ${rowNumber}`} {...extra} />
  )
  // Пустое из трёх полей «места / штук в месте / количество» показывает вычисленное значение.
  const quantityInput = (field: QuantityField, label: string) => {
    const hint = autoQuantityHint(item, field)
    return numberInput(field, label, hint ? { placeholder: hint.placeholder, title: hint.title } : {})
  }
  const status = reviewStatus(item, resultItem)

  return (
    <tr className={[item.excluded ? 'rv-excluded' : '', status.tone === 'bad' ? 'rv-error' : '', selected ? 'rv-selected' : ''].filter(Boolean).join(' ')}>
      <td className="rv-check"><input type="checkbox" checked={selected} onChange={(event) => onSelect(id, event.target.checked)} aria-label={`Выбрать строку ${rowNumber}`} /></td>
      <td className="rv-num">{rowNumber}</td>
      <td className="rv-product">
        <div className="rv-product-grid">
          <div className="rv-photo">
            {photos.length > 0 && <button type="button" className="rv-thumb" onClick={() => onPreview(photos, 0)} title={photos.length > 1 ? `Открыть фотографии (${photos.length})` : 'Открыть фото'}><img src={photos[0]} alt="Фото товара" />{photos.length > 1 && <b>{photos.length}</b>}</button>}
            <label className={photos.length ? 'rv-photo-add' : 'rv-photo-add rv-photo-empty'} title="Добавить фото">+<input type="file" accept="image/jpeg,image/png" onChange={(event) => onUpload(id, event)} aria-label={`Добавить фото, строка ${rowNumber}`} /></label>
          </div>
          <input className="rv-input rv-name" value={value('name')} onChange={(event) => onChange(id, 'name', event.target.value)} onKeyDown={moveFocus} placeholder="Название товара" aria-label={`Название товара, строка ${rowNumber}`} />
          <div className="rv-code-line">
            <button type="button" className={`rv-code ${item.code ? (verified ? 'ok' : 'warn') : 'missing'}`} onClick={() => onOpenPicker(id)} title={item.code ? `${verified ? 'Проверено оператором' : 'Ставки не проверены оператором'} · изменить товар или код` : 'Выбрать товар или код ТН ВЭД'}>{item.code || 'Выбрать код ТН ВЭД'}</button>
            {item.code && !verified && <span className="rv-code-note">проверьте ставки</span>}
            {item.code && <button type="button" className="rv-code-clear" onClick={() => onClearDirectory(id)} title="Снять товар и код">снять</button>}
          </div>
        </div>
      </td>
      <td>{quantityInput('places', 'Места')}</td>
      <td>{quantityInput('pairsPerPlace', 'Штук в месте')}</td>
      <td>{quantityInput('pairs', 'Количество')}</td>
      <td>{numberInput('grossKg', 'Брутто')}</td>
      <td className="rv-net">
        <div className="rv-net-box">
          {numberInput('netKg', 'Нетто', { placeholder: autoNet !== null ? `авто ${formatWeight(autoNet)}` : 'авто', title: netHintValue.text })}
          {item.netKg && item.netSource !== 'excel' && <button type="button" className="rv-net-reset" onClick={() => onResetNet(id)} title="Вернуть автоматический расчёт нетто" aria-label="Вернуть авто-нетто">↺</button>}
        </div>
        {autoNetMode && <label className={ownPackage ? 'rv-pkg own' : 'rv-pkg'} title={ownPackage ? `Своя упаковка для этой строки (общая — ${defaultPackageWeight || '0.3'} кг)` : 'Вес упаковки одного места. Измените, чтобы задать свою упаковку для этой строки'}>
          уп.<input inputMode="decimal" value={item.packageWeightKg} placeholder={defaultPackageWeight} onChange={(event) => onChange(id, 'packageWeightKg', event.target.value.replace(',', '.'))} aria-label={`Вес упаковки, кг, строка ${rowNumber}`} />кг
          {ownPackage && <button type="button" onClick={() => onChange(id, 'packageWeightKg', defaultPackageWeight)} title="Вернуть общую упаковку" aria-label="Вернуть общую упаковку">↺</button>}
        </label>}
        {netHintValue.className !== 'net-ok' && <small className={`rv-hint ${netHintValue.className}`} title={netHintValue.text}>{netHintValue.text}</small>}
      </td>
      <td className="rv-include"><label className="rv-switch" title={item.excluded ? 'Строка не учитывается в расчёте' : 'Строка учитывается в расчёте'}><input type="checkbox" checked={!item.excluded} onChange={(event) => onChange(id, 'excluded', !event.target.checked)} aria-label={`Учитывать строку ${rowNumber} в расчёте`} /><span /></label></td>
      <td className="rv-status"><span className={`rs-dot ${status.tone}`} title={status.title} aria-label={status.label}>{status.tone === 'ok' ? '✓' : status.tone === 'off' ? '–' : '!'}</span></td>
      <td className="rv-remove-col"><button type="button" className="rv-remove" onClick={() => onRemove(id)} title="Удалить строку" aria-label={`Удалить строку ${rowNumber}`}>×</button></td>
    </tr>
  )
})
