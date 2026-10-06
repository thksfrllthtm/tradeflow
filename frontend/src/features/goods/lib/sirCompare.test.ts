import { describe, expect, it } from 'vitest'
import { GoodsCalculationResult } from '@/shared/types/goods'
import { compareCounts, compareRows, compareTotals } from './sirCompare'

type Item = GoodsCalculationResult['items'][number]
const item = (values: Partial<Item>) => ({ excluded: false, netKg: '', grossKg: '', costUsd: '', customsValueKzt: '', ...values }) as Item
const result = (items: Item[], totals: Partial<GoodsCalculationResult['totals']>) =>
  ({ usdKzt: '500', items, totals: { customsValueKzt: '0', dutyKzt: '0', antiDumpingKzt: '0.00', vatKzt: '0', feeKzt: '0', customsPaymentsKzt: '0', ...totals } }) as GoodsCalculationResult

const sir = (priceUsd: string, pricePerKgUsd: string, weightKg: string) => ({ profile: '50100/190318/00024', country: 'CN', priceUsd, discountPercent: '60', pricePerKgUsd, weight: 'net' as const, weightKg, valueUsd: '' })

describe('сравнение по инвойсу и по СИР', () => {
  const invoice = result([
    item({ netKg: '1580', grossKg: '1680', costUsd: '927.60', customsValueKzt: '422039.45' }),
    item({ netKg: '100', costUsd: '500.00', customsValueKzt: '250000.00' }),
    item({ excluded: true }),
    item({ netKg: '10', costUsd: '', customsValueKzt: '1000.00' }),
  ], { customsValueKzt: '673039.45', dutyKzt: '60000', vatKzt: '120000', feeKzt: '25950', customsPaymentsKzt: '205950.00' })
  const byIndicator = result([
    item({ customsValueKzt: '603849.46', sir: sir('2.1', '0.84', '1580'), customsBasis: 'sir' }),
    item({ customsValueKzt: '250000.00' }),
    item({ excluded: true }),
    item({ customsValueKzt: '1000.00' }),
  ], { customsValueKzt: '854849.46', dutyKzt: '76000', vatKzt: '150000', feeKzt: '25950', customsPaymentsKzt: '251950.00' })

  it('строки: цена за кг, метка «ниже СИР», разница ТС; исключённые не показываются', () => {
    const rows = compareRows(invoice, byIndicator, 'net')
    expect(rows.map((row) => row.index)).toEqual([0, 1, 3])
    expect(rows[0].invoicePerKg).toBeCloseTo(0.5871, 4)
    expect(rows[0]).toMatchObject({ sirPerKg: 2.1, sirDiscountedPerKg: 0.84, belowSir: true, weightKg: 1580 })
    expect(rows[0].customsDiff).toBeCloseTo(181810.01, 2)
    // Индикатора нет — строка по инвойсу, без метки; цены нет — цена за кг неизвестна.
    expect(rows[1]).toMatchObject({ sirPerKg: null, belowSir: false, invoicePerKg: 5, customsDiff: 0 })
    expect(rows[2]).toMatchObject({ invoicePerKg: null, belowSir: false })
    expect(compareCounts(rows)).toEqual({ sir: 1, below: 1, total: 3 })
  })

  it('итоги: разница по каждой строке, антидемпинг только если он есть, растаможка в USD', () => {
    const lines = compareTotals(invoice, byIndicator)
    expect(lines.map((line) => line.label)).toEqual(['Таможенная стоимость', 'Пошлина', 'НДС', 'Сбор', 'Растаможка к оплате', 'Растаможка к оплате, USD'])
    expect(lines[4]).toMatchObject({ invoice: 205950, sir: 251950, diff: 46000, total: true })
    expect(lines[5]).toMatchObject({ invoice: 411.9, sir: 503.9 })
    expect(lines[5].diff).toBeCloseTo(92, 6)
  })
})
