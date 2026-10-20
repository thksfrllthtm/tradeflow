import { describe, expect, it } from 'vitest'
import { normalizeNumberInput } from './NumberField'

describe('normalizeNumberInput', () => {
  it('accepts a comma or a dot and drops thousands spaces', () => {
    expect(normalizeNumberInput('66 974,8')).toBe('66974.8')
    expect(normalizeNumberInput('452.59')).toBe('452.59')
    expect(normalizeNumberInput('9\u00a0500')).toBe('9500')
  })
})
