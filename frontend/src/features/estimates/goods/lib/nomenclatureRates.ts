import { NomenclatureCode, NomenclatureImport, nomenclatureApi } from '@/features/directories/nomenclature/nomenclatureApi'
import { nameParts } from '@/features/directories/nomenclature/nomenclatureDraft'

// Ставки строки сметы — только из номенклатуры ТН ВЭД ЕАЭС: пошлина ЕТТ и НДС кода. Справочник
// ТН ВЭД и карточка товара дают название и ИТС; окончательно ставки подставляет сервер при
// расчёте, здесь — чтобы строка сразу показывала то же самое. Ставки перечня РК не применяются.

export type RateSnapshot = Record<string, unknown>

export function normalizeTnvedCode(value: string): string {
  const compact = String(value ?? '').trim().replace(/^['`]/, '').replace(/\s+/g, '')
  if (/^\d{10}$/.test(compact)) return compact
  return compact.match(/^(\d{10})\.0+$/)?.[1] ?? ''
}

const dutyType = (type: string) => (type === 'percentage' || type === 'specific' || type === 'combined_minimum' ? type : 'unknown')

// Снимок строки со ставками кода из номенклатуры; название и ИТС выбранной карточки (base)
// остаются, антидемпинг и статус проверки справочника отбрасываются.
export function withNomenclatureRates(item: NomenclatureCode, source: NomenclatureImport | null, base: RateSnapshot = {}): RateSnapshot {
  const sourceText = `Номенклатура ТН ВЭД ЕАЭС (ЕТТ), ${source?.source ?? 'TWS.BY'}${source?.actualDate ? ` на ${source.actualDate.split('-').reverse().join('.')}` : ''}`
  const rest = { ...base }
  delete rest.antiDumpingPercent
  delete rest.verificationStatus
  const { percent, amount, currency, unit } = item.duty
  return {
    ...rest,
    code: item.code,
    name: typeof base.name === 'string' && base.name.trim() ? base.name : nameParts(item.name).join(' '),
    tariffText: item.tariffText,
    rateSource: 'nomenclature',
    duty: { type: dutyType(item.duty.type), percent, amount, currency, unit, source: sourceText, sourceKind: 'nomenclature' },
    vat: { type: 'percentage', percent: item.vatPercent || '16', source: sourceText, sourceKind: 'nomenclature' },
  }
}

// Коды строк → найденные в номенклатуре; null — номенклатура не загружена.
export async function lookupNomenclature(codes: string[]): Promise<{ byCode: Map<string, NomenclatureCode>; source: NomenclatureImport | null } | null> {
  const unique = [...new Set(codes.filter(Boolean))]
  if (unique.length === 0) return { byCode: new Map(), source: null }
  const found = await nomenclatureApi.lookup(unique)
  if (!found.import) return null
  return { byCode: new Map(found.items.map((item) => [item.code, item])), source: found.import }
}
