import { ChangeEvent, InputHTMLAttributes, KeyboardEvent, memo, useState } from 'react'
import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'
import { autoNetFor, DEFAULT_PACKAGE_WEIGHT as DEFAULT_PACKAGE, effectivePackageWeight, formatWeight, netHint } from '../../lib/netWeight'
import { photosOf } from '@/shared/lib/itemPhotos'
import { autoQuantityHint, QuantityField } from '../../lib/quantities'
import { RowCheck } from '../../lib/reviewCheck'
import { hasNomenclatureRates, ReviewTone, reviewStatus, rowMissing } from './reviewStatus'
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
  // Ошибки прошлого расчёта этой строки (по id), если актуального результата нет.
  pastCheck?: RowCheck
  selected: boolean
  onSelect: (id: string, checked: boolean, range: boolean) => void
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

// Числа в тексте вне полей — через запятую; в полях — как вводятся (точка), значения не меняются.
const ru = (text: string) => text.replace(/(\d)\.(\d)/g, '$1,$2')

// Spreadsheet-style navigation: Enter / ↓ go to the same column in the next row, Shift+Enter / ↑ go up.
function moveFocus(event: KeyboardEvent<HTMLInputElement>) {
  const step = event.key === 'ArrowDown' || (event.key === 'Enter' && !event.shiftKey) ? 1 : event.key === 'ArrowUp' || (event.key === 'Enter' && event.shiftKey) ? -1 : 0
  const cell = event.currentTarget.closest('td')
  const row = cell?.parentElement
  if (!step || !cell || !row) return
  const target = (step > 0 ? row.nextElementSibling : row.previousElementSibling) as HTMLTableRowElement | null
  const input = target?.cells[cell.cellIndex]?.querySelector<HTMLInputElement>(event.currentTarget.classList.contains('rv-name') ? 'input.rv-name' : 'input.rv-input')
  if (!input) return
  event.preventDefault()
  input.focus()
  input.select()
}

// Значок состояния строки: форма отличается, а не только цвет (✓, !, ?, ↻, –).
function StateIcon({ tone }: { tone: ReviewTone }) {
  return <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
    {tone === 'ok' && <path d="M2.8 6.3 5 8.4l4.2-4.6" />}
    {tone === 'bad' && <><path d="M6 2.8v4" /><circle cx="6" cy="9.2" r=".5" fill="currentColor" /></>}
    {(tone === 'warn' || tone === 'todo') && <><path d="M4.4 4.4a1.7 1.7 0 1 1 2.4 1.5c-.5.3-.8.6-.8 1.2v.2" /><circle cx="6" cy="9.4" r=".5" fill="currentColor" /></>}
    {tone === 'changed' && <><path d="M9.3 4.6A3.6 3.6 0 1 0 9.6 7" /><path d="M9.6 2.4v2.4H7.2" /></>}
    {tone === 'off' && <path d="M3.4 6h5.2" />}
  </svg>
}

export const ReviewRow = memo(function ReviewRow({ item, rowNumber, resultItem, pastCheck, selected, onSelect, onOpenPicker, onClearDirectory, onChange, onRemove, onPhotos, onUpload, onResetNet, defaultPackageWeight, agentStatus, onAgent }: ReviewRowProps) {
  const [expanded, setExpanded] = useState(false)
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
  const status = reviewStatus(item, resultItem, pastCheck, rowMissing(item, defaultPackageWeight))
  const reasonId = status.issues.length > 0 || status.tone === 'changed' ? `rv-reason-${id}` : undefined
  // Нет брутто и нетто — авто-нетто не посчитать: поле брутто подсвечено.
  const needGross = !item.excluded && !item.grossKg.trim() && !item.netKg.trim()
  const numberInput = (field: keyof GoodsItemInput, label: string, extra: InputHTMLAttributes<HTMLInputElement> = {}) => (
    <input className="rv-input" inputMode="decimal" placeholder="—" value={value(field)} onChange={(event) => onChange(id, field, event.target.value.replace(',', '.'))} onKeyDown={moveFocus} aria-label={`${label}, строка ${rowNumber}`} {...extra} />
  )
  // Пустое из трёх полей «места / штук в месте / количество» показывает вычисленное значение.
  const quantityInput = (field: QuantityField, label: string) => {
    const hint = autoQuantityHint(item, field)
    return numberInput(field, label, hint ? { placeholder: hint.placeholder, title: ru(hint.title), className: 'rv-input rv-auto' } : {})
  }

  return (
    <tr data-row-id={id} className={[item.excluded ? 'rv-excluded' : '', `rv-row-${status.tone}`, status.attention ? 'attention' : '', selected ? 'rv-selected' : ''].filter(Boolean).join(' ')}>
      <td className="rv-check"><input type="checkbox" checked={selected} onChange={(event) => onSelect(id, event.target.checked, (event.nativeEvent as MouseEvent).shiftKey)} aria-label={`Выбрать строку ${rowNumber}`} /></td>
      <td className="rv-num">
        <span className="rv-num-cell">
          <span>{rowNumber}</span>
          <span className={`rv-state ${status.tone}${status.source === 'past' ? ' past' : ''}`} role="img" aria-label={status.label} title={[status.label, ...status.issues].join('\n')}><StateIcon tone={status.tone} /></span>
        </span>
      </td>
      <td className="rv-product">
        <div className="rv-product-grid">
          <div className="rv-photo">
            {photos.length > 0 && <button type="button" className="rv-thumb" onClick={() => onPhotos(id, 0)} title={photos.length > 1 ? `Фото строки (${photos.length}): посмотреть, удалить, перенести` : 'Фото строки: посмотреть, удалить, перенести'}><img src={photos[0]} alt="Фото товара" />{photos.length > 1 && <b>{photos.length}</b>}</button>}
            <label className={photos.length ? 'rv-photo-add' : 'rv-photo-add rv-photo-empty'} title="Добавить фото">+<input type="file" accept="image/jpeg,image/png" onChange={(event) => onUpload(id, event)} aria-label={`Добавить фото, строка ${rowNumber}`} /></label>
          </div>
          <input className="rv-input rv-name" value={value('name')} onChange={(event) => onChange(id, 'name', event.target.value)} onKeyDown={moveFocus} placeholder="Название товара" title={value('name')} aria-label={`Название товара, строка ${rowNumber}`} aria-describedby={reasonId} />
          <div className="rv-code-line">
            <button type="button" className={`rv-code ${item.code ? (verified ? 'ok' : 'warn') : 'missing'}`} onClick={() => onOpenPicker(id)} title={item.code ? `${verified ? 'Ставки из номенклатуры ТН ВЭД ЕАЭС' : 'Кода нет в номенклатуре ТН ВЭД ЕАЭС или она не загружена'} · изменить товар или код` : 'Выбрать товар или код ТН ВЭД'}>{item.code ? formatCode(item.code) : 'Выбрать код ТН ВЭД'}</button>
            {item.code && !verified && <span className="rv-code-note">нет в номенклатуре</span>}
            {item.code && <button type="button" className="rv-code-clear" onClick={() => onClearDirectory(id)} title="Снять товар и код">снять</button>}
            <AgentButton status={agentStatus} code={item.code} rowNumber={rowNumber} onClick={() => onAgent(id)} />
            {item.otherNames?.length ? <span className="rv-other-names" title={`Другие наименования из файла: ${item.otherNames.join(' · ')}`}>{item.otherNames.join(' · ')}</span> : null}
          </div>
        </div>
        {reasonId && <button type="button" id={reasonId} className={`rv-reason ${status.tone}`} aria-expanded={expanded} onClick={() => setExpanded(!expanded)} title={expanded ? 'Свернуть' : [status.label, ...status.issues].join('\n')}>
          {status.tone === 'changed' && <b className="rv-reason-label">Изменено — требуется пересчёт{status.issues.length > 0 ? '. В прошлом расчёте:' : ''}</b>}
          {status.tone !== 'changed' && status.source === 'past' && <b className="rv-reason-label">Прошлый расчёт:</b>}
          {expanded
            ? status.issues.map((issue) => <span key={issue}>{issue}</span>)
            : status.issues.length > 0 && <span>{status.issues[0]}{status.issues.length > 1 && <b> +{status.issues.length - 1}</b>}</span>}
        </button>}
      </td>
      <td>{quantityInput('places', 'Места')}</td>
      <td>{quantityInput('pairsPerPlace', 'Штук в месте')}</td>
      <td>{quantityInput('pairs', 'Количество')}</td>
      <td>{numberInput('grossKg', 'Брутто', needGross ? { className: 'rv-input rv-need', 'aria-invalid': true, title: 'Укажите брутто: без него не посчитать авто-нетто' } : {})}</td>
      <td className="rv-net">
        {/* Одна строка: упаковка места (видна при наведении или если своя) и нетто. Подсказка — только при ошибке. */}
        <div className="rv-net-box">
          {autoNetMode && <label className={ownPackage ? 'rv-pkg own' : 'rv-pkg'} title={ownPackage ? `Своя упаковка места для этой строки, кг (общая — ${ru(defaultPackageWeight || DEFAULT_PACKAGE)} кг)` : 'Вес упаковки одного места, кг. Измените, чтобы задать свою упаковку для этой строки'}>
            уп<input inputMode="decimal" value={item.packageWeightKg} placeholder={defaultPackageWeight || DEFAULT_PACKAGE} onChange={(event) => onChange(id, 'packageWeightKg', event.target.value.replace(',', '.'))} aria-label={`Вес упаковки, кг, строка ${rowNumber}`} />
            {ownPackage && <button type="button" onClick={() => onChange(id, 'packageWeightKg', defaultPackageWeight)} title="Вернуть общую упаковку" aria-label="Вернуть общую упаковку">↺</button>}
          </label>}
          <div className="rv-net-input">
            {numberInput('netKg', 'Нетто', { placeholder: autoNet !== null ? `авто ${formatWeight(autoNet)}` : '—', title: ru(netHintValue.text), className: autoNet !== null && !item.netKg ? 'rv-input rv-auto' : 'rv-input' })}
            {item.netKg && item.netSource !== 'excel' && <button type="button" className="rv-net-reset" onClick={() => onResetNet(id)} title="Вернуть автоматический расчёт нетто" aria-label="Вернуть авто-нетто">↺</button>}
          </div>
        </div>
        {netHintValue.className === 'net-bad' && <small className="rv-hint net-bad" title={ru(netHintValue.text)}>{ru(netHintValue.text)}</small>}
      </td>
      <td className="rv-include"><label className="rv-switch" title={item.excluded ? 'Строка не учитывается в расчёте' : 'Строка учитывается в расчёте'}><input type="checkbox" checked={!item.excluded} onChange={(event) => onChange(id, 'excluded', !event.target.checked)} aria-label={`Учитывать строку ${rowNumber} в расчёте`} /><span /></label></td>
      <td className="rv-remove-col"><button type="button" className="rv-remove" onClick={() => onRemove(id)} title="Удалить строку (можно вернуть)" aria-label={`Удалить строку ${rowNumber}`}>×</button></td>
    </tr>
  )
})

// Кнопка чата подбора ИИ: «Подбор» и отметка состояния (…, ?, !, ✓ или предложенный код).
// В компактном виде слово «Подбор» скрыто (остаётся в подсказке и для диктора) — видна отметка.
function AgentButton({ status, code, rowNumber, onClick }: { status?: AgentRowStatus; code: string; rowNumber: number; onClick: () => void }) {
  let mark = ''
  let tone = 'idle'
  let title = 'Подобрать код ТН ВЭД с ИИ'
  if (status?.status === 'running') { mark = '…'; tone = 'running'; title = 'Идёт подбор кода — открыть чат' }
  else if (status?.status === 'searching') { mark = '…'; tone = 'running'; title = 'Поиск в базе решений — открыть чат' }
  else if (status?.status === 'ready') { title = 'Поиск в базе закончен — открыть чат и начать подбор' }
  else if (status?.status === 'waiting') { mark = '?'; tone = 'waiting'; title = 'Подбор ждёт ответа на вопрос — открыть чат' }
  else if (status?.status === 'error') { mark = '!'; tone = 'error'; title = 'Подбор прервался — открыть чат' }
  else if (status?.status === 'done' && status.bestCode) {
    if (status.bestCode === code) { mark = '✓'; tone = 'same'; title = 'Код совпадает с подбором — открыть чат' }
    else { mark = formatCode(status.bestCode); tone = 'suggest'; title = 'Подбор предлагает другой код — открыть чат' }
  } else if (status) { title = 'Открыть чат подбора' }
  return <button type="button" className={`rv-agent ${tone}`} onClick={onClick} title={title} aria-label={`${title}, строка ${rowNumber}`}>
    {tone === 'idle' && <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><path d="M6 1.5v2M6 8.5v2M1.5 6h2M8.5 6h2M3 3l1.2 1.2M7.8 7.8 9 9M9 3 7.8 4.2M4.2 7.8 3 9" /></svg>}
    <span className="rv-agent-label">{tone === 'suggest' ? 'Подбор:' : 'Подбор'}</span>
    {mark && <span className="rv-agent-mark">{mark}</span>}
  </button>
}
