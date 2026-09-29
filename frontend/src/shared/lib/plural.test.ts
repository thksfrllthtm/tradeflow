import { describe, expect, it } from 'vitest'
import { plural } from './plural'

describe('plural', () => {
  it('picks the Russian word form by the last digits', () => {
    const word = (count: number) => plural(count, 'код', 'кода', 'кодов')
    expect([1, 2, 5, 11, 12, 14, 21, 22, 25, 111, 13293].map(word)).toEqual(['код', 'кода', 'кодов', 'кодов', 'кодов', 'кодов', 'код', 'кода', 'кодов', 'кодов', 'кода'])
  })
})
