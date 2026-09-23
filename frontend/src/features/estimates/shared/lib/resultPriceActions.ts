import { Dispatch, SetStateAction } from 'react'
import { GlovesCalculationResult, GlovesItemInput } from '../types'

type PriceState = {
  items: GlovesItemInput[]
  result: GlovesCalculationResult | null
  previousBulkResult: GlovesCalculationResult | null
  setResult: Dispatch<SetStateAction<GlovesCalculationResult | null>>
  setPreviousBulkResult: Dispatch<SetStateAction<GlovesCalculationResult | null>>
  // Пересчёт без сохранения черновика на сервер и без замены исходных строк.
  calculate: (itemsOverride?: GlovesItemInput[], saveServerDraft?: boolean, updateInputItems?: boolean) => Promise<void>
}

// Этап 3: временные цены единицы — ручная правка, массовое уменьшение и откат.
export function resultPriceActions({ items, result, previousBulkResult, setResult, setPreviousBulkResult, calculate }: PriceState) {
  return {
    recalculateResultPrice(index: number, value: string) {
      const nextItems = items.map((item, itemIndex) => itemIndex === index ? { ...item, pricePerPair: value } : item)
      setResult((current) => current ? { ...current, items: current.items.map((item, itemIndex) => itemIndex === index ? { ...item, pricePerPair: value } : item) } : current)
      void calculate(nextItems, false, false)
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
          return { ...item, pricePerPair: currentPrice.toFixed(4).replace(/\.?0+$/, '') }
        }
        const nextPrice = Math.max(0, currentPrice - decrement)
        return { ...item, pricePerPair: nextPrice.toFixed(4).replace(/\.?0+$/, '') }
      })
      setPreviousBulkResult(result)
      void calculate(nextItems, false, false)
    },

    restoreInitialResult() {
      setPreviousBulkResult(null)
      void calculate(undefined, false, false)
    },

    undoLastBulkPriceDecrease() {
      if (!previousBulkResult) return
      setResult(previousBulkResult)
      setPreviousBulkResult(null)
    },
  }
}
