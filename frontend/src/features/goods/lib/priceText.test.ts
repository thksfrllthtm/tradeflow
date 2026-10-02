import { describe, expect, it } from 'vitest'
import { priceText } from './priceText'

describe('priceText', () => {
  it('keeps up to four decimals without trailing zeros', () => {
    expect(priceText(4.164)).toBe('4.164')
    expect(priceText(4.17584)).toBe('4.1758')
    expect(priceText(4.1)).toBe('4.1')
    expect(priceText(4)).toBe('4')
    expect(priceText(0)).toBe('0')
  })
})
