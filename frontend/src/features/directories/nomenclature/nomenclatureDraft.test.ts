import { describe, expect, it } from 'vitest'
import { directoryDraft, directoryName, nameParts } from './nomenclatureDraft'
import { NomenclatureCode } from './nomenclatureApi'

const code = (tariff: Partial<NomenclatureCode['duty']>, name = 'перчатки, рукавицы и митенки: 🠺 прочие'): NomenclatureCode =>
  ({ code: '4015190000', name, tariffText: '', vatPercent: '16', detailsUrl: '', inDirectory: false, kz: null, duty: { type: 'unknown', percent: '', amount: '', currency: '', unit: '', ...tariff } })
const source = { source: 'TWS.BY', fileName: '', actualDate: '2026-09-29', codeCount: 1, addedCount: 1, removedCount: 0, changedCount: 0, importedAt: '', importedByName: '' }

describe('nomenclatureDraft', () => {
  it('splits the TWS name into tree levels', () => {
    expect(nameParts('перчатки: 🠺 прочие: 🠺 защитные')).toEqual(['перчатки:', 'прочие:', 'защитные'])
  })

  it('keeps the description within 300 characters', () => {
    expect(directoryName('перчатки: 🠺 прочие')).toBe('перчатки: прочие')
    const long = `${'а'.repeat(320)}: 🠺 прочие`
    expect(directoryName(long)).toBe('прочие')
  })

  it('fills code, description, EAEU duty and 16% VAT, leaving the card for review', () => {
    const draft = directoryDraft(code({ type: 'percentage', percent: '10' }), source)
    expect(draft).toMatchObject({ code: '4015190000', name: 'перчатки, рукавицы и митенки: прочие', verificationStatus: 'needs_review' })
    expect(draft.duty).toMatchObject({ type: 'percentage', percent: '10', source: 'ЕТТ ЕАЭС, TWS.BY на 29.09.2026. Сверить с перечнем РК' })
    expect(draft.vat).toMatchObject({ type: 'percentage', percent: '16' })
  })

  it('moves specific and minimum rates, leaves «plus» rates unknown', () => {
    expect(directoryDraft(code({ type: 'combined_minimum', percent: '50', amount: '1', currency: 'EUR', unit: 'кг' }), source).duty)
      .toMatchObject({ type: 'combined_minimum', percent: '50', amount: '1', currency: 'EUR', unit: 'кг' })
    expect(directoryDraft(code({ type: 'combined_plus', percent: '4.5', amount: '0.02', currency: 'EUR', unit: 'кг' }), source).duty)
      .toMatchObject({ type: 'unknown', percent: '', amount: '' })
  })

  describe('with the Kazakhstan list', () => {
    const kzSource = { source: 'pdf' as const, fileName: 'ПЕРЕЧЕНЬ РК на 07-02-2026.pdf', rowCount: 3869, importedAt: '', importedByName: '' }
    const kz = (rate: Partial<NonNullable<NomenclatureCode['kz']>>): NomenclatureCode => ({
      ...code({ type: 'percentage', percent: '10' }), tariffText: '10%',
      kz: { rateText: '8', duty: { type: 'percentage', percent: '8', amount: '', currency: '', unit: '' }, validFrom: '', validTo: '', active: true, status: 'действует', notesText: '', ...rate },
    })

    it('takes the lower Kazakhstan rate while it applies', () => {
      expect(directoryDraft(kz({}), source, kzSource).duty).toMatchObject({
        type: 'percentage', percent: '8', source: 'Перечень РК (ставки ниже ЕТТ), ПЕРЕЧЕНЬ РК на 07-02-2026.pdf: «8»; ЕТТ — 10%',
      })
    })

    it('leaves a «whichever is lower» rate for manual entry', () => {
      const draft = directoryDraft(kz({ rateText: '13 или 1,32 евро за 1 л в зависимости, что ниже', duty: { type: 'other', percent: '', amount: '', currency: '', unit: '' } }), source, kzSource)
      expect(draft.duty).toMatchObject({ type: 'unknown', percent: '' })
      expect(draft.duty?.source).toContain('Заполните ставку вручную')
    })

    it('keeps the EAEU rate when the Kazakhstan rate does not apply yet or any more', () => {
      expect(directoryDraft(kz({ active: false, status: 'с 01.01.2030, до этого ЕТТ' }), source, kzSource).duty)
        .toMatchObject({ type: 'percentage', percent: '10', source: 'ЕТТ ЕАЭС, TWS.BY на 29.09.2026. Перечень РК: «8» — с 01.01.2030, до этого ЕТТ' })
      expect(directoryDraft(code({ type: 'percentage', percent: '10' }), source, kzSource).duty?.source).toBe('ЕТТ ЕАЭС, TWS.BY на 29.09.2026. Кода нет в перечне РК')
    })
  })
})
