import { describe, expect, it } from 'vitest'
import { stableJson } from './useSavedSnapshot'

describe('stableJson', () => {
  it('не зависит от порядка ключей (jsonb возвращает их по-своему)', () => {
    expect(stableJson({ b: 1, a: { d: [2, { y: 1, x: 2 }], c: 3 } })).toBe(stableJson({ a: { c: 3, d: [2, { x: 2, y: 1 }] }, b: 1 }))
    expect(stableJson(undefined)).toBe('null')
  })
})
