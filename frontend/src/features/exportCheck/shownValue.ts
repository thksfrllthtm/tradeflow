import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'

// Число для показа: тот же знак после запятой, что и при сравнении на сервере (сумма — 2, цена — 4,
// вес — до 3), и разделители тысяч. Поэтому «280.00» и «280» выглядят одинаково, а расхождение,
// которое нашёл сервер, всегда видно глазами. Не число — как есть.
export function shownValue(label: string, text: string) {
  const value = text.trim()
  if (!/^-?\d+(\.\d+)?$/.test(value)) return value || '—'
  const number = Number(value)
  const lower = label.toLowerCase()
  let fixed = value
  if (lower.includes('сумма') || lower.includes('стоимость')) fixed = number.toFixed(2)
  else if (lower.includes('цена')) fixed = number.toFixed(4)
  else if (lower.includes('кг')) fixed = number.toFixed(3).replace(/\.?0+$/, '')
  return formatRuNumber(fixed)
}
