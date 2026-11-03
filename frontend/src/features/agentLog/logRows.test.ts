import { describe, expect, it } from 'vitest'
import { comparison, costText, durationText, isoDaysAgo, metaText, modelOutcome, percent, rangeText } from './logRows'

const model = (status: string, bestCode = '', costUsd = '0', seconds: number | null = null, steps = 0) => ({ status, model: 'm', bestCode, costUsd, steps, seconds })

describe('журнал агента', () => {
  it('итог модели: код, иначе причина', () => {
    expect(modelOutcome(null)).toEqual({ text: '—', tone: 'muted' })
    expect(modelOutcome(model('done', '6116102000'))).toEqual({ text: '6116 10 200 0', tone: 'ok' })
    expect(modelOutcome(model('done'))).toEqual({ text: 'без итога', tone: 'warn' })
    expect(modelOutcome(model('skipped')).text).toBe('пропущено')
    expect(modelOutcome(model('error')).tone).toBe('bad')
    expect(modelOutcome(model('running')).text).toBe('идёт')
  })

  it('сравнение: согласие всегда, правота только по проверенной записи', () => {
    const verified = { id: 1, code: '6116102000', status: 'verified' as const, agentCorrect: true }
    expect(comparison({ agree: true, mainCorrect: true, shadowCorrect: false, decision: verified, documentsAgree: null }).map((mark) => mark.text))
      .toEqual(['код совпал', 'основная права', 'тень ошиблась'])
    expect(comparison({ agree: false, mainCorrect: false, shadowCorrect: true, decision: { ...verified, status: 'pending' }, documentsAgree: false }).map((mark) => mark.text))
      .toEqual(['код разошёлся', 'документы разошлись'])
    expect(comparison({ agree: null, mainCorrect: null, shadowCorrect: null, decision: null, documentsAgree: true }).map((mark) => mark.text)).toEqual(['документы совпали'])
    expect(comparison({ agree: null, mainCorrect: null, shadowCorrect: null, decision: null, documentsAgree: null })).toEqual([])
  })

  it('доля, время, стоимость и диапазон', () => {
    expect(percent(1, 3)).toBe('33%')
    expect(percent(0, 0)).toBe('—')
    expect(durationText(42)).toBe('42 с')
    expect(durationText(102.4)).toBe('1 мин 42 с')
    expect(durationText(null)).toBe('—')
    expect(costText('0.5369')).toBe('$0.54')
    expect(costText('0.0046')).toBe('$0.0046')
    expect(costText('0.0000')).toBe('$0')
    expect(metaText(model('done', '6116102000', '0.5369', 128, 40))).toBe('$0.54 · 2 мин 8 с · 40 ш.')
    expect(rangeText(1, 50, 36)).toBe('1–36 из 36')
    expect(rangeText(3, 50, 136)).toBe('101–136 из 136')
    expect(rangeText(1, 50, 0)).toBe('нет подборов')
    expect(isoDaysAgo(29, new Date(2026, 10, 3))).toBe('2026-10-05')
    expect(isoDaysAgo(0, new Date(2026, 0, 9))).toBe('2026-01-09')
  })
})
