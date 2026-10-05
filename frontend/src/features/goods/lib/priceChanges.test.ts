import { describe, expect, it } from 'vitest'
import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'
import { changedFromExcel, changedFromRows, priceDiffers } from './priceChanges'

const row = (pricePerPair: string, sourcePricePerPair?: string, excluded = false) => ({ pricePerPair, sourcePricePerPair, excluded }) as GoodsItemInput
const result = (...prices: string[]) => ({ items: prices.map((pricePerPair) => ({ pricePerPair, pairPrice: '' })) }) as unknown as GoodsCalculationResult

describe('priceChanges', () => {
  it('compares prices as numbers and ignores empty sides', () => {
    expect(priceDiffers('12.5', '12,50')).toBe(false)
    expect(priceDiffers('12.5', '12.4')).toBe(true)
    expect(priceDiffers('', '12.4')).toBe(false)
    expect(priceDiffers('12.5', '')).toBe(false)
  })

  it('counts rows changed against Excel and against estimate rows', () => {
    // 0: подбор под бюджет; 1: правка на этапе 2 (строка = экран); 2: без цены в Excel; 3: исключена.
    const items = [row('10', '10'), row('9', '10'), row('', ''), row('5', '5', true)]
    const shown = result('8', '9', '7', '1')
    expect(changedFromExcel(items, shown)).toEqual([0, 1])
    expect(changedFromRows(items, shown)).toEqual([0])
    expect(changedFromExcel(items, null)).toEqual([1])
  })
})
