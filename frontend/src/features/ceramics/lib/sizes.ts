import { CeramicSize, SizeUnit } from '../types'

export const sizeUnits: { value: SizeUnit; label: string }[] = [
  { value: 'inch', label: 'дюйм (")' },
  { value: 'cm', label: 'см' },
  { value: 'mm', label: 'мм' },
  { value: 'ml', label: 'мл' },
  { value: 'l', label: 'л' },
]

// Короткая запись размера: 7", 20 см, 500 мл.
export function formatSize(size: CeramicSize | null | undefined) {
  if (!size?.value) return ''
  switch (size.unit) {
    case 'inch': return `${size.value}"`
    case 'cm': return `${size.value} см`
    case 'mm': return `${size.value} мм`
    case 'ml': return `${size.value} мл`
    case 'l': return `${size.value} л`
  }
}

export function sizeKey(size: CeramicSize | null | undefined) {
  return size?.value ? `${size.value} ${size.unit}` : ''
}

// Наименование для базы: русская часть, «/ », китайское название клиента —
// «Керамическая фруктовница 7 дюймов для взрослых/ 成年人陶瓷盘7高脚水果盘».
export function baseName(russian: string, chinese: string) {
  const left = russian.trim()
  const right = chinese.trim()
  if (!left) return right
  return right ? `${left}/ ${right}` : left
}

// Число без лишних нулей: «7,50» → «7.5».
export function normalizeNumber(text: string) {
  const compact = String(text ?? '').replace(/\s+/g, '').replace(',', '.')
  if (!/^\d+(\.\d+)?$/.test(compact)) return ''
  return String(Number(compact))
}

// Число цены из ячейки: «0,2», «$4.50», «0.2 USD», «1 234,5», «1,234.5»; не число — пусто.
export function priceNumber(text: string) {
  let compact = String(text ?? '').replace(/\s+/g, '').replace(/^\$/, '').replace(/(usd|\$)$/i, '')
  // Есть и запятая, и точка: первая из них — разделитель тысяч.
  if (compact.includes(',') && compact.includes('.')) {
    compact = compact.indexOf(',') < compact.indexOf('.') ? compact.replace(/,/g, '') : compact.replace(/\./g, '')
  }
  return normalizeNumber(compact)
}

// Цена для базы: положительное число, иначе пусто.
export function parsePriceCell(text: string) {
  const value = priceNumber(text)
  return value && Number(value) > 0 ? value : ''
}
