import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'

const amount = (value?: string) => {
  const text = String(value ?? '').replace(/\s/g, '').replace(',', '.')
  if (!text) return null
  const parsed = Number(text)
  return Number.isFinite(parsed) ? parsed : null
}

// Цена единицы строки на экране результата: цена текущего результата (после правки или
// подбора под бюджет), иначе цена строки сметы.
export function shownPrice(items: GoodsItemInput[], result: GoodsCalculationResult | null, index: number) {
  const item = result?.items[index]
  return item?.pricePerPair || item?.pairPrice || items[index]?.pricePerPair || ''
}

// Цены различаются как числа; пустая цена с одной из сторон — не изменение.
export function priceDiffers(base: string | undefined, shown: string) {
  const before = amount(base)
  const after = amount(shown)
  return before !== null && after !== null && before !== after
}

// Строки, где цена на экране отличается от цены из Excel. Строки без цены в Excel (цена по ИТП)
// не считаются: их цену программа не меняла по просьбе сотрудника.
export function changedFromExcel(items: GoodsItemInput[], result: GoodsCalculationResult | null) {
  return items.flatMap((item, index) => !item.excluded && priceDiffers(item.sourcePricePerPair, shownPrice(items, result, index)) ? [index] : [])
}

// Строки, где цена на экране отличается от цены строки сметы: их меняет «Сбросить цены».
export function changedFromRows(items: GoodsItemInput[], result: GoodsCalculationResult | null) {
  return items.flatMap((item, index) => !item.excluded && priceDiffers(item.pricePerPair, shownPrice(items, result, index)) ? [index] : [])
}
