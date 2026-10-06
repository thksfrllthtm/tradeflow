import { describe, expect, it } from 'vitest'
import { checkNomenclature, parsePastedCodes } from './pastedCodes'
import { NomenclatureCode } from '@/shared/tnved/nomenclature'

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

describe('checkNomenclature', () => {
  it('puts codes the EAEU nomenclature does not have first', () => {
    const { codes } = parsePastedCodes('4015190000\n1111111111\n4203291000')
    const found = [{ code: '4015190000', name: 'перчатки: 🠺 прочие' }, { code: '4203291000', name: 'защитные' }] as NomenclatureCode[]
    expect(checkNomenclature(codes, found).map((row) => [row.code, row.status])).toEqual([
      ['1111111111', 'unknown'],
      ['4015190000', 'official'],
      ['4203291000', 'official'],
    ])
  })
})
