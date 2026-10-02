import { describe, expect, it } from 'vitest'
import { decimalText, moneyText, roundHalfUp, roundWeightKg, weightText } from './rounding'

describe('rounding by declaration rules', () => {
  it('rounds money mathematically, unlike toFixed', () => {
    expect((1.005).toFixed(2)).toBe('1.00')
    expect(moneyText(1.005)).toBe('1.01')
    expect(moneyText(2.675)).toBe('2.68')
    expect(moneyText(1234.5649)).toBe('1234.56')
    expect(moneyText(-1.005)).toBe('-1.01')
    expect(moneyText(123456789.125)).toBe('123456789.13')
    expect(roundHalfUp(0.335 * 3, 2)).toBe(1.01)
  })

  it('rounds weight to 3 decimals, below one gram to 6', () => {
    expect(weightText(3073.8)).toBe('3073.800')
    expect(weightText(12.3455)).toBe('12.346')
    expect(weightText(0.0004567)).toBe('0.000457')
    expect(weightText(0.0009996)).toBe('0.001')
    expect(weightText(3073.8, true)).toBe('3073.8')
    expect(roundWeightKg(0.1 + 0.2)).toBe(0.3)
  })

  it('keeps unit price precision without trailing zeros', () => {
    expect(decimalText(0.05135, 4)).toBe('0.0514')
    expect(decimalText(2, 4)).toBe('2')
  })
})
