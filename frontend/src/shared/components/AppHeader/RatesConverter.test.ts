import { describe, expect, it } from 'vitest'
import { amountText, convert } from './RatesConverter'

const rates = [{ currency: 'USD', rate: '447.14', date: '2026-10-08', source: 'НБРК' }, { currency: 'EUR', rate: '500.35', date: '2026-10-08', source: 'НБРК' }]

describe('convert', () => {
  it('пересчитывает через тенге по курсу НБРК', () => {
    expect(convert('1000', 'USD', 'KZT', rates)).toBeCloseTo(447140, 6)
    expect(convert('447 140', 'KZT', 'USD', rates)).toBeCloseTo(1000, 6)
    expect(convert('100,5', 'EUR', 'USD', rates)).toBeCloseTo(100.5 * 500.35 / 447.14, 6)
  })
  it('пустая сумма, текст или неизвестная валюта — нет результата', () => {
    expect(convert('', 'USD', 'KZT', rates)).toBeNull()
    expect(convert('abc', 'USD', 'KZT', rates)).toBeNull()
    expect(convert('10', 'CNY', 'KZT', rates)).toBeNull()
  })
})

describe('amountText', () => {
  it('от единицы — копейки, меньше единицы — до 6 знаков', () => {
    expect(amountText(447140)).toBe('447 140,00')
    expect(amountText(1 / 447.14)).toBe('0,002236')
    expect(amountText(0)).toBe('0,00')
  })
})
