import { TNVEDDirectoryEntry, TNVEDDirectoryInput, TNVEDRate } from './types'

// Значения формы записи ТН ВЭД: пустая запись и перенос существующей записи в форму.
const emptyRate = (): TNVEDRate => ({ type: 'unknown', percent: '', amount: '', currency: '', unit: '', basis: '', validFrom: '', validTo: '', checkedAt: '', source: '' })
// Новая запись сразу с НДС 16% — основной ставкой Казахстана; 5% или освобождение меняются вручную.
export const emptyInput = (): TNVEDDirectoryInput => ({ category: 'Товары', name: '', nameZh: '', code: '', companyCoefficient: '', antiDumpingPercent: '', synonymsRu: [], synonymsZh: [], synonymsEn: [], verificationStatus: 'needs_review', duty: emptyRate(), vat: { ...emptyRate(), type: 'percentage', percent: '16' } })

export function entryToInput(item: TNVEDDirectoryEntry): TNVEDDirectoryInput { return { category: item.category, name: item.name, nameZh: item.nameZh, code: item.code, companyCoefficient: item.companyCoefficient, antiDumpingPercent: item.antiDumpingPercent, synonymsRu: item.synonymsRu, synonymsZh: item.synonymsZh, synonymsEn: item.synonymsEn, verificationStatus: item.verificationStatus, duty: { ...item.duty, checkedAt: item.duty.checkedAt ?? '' }, vat: { ...item.vat, checkedAt: item.vat.checkedAt ?? '' } } }
