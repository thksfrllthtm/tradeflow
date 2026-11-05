import { describe, expect, it } from 'vitest'
import { formatRuDate, savedAtText } from './dates'

describe('savedAtText', () => {
  it('shows only the time for today and the date otherwise', () => {
    const now = new Date(2026, 9, 20, 15, 0)
    expect(savedAtText(new Date(2026, 9, 20, 14, 30), now)).toBe('Сохранено в 14:30')
    expect(savedAtText(new Date(2026, 9, 19, 9, 5), now)).toBe('Сохранено 19.10.2026 в 09:05')
  })
})

describe('formatRuDate', () => {
  it('accepts a plain date and a full timestamp', () => {
    expect(formatRuDate('2026-09-21')).toBe('21.09.2026')
    expect(formatRuDate('2026-09-21T14:30:00.123456+05:00')).toBe('21.09.2026')
  })
})
