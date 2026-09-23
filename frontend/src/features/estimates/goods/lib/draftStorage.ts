import { createPhotoStore } from '../../shared/lib/photoStore'
import { storageKey } from '../../shared/lib/localDraft'
import { GlovesItemInput, GlovesParams } from '../../shared/types'

// Черновик сметы товаров в браузере: параметры и строки — в localStorage,
// фотографии — в IndexedDB. Ключи не менять: по ним читаются уже сохранённые черновики.
export const PARAMS_KEY = 'tradeflow.gloves-copy.params'
export const ITEMS_KEY = 'tradeflow.gloves-copy.items'
export const FILE_KEY = 'tradeflow.gloves-copy.file'
export const LOCAL_UPDATED_KEY = 'tradeflow.gloves-copy.localUpdatedAt'

export const goodsPhotoStore = createPhotoStore('tradeflow-gloves-copy-photos')

export function emptyParams(): GlovesParams {
  return { assessmentDate: new Date().toISOString().slice(0, 10), origin: '', usdKzt: '', eurKzt: '', deliveryMode: 'wagons', wagons: '', wagonTariffUsd: '420', deliveryTotalUsd: '', packageWeightKg: '0.3', customsFeeKzt: '', mrp: '4325', feeMultiplier: '', cashUsd: '', directoryCoefficientOnly: true, genericItems: true }
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
    // ИТП принадлежит карточке товара, а не самому коду ТН ВЭД. Старые
    // черновики могли сохранить ИТП после автоматического сопоставления кода,
    // поэтому очищаем его у строк без явно выбранного товара.
    const hasProduct = Number(item.productDirectoryId ?? 0) > 0
    const directorySnapshot = item.directorySnapshot ? { ...item.directorySnapshot } : undefined
    if (!hasProduct && directorySnapshot) delete directorySnapshot.companyCoefficient
    const normalized: GlovesItemInput = {
      ...item,
      // The editable price on the result screen is temporary. The input rows
      // must keep the original source data so a new calculation starts from it.
      pricePerPair: '',
      coefficient: hasProduct ? (item.coefficient ?? '') : '',
      directorySnapshot,
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
