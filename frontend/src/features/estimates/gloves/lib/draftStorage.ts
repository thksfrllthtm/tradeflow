import { createPhotoStore } from '../../shared/lib/photoStore'
import { todayIso } from '../../shared/lib/dates'
import { storageKey } from '../../shared/lib/localDraft'
import { GlovesItemInput, GlovesParams } from '../../shared/types'

// Черновик сметы перчаток в браузере: параметры и строки — в localStorage,
// фотографии — в IndexedDB. Ключи не менять: по ним читаются уже сохранённые черновики.
export const PARAMS_KEY = 'tradeflow.gloves.params'
export const ITEMS_KEY = 'tradeflow.gloves.items'
export const FILE_KEY = 'tradeflow.gloves.file'
export const LOCAL_UPDATED_KEY = 'tradeflow.gloves.localUpdatedAt'

export const glovesPhotoStore = createPhotoStore('tradeflow-gloves-photos')

export function emptyParams(): GlovesParams {
  return { assessmentDate: todayIso(), origin: '', usdKzt: '', eurKzt: '', deliveryMode: 'wagons', wagons: '', wagonTariffUsd: '420', deliveryTotalUsd: '', packageWeightKg: '0.3', customsFeeKzt: '', mrp: '4325', feeMultiplier: '6', cashUsd: '' }
}

export function normalizeParams(value: Partial<GlovesParams>): GlovesParams {
  const defaults = emptyParams()
  const result = { ...defaults, ...value }
  // Новая смета приходит с сервера с пустыми полями: подставляем значения по умолчанию
  // (сегодняшняя дата, доставка по вагонам, МРП и множитель сбора), чтобы не вводить их каждый раз.
  for (const field of ['assessmentDate', 'deliveryMode', 'wagonTariffUsd', 'mrp', 'feeMultiplier'] as const) {
    if (!String(result[field] ?? '').trim()) Object.assign(result, { [field]: defaults[field] })
  }
  return result
}

export function storedParams(estimateId: number): GlovesParams {
  try {
    const raw = localStorage.getItem(storageKey(PARAMS_KEY, estimateId))
    if (!raw) return emptyParams()
    return normalizeParams(JSON.parse(raw) as Partial<GlovesParams>)
  } catch {
    return emptyParams()
  }
}

export function storedItems(estimateId: number): GlovesItemInput[] {
  try {
    const raw = localStorage.getItem(storageKey(ITEMS_KEY, estimateId))
    if (!raw) return []
    const value = JSON.parse(raw)
    return Array.isArray(value) ? ensureItemIds(value) : []
  } catch {
    return []
  }
}

export function ensureItemIds(value: GlovesItemInput[]): GlovesItemInput[] {
  const stamp = Date.now()
  return value.map((item, index) => {
    const normalized: GlovesItemInput = {
      ...item,
      // The editable price on the result screen is temporary. The input rows
      // must keep the original source data so a new calculation starts from it:
      // the price from the Excel file, if the row had one.
      pricePerPair: item.sourcePricePerPair ?? '',
      costInput: item.costInput ?? '',
      costCurrency: item.costCurrency ?? '',
      extraExpenses: item.extraExpenses ?? '',
      customsValueDirectKzt: item.customsValueDirectKzt ?? '',
      deliveryAlreadyInCost: item.deliveryAlreadyInCost ?? false,
      photoUrls: Array.isArray(item.photoUrls) ? item.photoUrls : item.photoUrl ? [item.photoUrl] : [],
    }
    return normalized.id ? normalized : { ...normalized, id: `glove-restored-${item.sourceRow || 0}-${index}-${stamp}` }
  })
}
