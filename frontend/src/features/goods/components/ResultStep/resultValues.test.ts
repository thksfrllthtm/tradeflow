import { describe, expect, it } from 'vitest'
import { excelColumn, textColumn } from './resultValues'

describe('excelColumn', () => {
  it('gives one Excel-ready number per line', () => {
    expect(excelColumn(['0.0444', '2 664.00', 1298.7, undefined, '', '1\u00a0552.5'])).toBe('0.0444\n2664.00\n1298.7\n\n\n1552.5')
  })
})

describe('textColumn', () => {
  it('keeps each name in one cell', () => {
    expect(textColumn(['Плитка\nнапольная\t60×60 ', '', 'Обувь'])).toBe('Плитка напольная 60×60\n\nОбувь')
  })
})
