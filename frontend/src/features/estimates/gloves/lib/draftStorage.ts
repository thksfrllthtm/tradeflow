import { createPhotoStore } from '../../shared/lib/photoStore'
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
  return { assessmentDate: new Date().toISOString().slice(0, 10), origin: '', usdKzt: '', eurKzt: '', deliveryMode: 'wagons', wagons: '', wagonTariffUsd: '420', deliveryTotalUsd: '', packageWeightKg: '0.3', customsFeeKzt: '', mrp: '4325', feeMultiplier: '', cashUsd: '' }
}

export function normalizeParams(value: Partial<GlovesParams>): GlovesParams {
  const result = { ...emptyParams(), ...value }
  if (!String(result.mrp ?? '').trim()) result.mrp = '4325'
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
      // must keep the original source data so a new calculation starts from it.
      pricePerPair: '',
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
