import { describe, expect, it } from 'vitest'
import { documentBadge } from './DocumentsAnswer'

describe('documentBadge', () => {
  it('коротко: вид документа или санэпиднадзор', () => {
    expect(documentBadge({ kind: 'cc', measure: 'Сертификат соответствия ТР ТС 010/2011 «О безопасности машин и оборудования»' })).toBe('СС')
    expect(documentBadge({ kind: 'other', measure: 'Санитарно-эпидемиологический надзор (раздел I, без СГР)' })).toBe('Санэпиднадзор')
    expect(documentBadge({ measure: 'санитарно-эпидемиологический надзор' })).toBe('Санэпиднадзор')
    expect(documentBadge({ kind: 'sgr', measure: 'Санитарно-эпидемиологический надзор (раздел II, СГР)' })).toBe('СГР')
    expect(documentBadge({ kind: 'other', measure: 'Ветеринарный контроль' })).toBe('')
  })
})
