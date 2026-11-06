import { Dispatch, SetStateAction } from 'react'
import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'

type PriceState = {
  items: GoodsItemInput[]
  result: GoodsCalculationResult | null
  previousBulkResult: GoodsCalculationResult | null
  setItems: Dispatch<SetStateAction<GoodsItemInput[]>>
  setResult: Dispatch<SetStateAction<GoodsCalculationResult | null>>
  setPreviousBulkResult: Dispatch<SetStateAction<GoodsCalculationResult | null>>
  // Пересчёт без замены исходных строк (updateInputItems = false).
  calculate: (itemsOverride?: GoodsItemInput[], updateInputItems?: boolean) => Promise<unknown>
}

// Этап 3: временные цены единицы — ручная правка, цены подбора под бюджет и откат; пересчёт с этими ценами.
export function resultPriceActions({ items, result, previousBulkResult, setItems, setResult, setPreviousBulkResult, calculate }: PriceState) {
  // Строки с ценами текущего результата (правка, подбор под бюджет) — для пересчёта без потери этих цен.
  function withResultPrices(list: GoodsItemInput[]) {
    return list.map((item, itemIndex) => ({ ...item, pricePerPair: result?.items[itemIndex]?.pricePerPair || item.pricePerPair }))
  }

  return {
    withResultPrices,

    changeResultPrice(index: number, value: string) {
      // Keep the edited value in the source rows as well as in the result. The
      // next row recalculation is built from `items`; without this mirror the
      // previous row still contains its old price and appears to disappear.
      setItems((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, pricePerPair: value } : item))
      setResult((current) => current ? { ...current, items: current.items.map((item, itemIndex) => itemIndex === index ? { ...item, pricePerPair: value } : item) } : current)
    },

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

    // «Учитывать строку в расчёте» в панели строки результата; смета пересчитывается с текущими ценами.
    setResultIncluded(index: number, included: boolean) {
      const nextItems = items.map((item, itemIndex) => itemIndex === index ? { ...item, excluded: !included } : item)
      setItems(nextItems)
      void calculate(withResultPrices(nextItems), false)
    },

    // Вариант антидемпинга строки: сохраняется в строке сметы, расчёт повторяется с ценами текущего результата.
    chooseAntiDumping(index: number, key: string) {
      const choice = key || undefined
      setItems((current) => current.map((item, itemIndex) => itemIndex === index ? { ...item, antiDumpingChoice: choice } : item))
      void calculate(withResultPrices(items.map((item, itemIndex) => itemIndex === index ? { ...item, antiDumpingChoice: choice } : item)), false)
    },
  }
}
