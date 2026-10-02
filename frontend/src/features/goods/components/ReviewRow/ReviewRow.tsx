import { ChangeEvent, InputHTMLAttributes, KeyboardEvent, memo } from 'react'
import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'
import { autoNetFor, DEFAULT_PACKAGE_WEIGHT as DEFAULT_PACKAGE, effectivePackageWeight, formatWeight, netHint } from '../../lib/netWeight'
import { photosOf } from '@/shared/lib/itemPhotos'
import { autoQuantityHint, QuantityField } from '../../lib/quantities'
import { hasNomenclatureRates, reviewStatus } from './reviewStatus'
import { AgentRowStatus } from '../../agent/agentApi'
import '../../goods.css'
import './ReviewRow.css'
import { formatCode } from '@/shared/tnved/codes'

// Обработчики получают id строки и должны быть стабильными (не пересоздаваться
// на каждый рендер) — тогда memo перерисовывает только изменённую строку.
type ReviewRowProps = {
  item: GoodsItemInput
  rowNumber: number
  resultItem?: GoodsCalculationResult['items'][number]
  selected: boolean
  onSelect: (id: string, checked: boolean) => void
  onOpenPicker: (id: string) => void
  onClearDirectory: (id: string) => void
  onChange: (id: string, field: keyof GoodsItemInput, value: string | boolean | undefined) => void
  onRemove: (id: string) => void
  // Фото строки: просмотр, удаление, главное фото, перенос в другую строку.
  onPhotos: (id: string, index: number) => void
  onUpload: (id: string, event: ChangeEvent<HTMLInputElement>) => void
  onResetNet: (id: string) => void
  // Общая упаковка из параметров расчёта: строка с другим значением считается «со своей упаковкой».
  defaultPackageWeight: string
  // Последний подбор кода ИИ для строки и открытие его чата.
  agentStatus?: AgentRowStatus
  onAgent: (id: string) => void
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

export const ReviewRow = memo(function ReviewRow({ item, rowNumber, resultItem, selected, onSelect, onOpenPicker, onClearDirectory, onChange, onRemove, onPhotos, onUpload, onResetNet, defaultPackageWeight, agentStatus, onAgent }: ReviewRowProps) {
  const id = item.id ?? ''
  const value = (field: keyof GoodsItemInput) => String(item[field] ?? '')
  const photos = photosOf(item)
  const verified = hasNomenclatureRates(item)
  const ownPackage = item.packageWeightKg.trim() !== '' && item.packageWeightKg !== defaultPackageWeight
  // Пустая своя упаковка на сервере заменяется общей (или 0,3 кг) — считаем подсказку так же.
  const netItem = { ...item, packageWeightKg: effectivePackageWeight(item, defaultPackageWeight) }
  const netHintValue = netHint(netItem)
  const autoNet = autoNetFor(netItem)
  // Упаковка влияет только на авто-нетто; для нетто из Excel или введённого вручную её не показываем.
  const autoNetMode = !item.netKg
  const numberInput = (field: keyof GoodsItemInput, label: string, extra: InputHTMLAttributes<HTMLInputElement> = {}) => (
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
            {photos.length > 0 && <button type="button" className="rv-thumb" onClick={() => onPhotos(id, 0)} title={photos.length > 1 ? `Фото строки (${photos.length}): посмотреть, удалить, перенести` : 'Фото строки: посмотреть, удалить, перенести'}><img src={photos[0]} alt="Фото товара" />{photos.length > 1 && <b>{photos.length}</b>}</button>}
            <label className={photos.length ? 'rv-photo-add' : 'rv-photo-add rv-photo-empty'} title="Добавить фото">+<input type="file" accept="image/jpeg,image/png" onChange={(event) => onUpload(id, event)} aria-label={`Добавить фото, строка ${rowNumber}`} /></label>
          </div>
          <input className="rv-input rv-name" value={value('name')} onChange={(event) => onChange(id, 'name', event.target.value)} onKeyDown={moveFocus} placeholder="Название товара" aria-label={`Название товара, строка ${rowNumber}`} />
          <div className="rv-code-line">
            <button type="button" className={`rv-code ${item.code ? (verified ? 'ok' : 'warn') : 'missing'}`} onClick={() => onOpenPicker(id)} title={item.code ? `${verified ? 'Ставки из номенклатуры ТН ВЭД ЕАЭС' : 'Кода нет в номенклатуре ТН ВЭД ЕАЭС или она не загружена'} · изменить товар или код` : 'Выбрать товар или код ТН ВЭД'}>{item.code || 'Выбрать код ТН ВЭД'}</button>
            {item.code && !verified && <span className="rv-code-note">нет в номенклатуре</span>}
            {item.code && <button type="button" className="rv-code-clear" onClick={() => onClearDirectory(id)} title="Снять товар и код">снять</button>}
            <AgentButton status={agentStatus} code={item.code} onClick={() => onAgent(id)} />
            {item.otherNames?.length ? <span className="rv-other-names" title={`Другие наименования из файла: ${item.otherNames.join(' · ')}`}>{item.otherNames.join(' · ')}</span> : null}
          </div>
        </div>
      </td>
      <td>{quantityInput('places', 'Места')}</td>
      <td>{quantityInput('pairsPerPlace', 'Штук в месте')}</td>
      <td>{quantityInput('pairs', 'Количество')}</td>
      <td>{numberInput('grossKg', 'Брутто')}</td>
      <td className="rv-net">
        {/* Одна строка: упаковка места (видна при наведении или если своя) и нетто. Подсказка — только при ошибке. */}
        <div className="rv-net-box">
          {autoNetMode && <label className={ownPackage ? 'rv-pkg own' : 'rv-pkg'} title={ownPackage ? `Своя упаковка места для этой строки, кг (общая — ${defaultPackageWeight || DEFAULT_PACKAGE} кг)` : 'Вес упаковки одного места, кг. Измените, чтобы задать свою упаковку для этой строки'}>
            уп<input inputMode="decimal" value={item.packageWeightKg} placeholder={defaultPackageWeight || DEFAULT_PACKAGE} onChange={(event) => onChange(id, 'packageWeightKg', event.target.value.replace(',', '.'))} aria-label={`Вес упаковки, кг, строка ${rowNumber}`} />
            {ownPackage && <button type="button" onClick={() => onChange(id, 'packageWeightKg', defaultPackageWeight)} title="Вернуть общую упаковку" aria-label="Вернуть общую упаковку">↺</button>}
          </label>}
          <div className="rv-net-input">
            {numberInput('netKg', 'Нетто', { placeholder: autoNet !== null ? formatWeight(autoNet) : '—', title: netHintValue.text, className: autoNet !== null && !item.netKg ? 'rv-input rv-auto' : 'rv-input' })}
            {item.netKg && item.netSource !== 'excel' && <button type="button" className="rv-net-reset" onClick={() => onResetNet(id)} title="Вернуть автоматический расчёт нетто" aria-label="Вернуть авто-нетто">↺</button>}
          </div>
        </div>
        {netHintValue.className === 'net-bad' && <small className="rv-hint net-bad" title={netHintValue.text}>{netHintValue.text}</small>}
      </td>
      <td className="rv-include"><label className="rv-switch" title={item.excluded ? 'Строка не учитывается в расчёте' : 'Строка учитывается в расчёте'}><input type="checkbox" checked={!item.excluded} onChange={(event) => onChange(id, 'excluded', !event.target.checked)} aria-label={`Учитывать строку ${rowNumber} в расчёте`} /><span /></label></td>
      <td className="rv-status"><span className={`rs-dot ${status.tone}`} title={status.title} aria-label={status.label}>{status.tone === 'ok' ? '✓' : status.tone === 'off' ? '–' : status.tone === 'todo' ? '' : '!'}</span></td>
      <td className="rv-remove-col"><button type="button" className="rv-remove" onClick={() => onRemove(id)} title="Удалить строку" aria-label={`Удалить строку ${rowNumber}`}>×</button></td>
    </tr>
  )
})

// Кнопка чата подбора ИИ: подпись показывает, что сейчас с подбором строки.
function AgentButton({ status, code, onClick }: { status?: AgentRowStatus; code: string; onClick: () => void }) {
  let label = 'Подбор'
  let tone = ''
  let title = 'Подобрать код ТН ВЭД'
  if (status?.status === 'running') { label = 'Подбор…'; tone = 'running'; title = 'Идёт подбор кода — открыть чат' }
  else if (status?.status === 'searching') { label = 'Подбор…'; tone = 'running'; title = 'Поиск в базе решений — открыть чат' }
  else if (status?.status === 'ready') { title = 'Поиск в базе закончен — открыть чат и начать подбор' }
  else if (status?.status === 'waiting') { label = 'Подбор ?'; tone = 'waiting'; title = 'Подбор ждёт ответа на вопрос — открыть чат' }
  else if (status?.status === 'error') { label = 'Подбор !'; tone = 'error'; title = 'Подбор прервался — открыть чат' }
  else if (status?.status === 'done' && status.bestCode) {
    if (status.bestCode === code) { label = 'Подбор ✓'; tone = 'same'; title = 'Код совпадает с подбором — открыть чат' }
    else { label = `Подбор: ${formatCode(status.bestCode)}`; tone = 'suggest'; title = 'Подбор предлагает другой код — открыть чат' }
  } else if (status) { title = 'Открыть чат подбора' }
  return <button type="button" className={`rv-agent ${tone}`.trim()} onClick={onClick} title={title}>{label}</button>
}
