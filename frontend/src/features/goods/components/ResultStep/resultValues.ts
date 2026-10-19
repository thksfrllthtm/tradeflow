import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { moneyText, weightText } from '@/shared/lib/rounding'
import { GoodsCalculationResult } from '@/shared/types/goods'

type Value = string | number | undefined | null

// Числа результата приходят строками (иногда с пробелами-разделителями).
export function parseCalculatorNumber(value: Value) {
  const parsed = Number(String(value ?? '').replace(/\s/g, '').replace(',', '.'))
  return Number.isFinite(parsed) ? parsed : null
}

// Вес в поле ввода — по правилу гр.38: до трёх знаков, меньше грамма — до шести.
export function weightInputValue(value: Value) {
  const parsed = parseCalculatorNumber(value)
  return parsed === null ? '' : weightText(parsed, true)
}

export function calculatorInputValue(value: Value) {
  const parsed = parseCalculatorNumber(value)
  return parsed === null ? '' : moneyText(parsed)
}

export const display = (value: Value) =>
  value === undefined || value === null || value === '' ? '—' : String(value)

// Число для показа: разряды пробелом, дробная часть через запятую. Значение не меняется.
export const displayNumber = (value: Value) => {
  const shown = display(value)
  return shown === '—' ? shown : formatRuNumber(shown)
}

type DisplayNameItem = { standardName?: string; name?: string; supplierName?: string; productDirectoryId?: number; directorySnapshot?: Record<string, unknown> }

// Название строки в таблице результата. Выбран товар из справочника товаров — его название
// («русское / китайское»). Иначе — название из Excel (как в строке на этапе проверки), даже если
// код ТН ВЭД подобран: описание кода ТН ВЭД не заменяет название товара.
export function itemDisplayName(item: DisplayNameItem, source?: DisplayNameItem) {
  const snapshot = item.directorySnapshot
  const russian = typeof snapshot?.name === 'string' ? snapshot.name.trim() : ''
  const chinese = typeof snapshot?.nameZh === 'string' ? snapshot.nameZh.trim() : ''
  const productChosen = Boolean(source?.productDirectoryId) || snapshot?.productDirectoryId != null
  if (productChosen) return (russian && chinese ? `${russian} / ${chinese}` : '') || item.standardName || russian || chinese || source?.name || '—'
  return source?.name?.trim() || item.supplierName?.trim() || item.standardName || russian || chinese || '—'
}

const isPositive = (value: Value) => (parseCalculatorNumber(value) ?? 0) > 0

// Антидемпинг встречается редко — колонку и итог показываем только когда он есть.
export function hasAntiDumping(result: GoodsCalculationResult) {
  return isPositive(result.totals.antiDumpingKzt) || result.items.some((item) => isPositive(item.antiDumpingKzt))
}

// Итоги по включённым строкам для строки «Итого» таблицы.
export function rowTotals(result: GoodsCalculationResult) {
  return result.items.reduce((totals, item) => {
    if (item.excluded) return totals
    const number = (value: Value) => {
      const parsed = Number(String(value ?? '').replace(/\s/g, '').replace(',', '.'))
      return Number.isFinite(parsed) ? parsed : 0
    }
    totals.quantity += number(item.pairs)
    totals.grossKg += number(item.grossKg)
    totals.netKg += number(item.netKg)
    totals.places += number(item.places)
    return totals
  }, { quantity: 0, grossKg: 0, netKg: 0, places: 0 })
}

// Состояние строки результата: исключена, ошибка (не рассчитана), предупреждение
// (не рассчитана без ошибок или рассчитана с предупреждениями), рассчитана.
export type RowState = 'off' | 'bad' | 'warn' | 'ok'
export function rowState(item: { excluded?: boolean; status?: string; errors?: string[]; warnings?: string[] }): RowState {
  if (item.excluded) return 'off'
  if (item.errors?.length) return 'bad'
  if (item.status !== 'calculated' || item.warnings?.length) return 'warn'
  return 'ok'
}

// Ставка пошлины ЕТТ из снимка строки (подставлена сервером из номенклатуры) — подпись под суммой.
export function dutyRateText(snapshot?: Record<string, unknown>) {
  const duty = snapshot?.duty as { type?: string; percent?: string; amount?: string; currency?: string; unit?: string } | undefined
  if (!duty) return ''
  const percent = duty.percent ? `${formatRuNumber(duty.percent)}%` : ''
  const specific = duty.amount ? `${formatRuNumber(duty.amount)} ${duty.currency ?? ''}${duty.unit ? `/${duty.unit}` : ''}`.trim() : ''
  if (duty.type === 'percentage') return percent ? `ЕТТ ${percent}` : ''
  if (duty.type === 'specific') return specific ? `ЕТТ ${specific}` : ''
  if (duty.type === 'combined_minimum') return percent && specific ? `ЕТТ ${percent}, не менее ${specific}` : ''
  return ''
}

// Колонка для вставки в Excel столбцом: по числу на строку, пустая строка — пустая ячейка.
// Без пробелов-разрядов, дробная часть через запятую — так ждёт русский/казахстанский Excel сотрудников.
export const excelColumn = (values: Value[]) =>
  values.map((value) => String(value ?? '').replace(/\s/g, '').replace('.', ',')).join('\n')

// Та же колонка как HTML для Excel: число берётся из x:num (с точкой) при любых разделителях в настройках Excel,
// иначе «1,3272» при запятой-разделителе тысяч становится 13272. Без xmlns:x Excel атрибут игнорирует.
export const excelColumnHtml = (values: Value[]) => {
  const cells = values.map((value) => {
    const number = String(value ?? '').replace(/\s/g, '')
    return /^-?\d+(\.\d+)?$/.test(number) ? `<td x:num="${number}">${number.replace('.', ',')}</td>` : '<td></td>'
  })
  return `<html xmlns:x="urn:schemas-microsoft-com:office:excel"><body><table>${cells.map((cell) => `<tr>${cell}</tr>`).join('')}</table></body></html>`
}

// Текстовая колонка для Excel: перенос строки или табуляция внутри названия разбили бы его на несколько ячеек.
export const textColumn = (values: string[]) => values.map((value) => value.replace(/\s+/g, ' ').trim()).join('\n')
