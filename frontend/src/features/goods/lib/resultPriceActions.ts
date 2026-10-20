import { Dispatch, SetStateAction } from 'react'
import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'

type PriceState = {
  items: GoodsItemInput[]
  result: GoodsCalculationResult | null
  previousBulkResult: GoodsCalculationResult | null
  setResult: Dispatch<SetStateAction<GoodsCalculationResult | null>>
  setPreviousBulkResult: Dispatch<SetStateAction<GoodsCalculationResult | null>>
  // Пересчёт без замены исходных строк (updateInputItems = false).
  calculate: (itemsOverride?: GoodsItemInput[], updateInputItems?: boolean) => Promise<unknown>
}

// Этап 3: временные цены единицы — ручная правка, цены подбора под бюджет и откат.
export function resultPriceActions({ items, result, previousBulkResult, setResult, setPreviousBulkResult, calculate }: PriceState) {
  return {
    recalculateResultPrice(index: number, value: string) {
      // Остальные строки сохраняют цены текущего результата (после распределения бюджета),
      // иначе правка одной цены сбросила бы их к исходным.
      const nextItems = items.map((item, itemIndex) => itemIndex === index
        ? { ...item, pricePerPair: value }
        : { ...item, pricePerPair: result?.items[itemIndex]?.pricePerPair || item.pricePerPair })
      setResult((current) => current ? { ...current, items: current.items.map((item, itemIndex) => itemIndex === index ? { ...item, pricePerPair: value } : item) } : current)
      void calculate(nextItems, false)
    },

    // Цены после распределения бюджета: прежний результат остаётся для отката.
    applyDistributedPrices(nextItems: GoodsItemInput[]) {
      if (result) setPreviousBulkResult(result)
      void calculate(nextItems, false)
    },

    // Сброс к ценам строк сметы — тоже откатывается кнопкой «Отменить».
    restoreInitialResult() {
      if (result) setPreviousBulkResult(result)
      void calculate(undefined, false)
    },

    undoLastPriceChange() {
      if (!previousBulkResult) return
      setResult(previousBulkResult)
      setPreviousBulkResult(null)
    },
  }
}
