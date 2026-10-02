import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'

type ResultItem = GoodsCalculationResult['items'][number]
type ReviewTone = 'ok' | 'warn' | 'bad' | 'off'

// Цена и стоимость вводятся на этапе 3 — их ошибки здесь не показываем,
// иначе строка выглядит сломанной, хотя на этом этапе исправить нечего.
const PRICE_ISSUE = /цен[аеуы]|стоимост/i
const reviewOnly = (list?: string[]) => (list ?? []).map((text) => text.trim()).filter((text) => text && !PRICE_ISSUE.test(text))

export const hasNomenclatureRates = (item: GoodsItemInput) => item.directorySnapshot?.rateSource === 'nomenclature'

// Статус строки на этапе 2: ошибки последнего расчёта (кроме цены) и готовность кода ТН ВЭД.
export function reviewStatus(item: GoodsItemInput, resultItem?: ResultItem): { tone: ReviewTone; label: string; title: string } {
  const errors = reviewOnly(resultItem?.errors)
  const warnings = reviewOnly(resultItem?.warnings)
  // Ставки из номенклатуры ТН ВЭД ЕАЭС — готовы к расчёту; иначе код стоит проверить.
  const verified = hasNomenclatureRates(item)
  if (item.excluded) return { tone: 'off', label: 'Исключена', title: 'Строка не учитывается в расчёте' }
  if (errors.length) return { tone: 'bad', label: 'Ошибка', title: [...errors, ...warnings].join('\n') }
  const label = !item.code ? 'Нужен код ТН ВЭД' : verified || resultItem?.status === 'calculated' ? 'Готово' : 'Проверьте код'
  return { tone: label === 'Готово' ? 'ok' : 'warn', label, title: [label, ...warnings].join('\n') }
}
