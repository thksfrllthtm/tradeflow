import { formatDisplayNumber } from '@/shared/lib/formatDisplayNumber'
import { GlovesCalculationResult } from '../../../shared/types'

type Value = string | number | undefined | null

// Числа результата приходят строками (иногда с пробелами-разделителями).
export function parseCalculatorNumber(value: Value) {
  const parsed = Number(String(value ?? '').replace(/\s/g, '').replace(',', '.'))
  return Number.isFinite(parsed) ? parsed : null
}

export function calculatorInputValue(value: Value) {
  const parsed = parseCalculatorNumber(value)
  return parsed === null ? '' : parsed.toFixed(2)
}

export const display = (value: Value) =>
  value === undefined || value === null || value === '' ? '—' : String(value)

export const displayNumber = (value: Value) => {
  const shown = display(value)
  return shown === '—' ? shown : formatDisplayNumber(shown)
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
export function hasAntiDumping(result: GlovesCalculationResult) {
  return isPositive(result.totals.antiDumpingKzt) || result.items.some((item) => isPositive(item.antiDumpingKzt))
}

// Итоги по включённым строкам для строки «Итого» таблицы.
export function rowTotals(result: GlovesCalculationResult) {
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
