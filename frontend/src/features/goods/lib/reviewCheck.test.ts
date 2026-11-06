import { describe, expect, it } from 'vitest'
import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'
import { reviewStatus } from './reviewStatus'
import { checkFromResult, loadReviewCheck, saveReviewCheck } from './reviewCheck'

const row = (id: string, grossKg = '10') => ({ id, code: '4015190000', grossKg, pricePerPair: '1', excluded: false, directorySnapshot: { rateSource: 'nomenclature' } }) as unknown as GoodsItemInput
const result = (...errors: string[][]) => ({ items: errors.map((list) => ({ errors: list, warnings: [], status: list.length ? 'error' : 'calculated' })), updatedAt: '2026-10-05T09:00:00Z' }) as unknown as GoodsCalculationResult

describe('review check', () => {
  const items = [row('a'), row('b', ''), row('c', '')]
  const check = checkFromResult(7, items, result([], ['Укажите брутто'], ['Укажите брутто']))

  it('keeps errors of other rows when one row is edited, and does not call it fixed', () => {
    const edited = [items[0], { ...items[1], grossKg: '12' }, items[2]]
    const b = reviewStatus(edited[1], undefined, check.rows.b)
    const c = reviewStatus(edited[2], undefined, check.rows.c)
    expect(b.tone).toBe('changed')
    expect(b.label).toBe('Изменено — требуется пересчёт')
    expect(b.attention).toBe(true)
    expect(c).toMatchObject({ tone: 'bad', source: 'past', attention: true })
    expect(reviewStatus(edited[0], undefined, check.rows.a).tone).toBe('ok')
  })

  it('follows rows by id after deletion, not by position', () => {
    const withoutA = [items[1], items[2]]
    expect(withoutA.map((item) => reviewStatus(item, undefined, check.rows[item.id ?? '']).tone)).toEqual(['bad', 'bad'])
    expect(reviewStatus(items[0], undefined, check.rows.a).tone).toBe('ok')
  })

  it('ignores price edits and treats a fresh result as the current check', () => {
    expect(reviewStatus({ ...items[0], pricePerPair: '2' }, undefined, check.rows.a).tone).toBe('ok')
    expect(reviewStatus(items[1], result(['Укажите брутто']).items[0], check.rows.b)).toMatchObject({ tone: 'bad', source: 'current' })
  })

  it('stores the copy per estimate', () => {
    const store = new Map<string, string>()
    globalThis.localStorage = { getItem: (key: string) => store.get(key) ?? null, setItem: (key: string, value: string) => void store.set(key, value), removeItem: (key: string) => void store.delete(key) } as Storage
    saveReviewCheck(check, 7)
    expect(loadReviewCheck(7)?.rows.b.errors).toEqual(['Укажите брутто'])
    expect(loadReviewCheck(8)).toBeNull()
    saveReviewCheck(null, 7)
    expect(loadReviewCheck(7)).toBeNull()
  })
})
