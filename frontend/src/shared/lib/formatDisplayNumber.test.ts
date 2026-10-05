import { describe, expect, it } from 'vitest'
import { formatDisplayNumber, formatRuNumber } from './formatDisplayNumber'

describe('formatRuNumber', () => {
  it('groups thousands and shows the fraction after a comma', () => {
    expect(formatRuNumber('1234567.5')).toBe('1 234 567,5')
    expect(formatRuNumber('-1234.00')).toBe('-1 234,00')
    expect(formatRuNumber('12')).toBe('12')
    expect(formatRuNumber('—')).toBe('—')
    expect(formatRuNumber('')).toBe('')
  })

  it('leaves the dot formatter unchanged', () => {
    expect(formatDisplayNumber('1234.5')).toBe('1 234.5')
  })
})
