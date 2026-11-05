import { describe, expect, it } from 'vitest'
import type { AgentResult } from './agentApi'
import { brandShortText, brandText, documentsShortText, documentsText, resultShortText, resultText } from './answerCopy'

const siemens = { regNo: '01124/ТЗ-190219', name: 'SIEMENS', fromLogo: false, hasLogo: false, holder: 'Компания «Siemens Trademark GmbH & Co.KG»', goods: 'машины вычислительные',
  protectedUntil: '2026-12-20', kind: 'brand' as const, found: 'Siemens', sameGoods: false, otherGoods: true }

const answer: AgentResult = {
  description: 'Электронный модуль дискретного вывода для контроллера.', name: 'Модуль электронный дискретного вывода', notes: 'Сверьте артикул.',
  droppedCodes: [], quotesRemoved: 0, trademarks: [siemens],
  candidates: [
    { code: '8538909200', verdict: 'fits', quote: '', quoteSource: '', reason: 'Электронный модуль для аппаратуры 8537.', check: '', pageOpened: true,
      nomenclature: { code: '8538909200', name: '', tariffText: '0%', duty: { type: 'percent', percent: '0' }, vatPercent: '16', detailsUrl: '', kz: null } as never },
    { code: '8536908500', verdict: 'no', quote: '', quoteSource: '', reason: 'Не коммутация.', check: '', pageOpened: false, nomenclature: null },
  ],
}

describe('копирование ответа ИИ', () => {
  it('подбор кода: товар, код со ставками, наименование, примечание и ТРОИС; отвергнутые коды не пересылаются', () => {
    const text = resultText(answer)
    expect(text).toContain('Товар: Электронный модуль')
    expect(text).toContain('Код ТН ВЭД: 8538 90 920 0 — подходит\nПошлина 0%, НДС 16%\nЭлектронный модуль для аппаратуры 8537.')
    expect(text).toContain('Наименование для декларации: Модуль электронный дискретного вывода')
    expect(text).toContain('Примечание: Сверьте артикул.')
    expect(text).toContain('ТРОИС:\n• знак «SIEMENS»')
    expect(text).toContain('защита до 20.12.2026')
    expect(text).not.toContain('8536 90 850 0')
  })

  it('документы: нужно и возможно с требованием, не относящиеся — названиями', () => {
    const text = documentsText({ ...answer, trademarks: undefined, documentsCode: '8538909200', description: '', documents: [
      { kind: 'cc', measure: 'Сертификат соответствия ТР ТС 004/2011', act: 'Решение №91', url: '', requirement: 'Нужен сертификат.', applies: 'yes' },
      { kind: 'other', measure: 'Санэпиднадзор', act: 'Решение №299', url: '', requirement: 'Если контакт с водой.', applies: 'maybe' },
      { kind: 'other', measure: 'Лицензия на отходы', act: 'Решение №30', url: '', requirement: '', applies: 'no' },
    ] })
    expect(text).toContain('Документы для ввоза · 8538 90 920 0')
    expect(text).toContain('Нужно:\n• СС: Сертификат соответствия ТР ТС 004/2011 (Решение №91)\n  Нужен сертификат.')
    expect(text).toContain('Возможно, уточнить:\n• Санэпиднадзор (Решение №299)')
    expect(text).toContain('Не относятся к товару: Лицензия на отходы')
  })

  it('ТРОИС: бренды с фото и знаки; без знаков — что в реестре не найдено', () => {
    expect(brandText({ brands: ['Siemens'], pictures: [], hits: [siemens], compared: 0 })).toMatch(/^На фото: Siemens\n\nТРОИС:\n• знак «SIEMENS»/)
    expect(brandText({ brands: [], pictures: [], hits: [], compared: 0 })).toBe('В реестре ТРОИС по написанию не найдено.')
  })

  it("коротко: только код, наименование, ставки и есть ли знак; без обоснования и заметок", () => {
    expect(resultShortText(answer)).toBe('Код ТН ВЭД: 8538 90 920 0\nНаименование: Модуль электронный дискретного вывода\nПошлина 0%, НДС 16%\nТРОИС: «SIEMENS» — есть в реестре')
    expect(documentsShortText({ ...answer, documentsCode: "8538909200", documents: [
      { kind: "cc", measure: "Сертификат ТР ТС 004/2011", act: "", url: "", requirement: "", applies: "yes" },
      { kind: "other", measure: "Санэпиднадзор", act: "", url: "", requirement: "", applies: "maybe" },
    ] })).toBe('Документы для ввоза · 8538 90 920 0\nНужно: СС — Сертификат ТР ТС 004/2011\nВозможно: Санэпиднадзор')
    expect(brandShortText({ brands: [], pictures: [], hits: [], compared: 0 })).toBe("ТРОИС: в реестре не найдено")
  })
})
