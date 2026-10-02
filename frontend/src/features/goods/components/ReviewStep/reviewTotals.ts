import { GoodsItemInput } from '@/shared/types/goods'
import { autoNetFor } from '../../lib/netWeight'
import { autoQuantity } from '../../lib/quantities'

const number = (value: string) => {
  const text = String(value ?? '').trim().replace(',', '.')
  const parsed = Number(text)
  return text && Number.isFinite(parsed) ? parsed : null
}

// Итоги строки «Итого» по включённым строкам. Пустые поля берутся так же, как
// их покажет строка: количество и места — из формулы, нетто — авто-расчётом.
export function reviewTotals(items: GoodsItemInput[], defaultPackageWeight: string) {
  return items.reduce((totals, item) => {
    if (item.excluded) return totals
    const netItem = item.packageWeightKg.trim() ? item : { ...item, packageWeightKg: defaultPackageWeight }
    totals.places += number(item.places) ?? autoQuantity(item, 'places') ?? 0
    totals.quantity += number(item.pairs) ?? autoQuantity(item, 'pairs') ?? 0
    totals.grossKg += number(item.grossKg) ?? 0
    totals.netKg += (item.netKg.trim() ? number(item.netKg) : autoNetFor(netItem)) ?? 0
    return totals
  }, { places: 0, quantity: 0, grossKg: 0, netKg: 0 })
}
