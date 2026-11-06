import { describe, expect, it } from 'vitest'
import { GoodsCalculationResult } from '@/shared/types/goods'
import { excelColumn, excelColumnHtml } from '@/shared/lib/excelColumn'
import { calculatedRows, fixedNumber, itemNames, rowPaymentKzt, textColumn } from './resultValues'

describe('excelColumnHtml', () => {
  it('passes the exact number in x:num so Excel separators do not matter', () => {
    const html = excelColumnHtml(['1.3272', '2 664.00', undefined, '<b>'])
    expect(html).toContain('xmlns:x="urn:schemas-microsoft-com:office:excel"')
    expect(html).toContain('<tr><td x:num="1.3272">1,3272</td></tr><tr><td x:num="2664.00">2664,00</td></tr><tr><td></td></tr><tr><td></td></tr>')
  })
})

describe('excelColumn', () => {
  it('gives one Excel-ready number per line', () => {
    expect(excelColumn(['0.0444', '2 664.00', 1298.7, undefined, '', '1\u00a0552.5'])).toBe('0,0444\n2664,00\n1298,7\n\n\n1552,5')
  })
})

describe('textColumn', () => {
  it('keeps each name in one cell', () => {
    expect(textColumn(['Плитка\nнапольная\t60×60 ', '', 'Обувь'])).toBe('Плитка напольная 60×60\n\nОбувь')
  })
})

describe('rowPaymentKzt', () => {
  it('sums duty, anti-dumping, VAT and fee exactly', () => {
    expect(rowPaymentKzt({ dutyKzt: '43375.86', antiDumpingKzt: '', vatKzt: '84191.18', feeKzt: '865.00' })).toBe('128432.04')
    expect(rowPaymentKzt({ dutyKzt: '0.1', vatKzt: '0.2' })).toBe('0.30')
  })

  it('is empty for excluded or not calculated rows', () => {
    expect(rowPaymentKzt({ excluded: true, dutyKzt: '1' })).toBe('')
    expect(rowPaymentKzt({ dutyKzt: '', vatKzt: '' })).toBe('')
    expect(rowPaymentKzt({ errors: ['Укажите цену за единицу товара'], dutyKzt: '0', vatKzt: '0', feeKzt: '540.63' })).toBe('')
  })
})

describe('itemNames', () => {
  it('splits a chosen product into Russian and Chinese names', () => {
    expect(itemNames({ directorySnapshot: { name: 'Топоры', nameZh: '斧頭刀', productDirectoryId: 5 } })).toEqual({ main: 'Топоры', sub: '斧頭刀' })
  })

  it('shows the Excel name under a renamed row', () => {
    expect(itemNames({}, { name: 'Скобы для степлера', supplierName: '订书钉' })).toEqual({ main: 'Скобы для степлера', sub: '订书钉' })
    expect(itemNames({}, { name: 'Обувь', supplierName: 'Обувь' })).toEqual({ main: 'Обувь', sub: '' })
  })
})

describe('fixedNumber', () => {
  it('shows a fixed number of decimals through the shared formatter', () => {
    expect(fixedNumber('2', 2)).toBe('2,00')
    expect(fixedNumber('4.5', 2)).toBe('4,50')
    expect(fixedNumber('20230', 1)).toBe('20 230,0')
    expect(fixedNumber('1424.756', 1)).toBe('1 424,8')
    expect(fixedNumber('', 1)).toBe('—')
  })
})

describe('calculatedRows', () => {
  it('counts rows priced by invoice for lack of a risk indicator as calculated', () => {
    const result = { totals: { calculatedCount: 28 }, items: [
      { status: 'needs_review', errors: [], warnings: ['СУР: для кода 8716903000 и страны CN индикатора нет — ТС по инвойсу'] },
      { status: 'needs_review', errors: ['Укажите цену за единицу товара'], warnings: [] },
    ] } as unknown as GoodsCalculationResult
    expect(calculatedRows(result)).toBe(29)
  })
})
