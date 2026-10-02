import { describe, expect, it } from 'vitest'
import { resolveWeights } from './budgetWeights'

describe('resolveWeights', () => {
  it('uses known prices as weights', () => {
    expect(resolveWeights([{ key: 'a', price: 2 }, { key: 'b', price: 5 }])).toEqual({ a: 2, b: 5 })
  })

  it('puts groups without a price on the price scale through the price per kg', () => {
    // a: 4 USD for 0.5 kg → 8 USD/kg; b has no price but weighs 0.25 kg → 2 USD.
    const weights = resolveWeights([{ key: 'a', price: 4, netPerUnit: 0.5 }, { key: 'b', netPerUnit: 0.25 }, { key: 'c' }])
    expect(weights.a).toBe(4)
    expect(weights.b).toBe(2)
    expect(weights.c).toBe(4)
  })

  it('falls back to the net weight of one unit, then to equal weights', () => {
    expect(resolveWeights([{ key: 'a', netPerUnit: 1.5 }, { key: 'b', netPerUnit: 0.5 }, { key: 'c' }])).toEqual({ a: 1.5, b: 0.5, c: 1 })
    expect(resolveWeights([{ key: 'a' }, { key: 'b', price: 0 }])).toEqual({ a: 1, b: 1 })
  })
})
