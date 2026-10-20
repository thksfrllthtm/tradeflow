import { formatRuDate } from './dates'

// Название сметы для списка и заголовка: старые копии «Копия перчаток · …» показываются
// как «Смета товаров · …», пустое название — «Смета товаров».
export function estimateTitle(value: string) {
  const name = value.trim()
  const suffix = name.replace(/^Копия\s+(?:перчаток|товаров)\s*[·–-]?\s*/i, '')
  return suffix !== name ? `Смета товаров${suffix ? ` · ${suffix}` : ''}` : name || 'Смета товаров'
}

// Для показа: дата в названии («Смета товаров · 2026-10-14») — как везде, «14.10.2026». Хранится название как есть.
export function estimateDisplayTitle(value: string) {
  return estimateTitle(value).replace(/\b(\d{4}-\d{2}-\d{2})\b/g, (date) => formatRuDate(date))
}
