import { describe, expect, it } from 'vitest'
import { excelColumn, excelColumnHtml, textColumn } from './resultValues'

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
