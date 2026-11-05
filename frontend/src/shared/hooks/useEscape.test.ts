import { describe, expect, it } from 'vitest'
import { handleEscape, pushEscape } from './useEscape'

function press(key = 'Escape', defaultPrevented = false) {
  const event = { key, defaultPrevented, preventDefault() { event.defaultPrevented = true } }
  handleEscape(event)
  return event
}

describe('escape stack', () => {
  it('closes only the topmost window and skips already handled events', () => {
    const closed: string[] = []
    const removeDrawer = pushEscape(() => closed.push('drawer'))
    const removeModal = pushEscape(() => closed.push('modal'))

    expect(press().defaultPrevented).toBe(true)
    expect(closed).toEqual(['modal'])

    press('Escape', true)
    press('Enter')
    expect(closed).toEqual(['modal'])

    removeModal()
    press()
    expect(closed).toEqual(['modal', 'drawer'])

    removeDrawer()
    expect(press().defaultPrevented).toBe(false)
  })
})
