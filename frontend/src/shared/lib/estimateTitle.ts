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

type Named = { id: number; name: string; client: string }
const defaultTitle = /^Смета товаров(\s*·\s*\d{4}-\d{2}-\d{2})?$/

// Своё название сотрудника, а не стандартное «Смета товаров · дата».
export const hasOwnName = (name: string) => !defaultTitle.test(estimateTitle(name))

// Название сметы в списках и заголовке. Своё название — как есть; стандартное «Смета товаров · дата»
// одинаково у всех смет (дата и так видна рядом), поэтому вместо него клиент, а без клиента — «Смета №id».
export function estimateName(estimate: Named) {
  if (hasOwnName(estimate.name)) return estimateDisplayTitle(estimate.name)
  return estimate.client.trim() || `Смета №${estimate.id}`
}

// Строка под названием: клиент; если клиент уже стал названием — номер сметы.
export function estimateSubtitle(estimate: Named) {
  const client = estimate.client.trim()
  if (!client) return 'Без клиента'
  return estimateName(estimate) === client ? `Смета №${estimate.id}` : client
}
