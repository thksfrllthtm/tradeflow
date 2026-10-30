import { describe, expect, it } from 'vitest'
import { documentBadge, documentNotes } from './DocumentsAnswer'

describe('documentNotes', () => {
  it('по строке на акт; старый ответ одной строкой; пусто — нет', () => {
    expect(documentNotes('[№299] Контакт с пищей.\n[ТР ТС 019/2011] Только защитные.\n')).toEqual(['[№299] Контакт с пищей.', '[ТР ТС 019/2011] Только защитные.'])
    expect(documentNotes('Одно замечание.')).toEqual(['Одно замечание.'])
    expect(documentNotes(undefined)).toEqual([])
  })
})

describe('documentBadge', () => {
  it('коротко: вид документа или санэпиднадзор', () => {
    expect(documentBadge({ kind: 'cc', measure: 'Сертификат соответствия ТР ТС 010/2011 «О безопасности машин и оборудования»' })).toBe('СС')
    expect(documentBadge({ kind: 'other', measure: 'Санитарно-эпидемиологический надзор (раздел I, без СГР)' })).toBe('Санэпиднадзор')
    expect(documentBadge({ measure: 'санитарно-эпидемиологический надзор' })).toBe('Санэпиднадзор')
    expect(documentBadge({ kind: 'sgr', measure: 'Санитарно-эпидемиологический надзор (раздел II, СГР)' })).toBe('СГР')
    expect(documentBadge({ kind: 'other', measure: 'Ветеринарный контроль' })).toBe('')
    expect(documentBadge({ kind: 'other', measure: 'Расширенные обязательства производителей (импортеров) — утилизация отходов (РОП)', utilFee: 'за упаковку' })).toBe('Утильсбор за упаковку')
    expect(documentBadge({ kind: 'other', measure: 'Утилизационный платёж: раздел 2' })).toBe('Утильсбор')
  })
})
