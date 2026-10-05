import { Dispatch, SetStateAction } from 'react'
import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'
import { priceText } from './priceText'

type PriceState = {
  items: GoodsItemInput[]
  result: GoodsCalculationResult | null
  previousBulkResult: GoodsCalculationResult | null
  setResult: Dispatch<SetStateAction<GoodsCalculationResult | null>>
  setPreviousBulkResult: Dispatch<SetStateAction<GoodsCalculationResult | null>>
  // Пересчёт без замены исходных строк (updateInputItems = false).
  calculate: (itemsOverride?: GoodsItemInput[], updateInputItems?: boolean) => Promise<void>
}

// Этап 3: временные цены единицы — ручная правка, массовое уменьшение и откат.
export function resultPriceActions({ items, result, previousBulkResult, setResult, setPreviousBulkResult, calculate }: PriceState) {
  return {
    recalculateResultPrice(index: number, value: string) {
      // Остальные строки сохраняют цены текущего результата (после распределения бюджета или
      // массового вычета), иначе правка одной цены сбросила бы их к исходным.
      const nextItems = items.map((item, itemIndex) => itemIndex === index
        ? { ...item, pricePerPair: value }
        : { ...item, pricePerPair: result?.items[itemIndex]?.pricePerPair || item.pricePerPair })
      setResult((current) => current ? { ...current, items: current.items.map((item, itemIndex) => itemIndex === index ? { ...item, pricePerPair: value } : item) } : current)
      void calculate(nextItems, false)
    },

    decreaseResultPrices(value: string, selectedIndexes?: number[]) {
      const decrement = Number(String(value).replace(',', '.'))
      if (!Number.isFinite(decrement) || decrement <= 0 || !result) return
      const selected = selectedIndexes ? new Set(selectedIndexes) : null

      const nextItems = items.map((item, index) => {
        const resultItem = result.items[index]
        const currentPrice = Number(resultItem?.pricePerPair || resultItem?.pairPrice || item.pricePerPair || '')
        if (!Number.isFinite(currentPrice)) return item
        if (selected && !selected.has(index)) {
          return { ...item, pricePerPair: priceText(currentPrice) }
        }
        const nextPrice = Math.max(0, currentPrice - decrement)
        return { ...item, pricePerPair: priceText(nextPrice) }
      })
      setPreviousBulkResult(result)
      void calculate(nextItems, false)
    },

    // Цены после распределения бюджета: как массовый вычет — прежний результат остаётся для отката.
    applyDistributedPrices(nextItems: GoodsItemInput[]) {
      if (result) setPreviousBulkResult(result)
      void calculate(nextItems, false)
    },

    // Сброс к ценам строк сметы — тоже откатывается кнопкой «Отменить».
    restoreInitialResult() {
      if (result) setPreviousBulkResult(result)
      void calculate(undefined, false)
    },

    undoLastBulkPriceDecrease() {
      if (!previousBulkResult) return
      setResult(previousBulkResult)
      setPreviousBulkResult(null)
    },
  }
}
