import { describe, expect, it } from 'vitest'
import { GoodsCalculationResult } from '@/shared/types/goods'
import { commonProblems, problemGroups, rowProblems } from './errorGroups'

const noCode = { errors: ['Не выбрана карточка ТН ВЭД: укажите код ТН ВЭД в строке', 'Укажите цену за единицу товара'], warnings: ['Не выбраны ставки: укажите код ТН ВЭД'] }

describe('rowProblems', () => {
  it('merges card and rate messages into one code problem and marks a missing price as a next step', () => {
    expect(rowProblems(noCode, '')).toEqual([
      { kind: 'code', tone: 'error', text: 'Не указан код ТН ВЭД' },
      { kind: 'price', tone: 'todo', text: 'Не указана цена' },
    ])
  })
})

describe('problemGroups', () => {
  it('counts rows per problem, errors first, and keeps only global blocking errors', () => {
    const result = { items: [noCode, noCode, { errors: [], warnings: ['Вес нетто больше брутто'] }, { ...noCode, excluded: true }],
      totals: { blockingErrors: ['Не выбрана карточка ТН ВЭД: укажите код ТН ВЭД в строке', 'Нет курса USD'] } } as unknown as GoodsCalculationResult
    expect(problemGroups(result, ['', '', '1234567890', '']).map((group) => [group.text, group.tone, group.rows])).toEqual([
      ['Не указан код ТН ВЭД', 'error', [1, 2]],
      ['Нет курса USD', 'error', []],
      ['Не указана цена', 'todo', [1, 2]],
      ['Вес нетто больше брутто', 'warn', [3]],
    ])
  })
})

describe('commonProblems', () => {
  it('keeps problems most rows share, so a row shows only what differs', () => {
    expect([...commonProblems([['код', 'цена'], ['код', 'цена'], ['код'], []])]).toEqual(['код'])
    expect([...commonProblems([['код', 'вес'], ['код'], ['код']])]).toEqual(['код'])
    // Одна строка из многих с проблемой — не общая: у неё и значок, и текст.
    expect([...commonProblems([['вес'], [], [], []])]).toEqual([])
  })
})

describe('SIR fallback', () => {
  it('is not a row problem, and codes in server texts are formatted', () => {
    expect(rowProblems({ warnings: ['СУР: для кода 8716903000 и страны CN индикатора нет — ТС по инвойсу'] }, '8716903000')).toEqual([])
    expect(rowProblems({ warnings: ['Ставка для 8716903000 не найдена'] }, '8716903000')[0].text).toBe('Ставка для 8716 90 300 0 не найдена')
  })
})
