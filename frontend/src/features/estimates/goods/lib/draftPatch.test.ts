import { describe, expect, it } from 'vitest'
import { GlovesCalculationResult, GlovesItemInput } from '../../shared/types'
import { baselineOf, changesSince, DraftState, requestsFor } from './draftPatch'
import { emptyParams } from './draftStorage'

const row = (id: string, photos: string[] = [], name = id) => ({ id, name, photoUrl: photos[0] ?? '', photoUrls: photos } as unknown as GlovesItemInput)
const state = (overrides: Partial<DraftState> = {}): DraftState => ({ params: { ...emptyParams(), usdKzt: '450' }, items: [row('r1', ['data:image/png;base64,AAA']), row('r2')], result: null, source: null, ...overrides })

describe('changesSince', () => {
  it('finds nothing when the draft equals the saved one', () => {
    expect(changesSince(baselineOf(state()), state())).toBeNull()
  })

  it('sends only the changed parameter, without rows and photos', () => {
    const saved = state()
    const changes = changesSince(baselineOf(saved), { ...saved, params: { ...saved.params, usdKzt: '455' } })
    expect(changes?.input).toEqual({ usdKzt: '455' })
    expect(changes?.result).toBeUndefined()
    expect(changes?.photos).toEqual({})
  })

  it('sends rows without photos, and photos only of the changed row', () => {
    const saved = state()
    const items = [row('r1', ['data:image/png;base64,AAA'], 'Лопата штыковая'), row('r2', ['data:image/png;base64,BBB'])]
    const changes = changesSince(baselineOf(saved), { ...saved, items })
    const sentRows = changes?.input.items as Record<string, unknown>[]
    expect(sentRows.map((item) => item.name)).toEqual(['Лопата штыковая', 'r2'])
    expect(sentRows.every((item) => !('photoUrl' in item) && !('photoUrls' in item))).toBe(true)
    // Фото строки r1 не менялось — не отправляется.
    expect(changes?.photos).toEqual({ r2: ['data:image/png;base64,BBB'] })
  })

  it('sends a new result and null when the result is gone', () => {
    const result = { items: [], totals: {} } as unknown as GlovesCalculationResult
    const saved = state()
    expect(changesSince(baselineOf(saved), { ...saved, result })?.result).toBe(result)
    expect(changesSince(baselineOf({ ...saved, result }), saved)?.result).toBeNull()
  })

  it('sends everything when nothing is saved yet', () => {
    const changes = changesSince({ input: {}, result: '', photos: new Map() }, state())
    expect(Object.keys(changes?.input ?? {})).toEqual(expect.arrayContaining(['usdKzt', 'items', 'source']))
    expect(changes?.photos).toEqual({ r1: ['data:image/png;base64,AAA'] })
  })
})

describe('requestsFor', () => {
  it('puts fields and result first and splits big photos into several requests', () => {
    const big = 'data:image/png;base64,' + 'A'.repeat(600)
    const saved = state({ items: [row('r1'), row('r2'), row('r3')] })
    const changes = changesSince(baselineOf(saved), { ...saved, params: { ...saved.params, usdKzt: '1' }, items: [row('r1', [big]), row('r2', [big]), row('r3', [big])] })
    const requests = requestsFor(changes!, 1000)
    expect(requests[0].input?.usdKzt).toBe('1')
    expect(requests.length).toBeGreaterThan(1)
    expect(requests.flatMap((request) => Object.keys(request.photos ?? {})).sort()).toEqual(['r1', 'r2', 'r3'])
  })
})
