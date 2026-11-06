import { describe, expect, it } from 'vitest'
import { footerGap } from './footerGap'

describe('footerGap', () => {
  it('moves the totals row down only into free space', () => {
    expect(footerGap(470, 198)).toBe(272)
    expect(footerGap(470, 470)).toBe(0)
    expect(footerGap(470, 900)).toBe(0)
  })
})
