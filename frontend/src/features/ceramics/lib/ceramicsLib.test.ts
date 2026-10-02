import { describe, expect, it } from 'vitest'
import { GoodsParseResult } from '@/shared/types/goods'
import { normalizeMark } from './marks'
import { buildGroups, findHeaderRow, guessMapping, PHOTO_FROM_IMAGES, resultCsv } from './matchGroups'
import { baseName, formatSize, parsePriceCell } from './sizes'

describe('normalizeMark', () => {
  it('matches the server key', () => {
    expect(normalizeMark(' 705-7dj ')).toBe('705-7DJ')
    expect(normalizeMark('７０５－７ＤＪ')).toBe('705-7DJ')
    expect(normalizeMark('705–7 DJ')).toBe('705-7DJ')
  })
})

describe('sizes and names', () => {
  it('formats sizes and base names', () => {
    expect(formatSize({ value: '7', unit: 'inch' })).toBe('7"')
    expect(formatSize({ value: '350', unit: 'ml' })).toBe('350 мл')
    expect(baseName('Керамическая фруктовница 7 дюймов для взрослых', '成年人陶瓷盘7高脚水果盘')).toBe('Керамическая фруктовница 7 дюймов для взрослых/ 成年人陶瓷盘7高脚水果盘')
    expect(baseName('Керамический чайник', '')).toBe('Керамический чайник')
  })
  it('reads prices', () => {
    expect(parsePriceCell('0,2')).toBe('0.2')
    expect(parsePriceCell('4,50')).toBe('4.5')
    expect(parsePriceCell('$5')).toBe('5')
    expect(parsePriceCell('¥5')).toBe('')
    expect(parsePriceCell('0.2 USD')).toBe('0.2')
    expect(parsePriceCell('1,234.5')).toBe('1234.5')
    expect(parsePriceCell('1 234,5')).toBe('1234.5')
    expect(parsePriceCell('0')).toBe('')
  })
})

describe('match groups', () => {
  const parsed: GoodsParseResult = {
    sheets: [], headers: ['Марка', '中文品名', 'Наименование'],
    rows: [
      { sourceRow: 2, included: true, warnings: [], values: { 'Марка': '705-7DJ', '中文品名': '成人陶瓷沙拉盘7寸', 'Наименование': 'Салатник 7"' } },
      { sourceRow: 3, included: true, warnings: [], values: { 'Марка': '705-7dj ', '中文品名': '成人陶瓷沙拉盘7寸', 'Наименование': '' } },
      { sourceRow: 4, included: true, warnings: [], values: { 'Марка': '', '中文品名': '陶瓷茶壶', 'Наименование': '' } },
      { sourceRow: 5, included: false, warnings: [], values: { 'Марка': 'Итого', '中文品名': '', 'Наименование': '' } },
      { sourceRow: 6, included: true, warnings: [], values: { 'Марка': '', '中文品名': '', 'Наименование': 'пусто' } },
    ],
    images: [{ sourceRow: 3, mimeType: 'image/png', data: 'AAAA' }],
  } as GoodsParseResult

  it('guesses columns', () => {
    expect(guessMapping(parsed.headers, true)).toEqual({ mark: 'Марка', chineseName: '中文品名', russianName: 'Наименование', photo: PHOTO_FROM_IMAGES })
  })

  it('merges repeated marks and skips empty rows', () => {
    const groups = buildGroups(parsed, guessMapping(parsed.headers, true))
    expect(groups).toHaveLength(2)
    expect(groups[0]).toMatchObject({ key: '705-7DJ', sourceRows: [2, 3], photo: 'data:image/png;base64,AAAA' })
    expect(groups[1]).toMatchObject({ key: 'zh:陶瓷茶壶', markKey: '' })
  })

  it('exports every source row', () => {
    const groups = buildGroups(parsed, guessMapping(parsed.headers, false))
    groups[0].draft = { productType: 'Керамическая тарелка салатница', sizeValue: '7', sizeUnit: 'inch', price: '4.5', name: 'Керамическая тарелка салатница 7 дюймов/ 成人陶瓷沙拉盘7寸' }
    const lines = resultCsv(groups).replace('﻿', '').split('\r\n')
    expect(lines).toHaveLength(4)
    expect(lines[1]).toBe('2;705-7DJ;成人陶瓷沙拉盘7寸;Керамическая тарелка салатница 7 дюймов/ 成人陶瓷沙拉盘7寸;4.5;Керамическая тарелка салатница;"7""";Ожидает')
  })
})

describe('findHeaderRow', () => {
  it('finds column names below a title row', () => {
    const parsed = {
      sheets: [], headers: ['P70-3817606-3474件', 'Колонка 2', 'Колонка 3'], images: [],
      rows: [
        { sourceRow: 2, included: true, values: { 'P70-3817606-3474件': '序号', 'Колонка 2': '型号', 'Колонка 3': '品名' } },
        { sourceRow: 3, included: true, values: { 'P70-3817606-3474件': '1', 'Колонка 2': '721-8', 'Колонка 3': '成年人陶瓷盘8高脚水果盘' } },
      ],
    } as GoodsParseResult
    expect(findHeaderRow(parsed)).toBe(2)
    expect(findHeaderRow({ ...parsed, rows: parsed.rows.slice(1) })).toBeNull()
  })
})

describe('guessMapping', () => {
  it('knows 代码 as the mark column', () => {
    expect(guessMapping(['代码', '品名', '产品图片', '数量/件'], true)).toMatchObject({ mark: '代码', chineseName: '品名' })
  })
  it('falls back to the cell contents when there are no column names', () => {
    const rows = [['6G', '6果', '500'], ['7G', '7果', '200'], ['9FD-010', '9水纹平盘', '200'], ['A004-1', '2号英式茶壶', '66']]
      .map((values, index) => ({ sourceRow: index + 2, included: true, values: { 'P70-3817606-3474件': values[0], 'Колонка 2': values[1], 'Колонка 4': values[2] } }))
    expect(guessMapping(['P70-3817606-3474件', 'Колонка 2', 'Колонка 4'], false, rows)).toMatchObject({ mark: 'P70-3817606-3474件', chineseName: 'Колонка 2' })
  })
})
