import { describe, expect, it } from 'vitest'
import { sameAnswer } from './answerText'

const answer = 'Универсальный код — 6307 10 100 0 (салфетки/тряпки для уборки из микрофибры, трикотажные машинного или ручного вязания). Если материал окажется нетканым — 6307 10 300 0, если тканым (не вязаным) — 6307 10 900 0.'

describe('sameAnswer', () => {
  it('дословный повтор и повтор с другой пунктуацией — тот же ответ', () => {
    expect(sameAnswer(answer, answer)).toBe(true)
    expect(sameAnswer(answer.replace(/—/g, '-').toUpperCase(), answer)).toBe(true)
  })
  it('короткий пересказ и другой ответ — не повтор', () => {
    expect(sameAnswer('Основной код — 6307 10 100 0 (салфетки для уборки из микрофибры, трикотажные).', answer)).toBe(false)
    expect(sameAnswer('Да, латексное покрытие код не меняет: ветка та же.', answer)).toBe(false)
    expect(sameAnswer('', answer)).toBe(false)
  })
})
