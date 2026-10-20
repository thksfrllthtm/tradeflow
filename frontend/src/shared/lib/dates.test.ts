import { describe, expect, it } from 'vitest'
import { savedAtText } from './dates'

describe('savedAtText', () => {
  it('shows only the time for today and the date otherwise', () => {
    const now = new Date(2026, 9, 20, 15, 0)
    expect(savedAtText(new Date(2026, 9, 20, 14, 30), now)).toBe('Сохранено в 14:30')
    expect(savedAtText(new Date(2026, 9, 19, 9, 5), now)).toBe('Сохранено 19.10.2026 в 09:05')
  })
})
