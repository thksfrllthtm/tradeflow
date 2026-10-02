import { GoodsItemInput } from '@/shared/types/goods'
import { formatDisplayNumber } from '@/shared/lib/formatDisplayNumber'
import { autoQuantity } from './quantities'

// Вес нетто строки: авто-расчёт (брутто − места × вес упаковки) и подсказка к полю «Нетто».
function parseDecimal(value: string): number | null {
  const normalized = String(value ?? '').trim().replace(',', '.')
  if (!normalized || !/^[+-]?(\d+(\.\d*)?|\.\d+)$/.test(normalized)) return null
  const parsed = Number(normalized)
  return Number.isFinite(parsed) ? parsed : null
}

export function formatWeight(value: number): string {
  return formatDisplayNumber(value.toFixed(3).replace(/\.?0+$/, ''))
}

export function autoNetFor(item: GoodsItemInput): number | null {
  const gross = parseDecimal(item.grossKg)
  // Пустые места сервер вычисляет из количества и штук в месте — нетто считаем так же.
  const places = parseDecimal(item.places) ?? autoQuantity(item, 'places')
  const packageWeight = parseDecimal(item.packageWeightKg)
  if (gross === null || places === null || packageWeight === null) return null
  return gross - places * packageWeight
}

export function netHint(item: GoodsItemInput): { className: string; text: string } {
  const gross = parseDecimal(item.grossKg)
  const net = parseDecimal(item.netKg)
  const auto = autoNetFor(item)
  const excel = item.netSource === 'excel'
  const calculated = item.netSource === 'calculated'
  const manual = !excel && !calculated && item.netKg.trim() !== ''

  if (excel || manual || calculated) {
    if (net === null) return { className: 'net-bad', text: 'Нетто — не число' }
    if (gross === null) return { className: 'net-warn', text: 'Нет брутто для проверки' }
    const ok = net > 1e-9 && net <= gross + 1e-9
    const label = excel ? 'из Excel' : calculated ? 'рассчитано' : 'вручную'
    const compare = auto !== null ? ` · авто было бы ${formatWeight(auto)} кг` : ''
    return ok
      ? { className: 'net-ok', text: `✓ ${label} · ${formatWeight(net)} кг${compare}` }
      : { className: 'net-bad', text: `✕ нетто должно быть больше 0 и не больше брутто${compare}` }
  }

  if (auto === null) return { className: 'net-warn', text: 'Укажите брутто и места для авто-нетто' }
  if (gross === null) return { className: 'net-warn', text: `Авто: ${formatWeight(auto)} кг` }
  return auto > 1e-9 && auto <= gross + 1e-9
    ? { className: 'net-ok', text: `✓ авто: ${formatWeight(auto)} кг` }
    : { className: 'net-bad', text: `✕ авто: ${formatWeight(auto)} кг — проверьте брутто и места` }
}

