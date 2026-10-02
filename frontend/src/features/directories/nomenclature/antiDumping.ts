import { request } from '@/shared/api/client'

// Действующие антидемпинговые меры ЕАЭС из реестра ЕЭК: список и обновление с сайта ЕЭК.

export type AntiDumpingMeasure = {
  number: string; type: string; shortName: string; description: string; codes: string[]; countries: string[]
  rates: Record<string, string>; rateText: string; minPercent: string; maxPercent: string; startDate: string; endDate: string
  suspended: boolean; needsCheck: string; foundation: string; documentUrl: string; note: string
}
export type AntiDumpingImport = { source: 'seed' | 'eec'; measureCount: number; importedAt: string; importedByName: string; registryUrl: string }
type AntiDumpingList = { measures: AntiDumpingMeasure[]; import: AntiDumpingImport | null }

export const antiDumpingApi = {
  list: () => request<AntiDumpingList>('/api/tnved-antidumping'),
  refresh: () => request<AntiDumpingList>('/api/tnved-antidumping/refresh', { method: 'POST' }),
}

export const countryNames: Record<string, string> = {
  CN: 'Китай', UA: 'Украина', MY: 'Малайзия', TH: 'Таиланд', TR: 'Турция', JP: 'Япония', GE: 'Грузия', AZ: 'Азербайджан',
  IN: 'Индия', UZ: 'Узбекистан', KR: 'Республика Корея', VN: 'Вьетнам', ID: 'Индонезия', TW: 'Тайвань',
}

export const percentText = (value: string) => value ? `${value.replace('.', ',')}%` : ''

// «8511 10 000 1», «7320 10», «7210» — по группам цифр, как пишут коды ТН ВЭД.
export function formatMeasureCode(code: string) {
  const groups = [code.slice(0, 4), code.slice(4, 6), code.slice(6, 9), code.slice(9)]
  return groups.filter(Boolean).join(' ')
}
