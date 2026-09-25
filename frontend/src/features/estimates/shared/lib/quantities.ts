import { formatDisplayNumber } from '@/shared/lib/formatDisplayNumber'
import { GlovesItemInput } from '../types'

// Количество = места × штук в месте. Если два значения заполнены, а третье пустое,
// сервер при расчёте вычисляет его сам. Здесь то же вычисление — чтобы показать
// значение в пустом поле на этапе проверки («авто 40»), не записывая его в строку.
export type QuantityField = 'pairs' | 'places' | 'pairsPerPlace'

function positive(value: string): number | null {
  const text = String(value ?? '').trim().replace(',', '.')
  const parsed = Number(text)
  return text !== '' && Number.isFinite(parsed) && parsed > 0 ? parsed : null
}

export function autoQuantity(item: GlovesItemInput, field: QuantityField): number | null {
  if (String(item[field] ?? '').trim()) return null
  const pairs = positive(item.pairs)
  const places = positive(item.places)
  const perPlace = positive(item.pairsPerPlace)
  if (field === 'pairs') return places !== null && perPlace !== null ? places * perPlace : null
  if (field === 'places') return pairs !== null && perPlace !== null ? pairs / perPlace : null
  return pairs !== null && places !== null ? pairs / places : null
}

// Подсказка для пустого поля: placeholder «авто N» и пояснение; при дробном результате — предупреждение.
export function autoQuantityHint(item: GlovesItemInput, field: QuantityField): { placeholder: string; title: string } | null {
  const value = autoQuantity(item, field)
  if (value === null) return null
  const text = formatDisplayNumber(value.toFixed(3).replace(/\.?0+$/, ''))
  const formula = field === 'pairs' ? 'места × штук в месте' : field === 'places' ? 'количество ÷ штук в месте' : 'количество ÷ места'
  const whole = Math.abs(value - Math.round(value)) < 1e-9
  return {
    placeholder: `авто ${text}`,
    title: whole ? `Рассчитается автоматически: ${formula} = ${text}` : `${formula} = ${text} — не целое число, проверьте количество и места`,
  }
}
