import { describe, expect, it } from 'vitest'
import { estimateDisplayTitle } from './estimateTitle'

describe('estimateDisplayTitle', () => {
  it('shows the date in the name as dd.mm.yyyy', () => {
    expect(estimateDisplayTitle('Смета товаров · 2026-10-14')).toBe('Смета товаров · 14.10.2026')
    expect(estimateDisplayTitle('Смета №78')).toBe('Смета №78')
  })
})
