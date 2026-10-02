import { nameParts } from '@/shared/tnved/codes'
import { formatRuDate } from '@/shared/lib/dates'
import { TNVEDDirectoryInput, TNVEDRate } from '../tnved/types'
import { KzImport } from '../kzList/kzListApi'
import { NomenclatureCode, NomenclatureImport } from '@/shared/tnved/nomenclature'

// Описание в справочнике — не длиннее 300 символов: целиком, а если не помещается —
// последний уровень.
const maxDescription = 300
export function directoryName(name: string) {
  const parts = nameParts(name)
  const full = parts.join(' ')
  if (full.length <= maxDescription) return full
  const last = parts[parts.length - 1] ?? full
  return last.length <= maxDescription ? last : `${last.slice(0, maxDescription - 1)}…`
}

const emptyRate = (): TNVEDRate => ({ type: 'unknown', percent: '', amount: '', currency: '', unit: '', basis: '', validFrom: '', validTo: '', checkedAt: '', source: '' })

function rateFrom(value: { type: string; percent: string; amount: string; currency: string; unit: string }, source: string): TNVEDRate {
  const { type, percent, amount, currency, unit } = value
  const rate = { ...emptyRate(), source }
  if (type === 'percentage') return { ...rate, type, percent }
  if (type === 'specific') return { ...rate, type, amount, currency, unit }
  if (type === 'combined_minimum') return { ...rate, type, percent, amount, currency, unit }
  return rate
}

// Черновик карточки справочника из строки номенклатуры: код, описание, пошлина и НДС кода
// (16% — основная ставка Казахстана). Пошлина — по перечню РК, если ставка РК сегодня
// действует, иначе ЕТТ. Статус остаётся «Требует проверки»: ставку и НДС сверяет человек.
export function directoryDraft(item: NomenclatureCode, source: NomenclatureImport | null, kzSource: KzImport | null = null): Partial<TNVEDDirectoryInput> {
  const ettText = `ЕТТ ЕАЭС, ${source?.source ?? 'TWS.BY'}${source?.actualDate ? ` на ${formatRuDate(source.actualDate)}` : ''}`
  const kz = item.kz
  let duty: TNVEDRate
  if (kz?.active) {
    const kzText = `Перечень РК (ставки ниже ЕТТ)${kzSource?.fileName ? `, ${kzSource.fileName}` : ''}: «${kz.rateText}»; ЕТТ — ${item.tariffText || 'не указана'}`
    // Сложную ставку РК («… или …, что ниже») справочник не хранит — её вводят вручную.
    duty = kz.duty.type === 'other' ? { ...emptyRate(), source: `${kzText}. Заполните ставку вручную` } : rateFrom(kz.duty, kzText)
  } else {
    duty = rateFrom(item.duty, kz ? `${ettText}. Перечень РК: «${kz.rateText}» — ${kz.status}` : kzSource ? `${ettText}. Кода нет в перечне РК` : `${ettText}. Сверить с перечнем РК`)
  }
  return { code: item.code, name: directoryName(item.name), duty, vat: { ...emptyRate(), type: 'percentage', percent: item.vatPercent || '16' }, verificationStatus: 'needs_review' }
}
