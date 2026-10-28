import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { moneyText, roundHalfUp, weightText } from '@/shared/lib/rounding'
import { isSirFallback } from './errorGroups'
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

// Колонки для вставки в Excel — общие со страницей утильсбора.
export { excelColumn, excelColumnHtml } from '@/shared/lib/excelColumn'

// Текстовая колонка для Excel: перенос строки или табуляция внутри названия разбили бы его на несколько ячеек.
export const textColumn = (values: string[]) => values.map((value) => value.replace(/\s+/g, ' ').trim()).join('\n')

// Название строки в две строки: основное (русское) и под ним мелко — китайское или исходное из Excel.
export function itemNames(item: DisplayNameItem, source?: DisplayNameItem): { main: string; sub: string } {
  const snapshot = item.directorySnapshot
  const russian = typeof snapshot?.name === 'string' ? snapshot.name.trim() : ''
  const chinese = typeof snapshot?.nameZh === 'string' ? snapshot.nameZh.trim() : ''
  const productChosen = Boolean(source?.productDirectoryId) || snapshot?.productDirectoryId != null
  if (productChosen && russian) return { main: russian, sub: chinese !== russian ? chinese : '' }
  const main = itemDisplayName(item, source)
  const original = source?.supplierName?.trim() || item.supplierName?.trim() || ''
  return { main, sub: original !== main ? original : '' }
}

type PaymentItem = { excluded?: boolean; errors?: string[]; dutyKzt?: string; antiDumpingKzt?: string; vatKzt?: string; feeKzt?: string }

// К оплате по строке, ₸: пошлина + антидемпинг + НДС + сбор (как итог на сервере). Сумма в тиынах —
// без ошибок двоичной записи. Пусто — строка исключена или с ошибкой (пошлина и НДС не посчитаны).
export function rowPaymentKzt(item: PaymentItem): string {
  const parts = [item.dutyKzt, item.antiDumpingKzt, item.vatKzt, item.feeKzt]
  if (item.excluded || item.errors?.length || parts.every((value) => String(value ?? '').trim() === '')) return ''
  const cents = parts.reduce((total, value) => total + Math.round((parseCalculatorNumber(value) ?? 0) * 100), 0)
  return moneyText(cents / 100)
}

// Ставка НДС из снимка строки: «16%».
export function vatRateText(snapshot?: Record<string, unknown>) {
  const percent = (snapshot?.vat as { percent?: string } | undefined)?.percent
  return percent ? `${formatRuNumber(percent)}%` : ''
}

// Растаможка к оплате в USD по курсу сметы; null — нет суммы или курса.
export function customsPaymentsUsd(result: GoodsCalculationResult) {
  const kzt = parseCalculatorNumber(result.totals.customsPaymentsKzt)
  const rate = parseCalculatorNumber(result.usdKzt)
  return kzt !== null && rate !== null && rate > 0 ? kzt / rate : null
}

type SirLine = { country: string; profile: string; priceUsd: string; discountPercent: string; pricePerKgUsd: string; weightKg: string; weight: string; valueUsd: string }
const WEIGHT_LABEL: Record<string, string> = { net: 'нетто', gross: 'брутто' }

// Как посчитана ТС строки по СУР — подсказка суммы ТС.
export function sirTitle(sir: SirLine) {
  const discount = Number(sir.discountPercent) > 0 ? ` − ${displayNumber(sir.discountPercent)} % = ${fixedNumber(sir.pricePerKgUsd, 2)}` : ''
  return `ТС по СУР (${sir.country}, профиль ${sir.profile}): ${fixedNumber(sir.priceUsd, 2)} USD/кг${discount} × ${fixedNumber(sir.weightKg, 1)} кг ${WEIGHT_LABEL[sir.weight] ?? sir.weight} = ${displayNumber(sir.valueUsd)} USD, плюс доставка`
}

// Число с фиксированным числом знаков для показа: индикатор СУР — 2 («2,00»), вес в таблице — 1 («20 230,0»).
export function fixedNumber(value: Value, decimals: number) {
  const parsed = parseCalculatorNumber(value)
  return value === undefined || value === null || String(value).trim() === '' || parsed === null ? '—' : formatRuNumber(roundHalfUp(parsed, decimals).toFixed(decimals))
}

// Рассчитанные строки: сервер не считает рассчитанной строку без индикатора СУР (она по инвойсу, с предупреждением),
// но в итог она входит — для экрана она рассчитана.
export function calculatedRows(result: GoodsCalculationResult) {
  const invoiceFallback = result.items.filter((item) => !item.excluded && item.status !== 'calculated' && !item.errors?.length && (item.warnings ?? []).length > 0 && item.warnings.every(isSirFallback)).length
  return result.totals.calculatedCount + invoiceFallback
}
