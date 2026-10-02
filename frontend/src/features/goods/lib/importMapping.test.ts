import { describe, expect, it } from 'vitest'
import { buildGloveItems, emptyMapping } from './importMapping'

const row = (sourceRow: number, values: Record<string, string>) => ({ sourceRow, included: true, values })

describe('несколько колонок наименования', () => {
  it('первое непустое — наименование, остальные — дополнительные', () => {
    const mapping = { ...emptyMapping(), name: '名称', name2: 'НАИМЕНОВАНИЕ' }
    const [first, second, third] = buildGloveItems([
      row(2, { '名称': '桶红本色头，0.7公斤', 'НАИМЕНОВАНИЕ': '0,7 Лопата оран штыковой B-1' }),
      row(3, { '名称': '', 'НАИМЕНОВАНИЕ': '0,7 Лопата оран совковый B-2' }),
      row(4, { '名称': 'Лопата', 'НАИМЕНОВАНИЕ': 'Лопата' }),
    ], [], mapping, '0', 0)
    expect(first.name).toBe('桶红本色头，0.7公斤')
    expect(first.otherNames).toEqual(['0,7 Лопата оран штыковой B-1'])
    expect(second.name).toBe('0,7 Лопата оран совковый B-2')
    expect(second.otherNames).toBeUndefined()
    expect(third.otherNames).toBeUndefined()
  })
})
