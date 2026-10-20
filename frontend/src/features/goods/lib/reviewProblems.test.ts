import { describe, expect, it } from 'vitest'
import { emptyItem } from './rowActions'
import { problemChips, reviewProblems } from './reviewProblems'
import type { ReviewStatus } from '../components/ReviewRow/reviewStatus'

const item = (patch = {}) => ({ ...emptyItem('a', '0.3'), ...patch })
const status = (patch: Partial<ReviewStatus>): ReviewStatus => ({ tone: 'warn', source: 'none', label: '', issues: [], attention: true, noCode: false, withErrors: false, ...patch })

describe('reviewProblems', () => {
  it('row without a code is a code-cell error, and the duplicate local message is skipped', () => {
    expect(reviewProblems(item(), status({ tone: 'todo', noCode: true, issues: ['Нужен код ТН ВЭД'] }))).toEqual([
      { type: 'noCode', label: 'Нет кода', tone: 'error', column: 'code', text: 'Код ТН ВЭД не указан' },
    ])
  })

  it('maps weight, quantity and code messages to their cells', () => {
    const problems = reviewProblems(item({ code: '8305200000' }), status({ issues: [
      'нетто должно быть больше 0 и не больше брутто',
      'Количество товара не совпадает с местами × количества товара в одном месте',
      'Кода нет в номенклатуре ТН ВЭД ЕАЭС — проверьте код',
    ] }))
    expect(problems.map((problem) => [problem.type, problem.tone, problem.column])).toEqual([
      ['weight', 'error', 'netKg'],
      ['quantity', 'warn', 'pairs'],
      ['code', 'warn', 'code'],
    ])
  })

  it('excluded rows have no problems', () => {
    expect(reviewProblems(item({ excluded: true }), status({ noCode: true }))).toEqual([])
  })
})

describe('problemChips', () => {
  it('counts rows per type, errors first', () => {
    const quantity = { type: 'quantity', label: 'Количество не сходится', tone: 'warn' as const, column: 'pairs' as const, text: '' }
    const noCode = { type: 'noCode', label: 'Нет кода', tone: 'error' as const, column: 'code' as const, text: '' }
    expect(problemChips([[quantity], [quantity, noCode], []])).toEqual([
      { type: 'noCode', label: 'Нет кода', tone: 'error', rows: 1 },
      { type: 'quantity', label: 'Количество не сходится', tone: 'warn', rows: 2 },
    ])
  })
})
