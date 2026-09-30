import { describe, expect, it } from 'vitest'
import { checkCodes, checkNomenclature, parsePastedCodes } from './pastedCodes'
import { NomenclatureCode } from '../nomenclature/nomenclatureApi'
import { TNVEDDirectoryEntry } from './types'

const entry = (id: number, code: string, archivedAt = '') => ({ id, code, archivedAt }) as TNVEDDirectoryEntry

describe('parsePastedCodes', () => {
  it('reads an Excel column, counts repeats and skips the header', () => {
    const result = parsePastedCodes('Код ТН ВЭД\r\n4015190000\r\n6116102000\r\n6116102000\r\n\r\n4203291000\r\n')
    expect(result.rows).toBe(4)
    expect(result.codes).toEqual([
      { code: '4015190000', count: 1, padded: false },
      { code: '6116102000', count: 2, padded: false },
      { code: '4203291000', count: 1, padded: false },
    ])
    expect(result.invalid).toEqual([])
  })

  it('accepts spaced codes, restores a lost leading zero and reports wrong lengths', () => {
    const result = parsePastedCodes('4015 19 000 0\t101210000\n12345\n12345')
    expect(result.codes).toEqual([
      { code: '4015190000', count: 1, padded: false },
      { code: '0101210000', count: 1, padded: true },
    ])
    expect(result.invalid).toEqual(['12345'])
  })
})

describe('checkCodes', () => {
  it('puts missing codes first and tells archived-only codes apart', () => {
    const { codes } = parsePastedCodes('1111111111\n2222222222\n3333333333')
    const rows = checkCodes(codes, [entry(1, '1111111111'), entry(2, '3333333333', '2026-01-01'), entry(3, '1111111111', '2026-01-01')])
    expect(rows.map((row) => [row.code, row.status, row.entries.map((item) => item.id)])).toEqual([
      ['2222222222', 'missing', []],
      ['3333333333', 'archived', [2]],
      ['1111111111', 'found', [1]],
    ])
  })

  it('keeps the directory check about the directory, the nomenclature only fills the new card', () => {
    const { codes } = parsePastedCodes('1111111111\n2222222222\n4015190000')
    const official = [{ code: '4015190000', name: 'перчатки: 🠺 прочие' }] as NomenclatureCode[]
    const rows = checkCodes(codes, [entry(1, '1111111111')], official)
    expect(rows.map((row) => [row.code, row.status, row.nomenclature?.name ?? null])).toEqual([
      ['2222222222', 'missing', null],
      ['4015190000', 'missing', 'перчатки: 🠺 прочие'],
      ['1111111111', 'found', null],
    ])
  })
})

describe('checkNomenclature', () => {
  it('puts codes the EAEU nomenclature does not have first', () => {
    const { codes } = parsePastedCodes('4015190000\n1111111111\n4203291000')
    const found = [{ code: '4015190000', name: 'перчатки: 🠺 прочие' }, { code: '4203291000', name: 'защитные' }] as NomenclatureCode[]
    expect(checkNomenclature(codes, found).map((row) => [row.code, row.status, row.entries.length])).toEqual([
      ['1111111111', 'unknown', 0],
      ['4015190000', 'official', 0],
      ['4203291000', 'official', 0],
    ])
  })
})
