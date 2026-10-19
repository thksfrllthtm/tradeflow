import { describe, expect, it } from 'vitest'
import { BudgetRowAccess, distributeBudget } from './distributeBudget'
import { parseGap } from '../hooks/useBudgetDistribution'

type Row = { code: string; quantity: number; price?: string }
const access: BudgetRowAccess<Row> = {
  getCode: (row) => row.code,
  getQuantity: (row) => row.quantity,
  getPrice: (row) => row.price ? Number(row.price) : null,
  setPrice: (row, price) => ({ ...row, price }),
}
// Растаможка: пошлина 10%, НДС 12% на стоимость и пошлину, сбор 60 USD.
const customs = async (rows: Row[]) => 60 + rows.reduce((sum, row) => {
  const value = Number(row.price ?? 0) * row.quantity
  return sum + value * 0.1 + value * 1.1 * 0.12
}, 0)
const rows: Row[] = [{ code: 'A', quantity: 800 }, { code: 'B', quantity: 1200 }]

describe('budget gap', () => {
  it('lands inside a smaller and a bigger gap than the default 35–45', async () => {
    for (const [minGap, maxGap] of [[5, 10], [100, 150], [0, 2]]) {
      const result = await distributeBudget(rows, 11000, access, { evaluate: customs, minGap, maxGap })
      expect(result.status).toBe('ok')
      expect(result.gap).toBeGreaterThanOrEqual(minGap)
      expect(result.gap).toBeLessThanOrEqual(maxGap)
    }
  })

  it('lands above the budget in the overspend mode: budget 11000, 20–30 → 11020–11030', async () => {
    const result = await distributeBudget(rows, 11000, access, { evaluate: customs, minGap: 20, maxGap: 30, over: true })
    expect(result.status).toBe('ok')
    expect(result.total).toBeGreaterThanOrEqual(11020)
    expect(result.total).toBeLessThanOrEqual(11030)
    expect(result.gap).toBeLessThanOrEqual(-20)
    // Перерасход не требует, чтобы бюджет был больше разницы.
    expect((await distributeBudget(rows, 25, access, { evaluate: customs, minGap: 20, maxGap: 30, over: true })).message).not.toContain('больше недостачи')
  })

  it('picks prices with two decimals when asked and still lands in 35–45', async () => {
    const result = await distributeBudget(rows, 11000, access, { evaluate: customs, decimals: 2 })
    expect(result.status).toBe('ok')
    expect(result.gap).toBeGreaterThanOrEqual(35)
    expect(result.gap).toBeLessThanOrEqual(45)
    result.items.forEach((row) => expect(row.price).toMatch(/^\d+(\.\d{1,2})?$/))
  })

  // 20 000 шт: шаг цены 0,01 меняет растаможку на 46,88 USD — в окно 35–45 не попасть
  // (рядом недостача 17,12 и 64,00). Промах — в бóльшую разницу, не в меньшую.
  it('misses towards a bigger shortfall when two decimals cannot land in the window', async () => {
    const result = await distributeBudget([{ code: 'A', quantity: 20000 }], 9500, access, { evaluate: customs, decimals: 2 })
    expect(result.status).toBe('closest')
    expect(result.gap).toBeGreaterThan(45)
    expect(result.gap).toBeLessThan(45 + 46.88)
  })

  it('misses towards a bigger overspend in the overspend mode', async () => {
    const result = await distributeBudget([{ code: 'A', quantity: 20000 }], 9500, access, { evaluate: customs, decimals: 2, over: true })
    expect(result.status).toBe('closest')
    expect(result.gap).toBeLessThan(-45)
    expect(result.gap).toBeGreaterThan(-45 - 46.88)
  })

  it('keeps evaluations for the rounding fix: a small group lands two decimals in the window', async () => {
    const result = await distributeBudget([{ code: 'A', quantity: 20000 }, { code: 'B', quantity: 300 }], 9500, access, { evaluate: customs, decimals: 2 })
    expect(result.status).toBe('ok')
    expect(result.evaluations).toBeLessThanOrEqual(14)
  })

  it('rejects a gap the budget cannot hold or a reversed gap', async () => {
    expect((await distributeBudget(rows, 100, access, { evaluate: customs, minGap: 150, maxGap: 200 })).message).toContain('больше недостачи')
    expect((await distributeBudget(rows, 11000, access, { evaluate: customs, minGap: 30, maxGap: 20 })).status).toBe('nothing')
  })

  it('parses the gap fields', () => {
    expect(parseGap('20', '30')).toEqual({ min: 20, max: 30 })
    expect(parseGap('5,5', '10')).toEqual({ min: 5.5, max: 10 })
    expect(parseGap('0', '0')).toEqual({ min: 0, max: 0 })
    expect(parseGap('30', '20')).toBeNull()
    expect(parseGap('', '30')).toBeNull()
    expect(parseGap('-5', '30')).toBeNull()
  })
})
