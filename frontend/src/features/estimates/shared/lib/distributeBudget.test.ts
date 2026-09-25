import { describe, expect, it } from 'vitest'
import { budgetGroupKey, BudgetRowAccess, distributeBudget } from './distributeBudget'

type Row = { code: string; quantity: number; price?: string; excluded?: boolean; name?: string }

const access: BudgetRowAccess<Row> = {
  getCode: (row) => row.code,
  getQuantity: (row) => row.quantity,
  getPrice: (row) => row.price ? Number(row.price) : null,
  setPrice: (row, price) => ({ ...row, price }),
  isIncluded: (row) => !row.excluded,
}

const included = (rows: Row[]) => rows.filter((row) => !row.excluded)
// Бюджет на стоимость товаров: итог = Σ цена × количество.
const cost = async (rows: Row[]) => included(rows).reduce((sum, row) => sum + Number(row.price ?? 0) * row.quantity, 0)
// Похоже на растаможку: пошлина 10% (но не меньше 0.5 USD за единицу), НДС 12% на стоимость и пошлину, сбор 60 USD.
const customs = async (rows: Row[]) => 60 + included(rows).reduce((sum, row) => {
  const value = Number(row.price ?? 0) * row.quantity
  const duty = Math.max(value * 0.1, row.quantity * 0.5)
  return sum + duty + (value + duty) * 0.12
}, 0)

describe('distributeBudget', () => {
  it('matches the worked example when the budget is the goods cost', async () => {
    const result = await distributeBudget([
      { code: '6116', quantity: 500, price: '4' },
      { code: '6216', quantity: 1200 },
      { code: '6116', quantity: 300 },
    ], 11000, access, { evaluate: cost })
    expect(result.status).toBe('ok')
    // Цель 11000 − 25: цена второй группы (10975 − 3200) / 1200 ≈ 6.4792, недостача ≈ 25.
    expect([result.items[0].price, result.items[2].price]).toEqual(['4', '4'])
    expect(Number(result.items[1].price)).toBeCloseTo(6.479, 3)
    expect(result.gap).toBeGreaterThanOrEqual(20)
    expect(result.gap).toBeLessThanOrEqual(30)
    expect(result.total).toBe(Math.round((3200 + Number(result.items[1].price) * 1200) * 100) / 100)
  })

  it('brings the customs payment 20–30 below the budget in a few calculations', async () => {
    const rows: Row[] = [{ code: 'A', quantity: 800 }, { code: 'B', quantity: 1200 }, { code: 'C', quantity: 75, price: '12' }]
    const result = await distributeBudget(rows, 11000, access, { evaluate: customs, weights: { [budgetGroupKey('A', 0)]: 7.5, [budgetGroupKey('B', 1)]: 0.4 } })
    expect(result.status).toBe('ok')
    expect(result.gap).toBeGreaterThanOrEqual(20)
    expect(result.gap).toBeLessThanOrEqual(30)
    expect(await customs(result.items)).toBeCloseTo(result.total, 2)
    expect(result.items[2].price).toBe('12')
    expect(result.evaluations).toBeLessThanOrEqual(8)
  })

  it('gives the same price to every row with the same code', async () => {
    const result = await distributeBudget([
      { code: '6116', quantity: 10 },
      { code: '6216', quantity: 20 },
      { code: ' 6116 ', quantity: 30 },
    ], 1000, access, { evaluate: customs })
    expect(result.items[0].price).toBe(result.items[2].price)
    expect(result.status).toBe('ok')
  })

  it('does not change fixed prices and gives a partly priced group its price', async () => {
    const result = await distributeBudget([
      { code: 'A', quantity: 100, price: '2.5' },
      { code: 'A', quantity: 50 },
      { code: 'B', quantity: 10, price: '7' },
      { code: 'C', quantity: 40 },
    ], 1000, access, { evaluate: cost })
    expect(result.items.map((row) => row.price).slice(0, 3)).toEqual(['2.5', '2.5', '7'])
    expect(result.fixedRows).toBe(3)
    expect(result.status).toBe('ok')
  })

  it('reports the closest variant when the window cannot be reached', async () => {
    // One step of 0.0001 on a million units moves the total by 100 USD — more than the 10 USD wide window.
    const result = await distributeBudget([{ code: 'A', quantity: 1_000_000 }], 12_345, access, { evaluate: cost })
    expect(result.status).toBe('closest')
    expect(result.message).toMatch(/Не удалось точно попасть в бюджет/)
  })

  it('does nothing when every price is set, the payment cannot be counted or is over budget without prices', async () => {
    const allFixed = await distributeBudget([{ code: 'A', quantity: 10, price: '1' }], 1000, access, { evaluate: cost })
    expect(allFixed.status).toBe('nothing')
    expect(allFixed.items[0].price).toBe('1')
    expect((await distributeBudget([{ code: 'A', quantity: 0 }], 1000, access, { evaluate: cost })).status).toBe('nothing')
    const broken = await distributeBudget([{ code: 'A', quantity: 10 }], 1000, access, { evaluate: async () => null })
    expect(broken.status).toBe('nothing')
    expect(broken.message).toMatch(/исправьте ошибки/)
    // The minimum duty alone (0.5 × 5000 + VAT + fee) is above a 1000 USD budget.
    const tooSmall = await distributeBudget([{ code: 'A', quantity: 5000 }], 1000, access, { evaluate: customs })
    expect(tooSmall.status).toBe('nothing')
    expect(tooSmall.items[0].price).toBeUndefined()
  })

  it('splits by weights', async () => {
    const result = await distributeBudget([{ code: 'A', quantity: 100 }, { code: 'B', quantity: 100 }], 3015, access, {
      evaluate: cost, weights: { [budgetGroupKey('A', 0)]: 1, [budgetGroupKey('B', 1)]: 2 },
    })
    expect(Number(result.items[1].price) / Number(result.items[0].price)).toBeCloseTo(2, 2)
    expect(result.status).toBe('ok')
  })

  it('skips excluded rows and rows without a code get their own price', async () => {
    const result = await distributeBudget([
      { code: '', quantity: 10 },
      { code: '', quantity: 10 },
      { code: 'A', quantity: 10, excluded: true },
    ], 500, access, { evaluate: cost, weights: { [budgetGroupKey('', 0)]: 1, [budgetGroupKey('', 1)]: 3 } })
    expect(result.items[2].price).toBeUndefined()
    expect(Number(result.items[1].price)).toBeGreaterThan(Number(result.items[0].price))
  })
})

describe('rows that already have a price', () => {
  it('keep their own price even when the same code has other prices', async () => {
    const result = await distributeBudget([
      { code: '6116', quantity: 100, price: '2.5' },
      { code: '6116', quantity: 100, price: '3' },
      { code: '6116', quantity: 50 },
      { code: '6216', quantity: 200 },
    ], 2000, access, { evaluate: cost })
    expect(result.items.map((row) => row.price).slice(0, 3)).toEqual(['2.5', '3', '2.5'])
    expect(result.fixedRows).toBe(3)
    expect(result.status).toBe('ok')
  })
})

describe('products inside a code (AI prices)', () => {
  const productOf = (row: Row, index: number) => `${row.code.trim()}|${row.name ?? index}`

  it('splits the code price in the AI proportion and keeps one price per product', async () => {
    const rows: Row[] = [
      { code: '8201', name: 'штыковая', quantity: 200 },
      { code: '8201', name: 'совковая', quantity: 500 },
      { code: '7310', name: 'бочка', quantity: 50 },
      { code: '8201', name: 'совковая', quantity: 200 },
      { code: '8201', name: 'штыковая', quantity: 300 },
    ]
    const result = await distributeBudget(rows, 3000, access, {
      evaluate: customs,
      productOf,
      productPrices: { '8201|штыковая': 1.15, '8201|совковая': 1.1, '7310|бочка': 12 },
    })
    const price = (index: number) => Number(result.items[index].price)
    expect(result.status).toBe('ok')
    // Строки 1 и 5, 2 и 4 — один товар: одна цена.
    expect(result.items[0].price).toBe(result.items[4].price)
    expect(result.items[1].price).toBe(result.items[3].price)
    // Внутри кода — пропорция ИИ, а не одна цена.
    expect(price(0) / price(1)).toBeCloseTo(1.15 / 1.1, 3)
    expect(await customs(result.items)).toBeCloseTo(result.total, 2)
  })

  it('prices another product of a priced code from the row with a price', async () => {
    const result = await distributeBudget([
      { code: '8201', name: 'штыковая', quantity: 100, price: '2.3' },
      { code: '8201', name: 'совковая', quantity: 100 },
      { code: '8201', name: 'штыковая', quantity: 40 },
      { code: '7310', name: 'бочка', quantity: 50 },
    ], 2000, access, {
      evaluate: cost,
      productOf,
      productPrices: { '8201|штыковая': 1.15, '8201|совковая': 1.1, '7310|бочка': 12 },
    })
    // Тот же товар — цена строки с ценой; другой товар — в пропорции ИИ к ней: 2.3 × 1.1 / 1.15.
    expect(result.items.map((row) => row.price).slice(0, 3)).toEqual(['2.3', '2.2', '2.3'])
    expect(result.status).toBe('ok')
  })
})
