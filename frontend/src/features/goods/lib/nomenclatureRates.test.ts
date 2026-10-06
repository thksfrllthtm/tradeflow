import { describe, expect, it } from 'vitest'
import { normalizeTnvedCode, withNomenclatureRates } from './nomenclatureRates'
import { NomenclatureCode } from '@/shared/tnved/nomenclature'

const code: NomenclatureCode = {
  code: '4015190000', name: 'перчатки, рукавицы и митенки: 🠺 прочие', tariffText: '10%', vatPercent: '16', detailsUrl: '',
  duty: { type: 'percentage', percent: '10', amount: '', currency: '', unit: '' },
  kz: { rateText: '8', duty: { type: 'percentage', percent: '8', amount: '', currency: '', unit: '' }, validFrom: '', validTo: '', active: true, status: 'действует', notesText: '' },
}
const source = { source: 'TWS.BY', fileName: '', actualDate: '2026-09-29', codeCount: 1, addedCount: 0, removedCount: 0, changedCount: 0, importedAt: '', importedByName: '' }

describe('withNomenclatureRates', () => {
  it('takes EAEU duty and VAT from the nomenclature, not the Kazakhstan list', () => {
    const snapshot = withNomenclatureRates(code, source)
    expect(snapshot).toMatchObject({ code: '4015190000', name: 'перчатки, рукавицы и митенки: прочие', rateSource: 'nomenclature', duty: { type: 'percentage', percent: '10' }, vat: { type: 'percentage', percent: '16' } })
    expect((snapshot.duty as { source: string }).source).toBe('Номенклатура ТН ВЭД ЕАЭС (ЕТТ), TWS.BY на 29.09.2026')
  })

  it('keeps the name and ИТС of a chosen card and drops directory anti-dumping', () => {
    const snapshot = withNomenclatureRates(code, source, { name: 'Перчатки нитриловые', companyCoefficient: '0.65', antiDumpingPercent: '5', verificationStatus: 'operator_verified', duty: { type: 'percentage', percent: '8' } })
    expect(snapshot).toMatchObject({ name: 'Перчатки нитриловые', companyCoefficient: '0.65', duty: { percent: '10' } })
    expect(snapshot).not.toHaveProperty('antiDumpingPercent')
    expect(snapshot).not.toHaveProperty('verificationStatus')
  })

  it('reads codes the way Excel gives them', () => {
    expect(['4015190000', ' 4015 19 000 0 ', '4015190000.0', "'4015190000", '401519000'].map(normalizeTnvedCode)).toEqual(['4015190000', '4015190000', '4015190000', '4015190000', ''])
  })
})
