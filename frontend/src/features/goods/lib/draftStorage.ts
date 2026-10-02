import { createPhotoStore } from './photoStore'
import { todayIso } from '@/shared/lib/dates'
import { storageKey } from './localDraft'
import { GoodsCalculationResult, GoodsItemInput, GoodsParams } from '@/shared/types/goods'

// Черновик сметы товаров в браузере: параметры, строки и результат расчёта — в localStorage,
// фотографии — в IndexedDB. Ключи не менять: по ним читаются уже сохранённые черновики
// (в имени осталось «gloves-copy» от первой версии калькулятора; переименование потеряло бы
// несохранённые на сервере правки сотрудников).
export const PARAMS_KEY = 'tradeflow.gloves-copy.params'
export const ITEMS_KEY = 'tradeflow.gloves-copy.items'
export const RESULT_KEY = 'tradeflow.gloves-copy.result'
export const FILE_KEY = 'tradeflow.gloves-copy.file'
// Есть правки, которых нет на сервере. Ставится, только когда черновик отличается от сохранённого.
export const LOCAL_UPDATED_KEY = 'tradeflow.gloves-copy.localUpdatedAt'

// Последний результат расчёта (фото в нём нет): смета снова открывается на этапе результата
// с ценами, изменёнными на этом этапе.
export function storedResult(estimateId: number): GoodsCalculationResult | null {
  try {
    const raw = localStorage.getItem(storageKey(RESULT_KEY, estimateId))
    const value = raw ? JSON.parse(raw) as GoodsCalculationResult : null
    return value && Array.isArray(value.items) && value.totals ? value : null
  } catch {
    return null
  }
}

// Черновик без фото — чтобы сравнить его с сохранённым на сервере.
export function draftSnapshot(params: GoodsParams, items: GoodsItemInput[], result: GoodsCalculationResult | null) {
  return JSON.stringify({ params, items: items.map(({ photoUrl: _photoUrl, photoUrls: _photoUrls, ...item }) => item), result })
}

export const goodsPhotoStore = createPhotoStore('tradeflow-gloves-copy-photos')

export function emptyParams(): GoodsParams {
  return { assessmentDate: todayIso(), origin: '', usdKzt: '', eurKzt: '', deliveryMode: 'none', wagons: '', wagonTariffUsd: '420', deliveryTotalUsd: '', packageWeightKg: '0.3', customsFeeKzt: '', mrp: '4325', feeMultiplier: '6', cashUsd: '', directoryCoefficientOnly: true, genericItems: true }
}

export function normalizeParams(value: Partial<GoodsParams>): GoodsParams {
  const defaults = emptyParams()
  const result = { ...defaults, ...value }
  // Новая смета приходит с сервера с пустыми полями: подставляем значения по умолчанию
  // (сегодняшняя дата, без доставки, МРП и множитель сбора), чтобы не вводить их каждый раз.
  for (const field of ['assessmentDate', 'deliveryMode', 'wagonTariffUsd', 'mrp', 'feeMultiplier'] as const) {
    if (!String(result[field] ?? '').trim()) Object.assign(result, { [field]: defaults[field] })
  }
  return result
}

export function storedParams(estimateId: number): GoodsParams {
  try {
    const raw = localStorage.getItem(storageKey(PARAMS_KEY, estimateId))
    if (!raw) return emptyParams()
    return normalizeParams(JSON.parse(raw) as Partial<GoodsParams>)
  } catch {
    return emptyParams()
  }
}

export function storedItems(estimateId: number): GoodsItemInput[] {
  try {
    const raw = localStorage.getItem(storageKey(ITEMS_KEY, estimateId))
    if (!raw) return []
    const value = JSON.parse(raw)
    return Array.isArray(value) ? ensureItemIds(value) : []
  } catch {
    return []
  }
}

export function ensureItemIds(value: GoodsItemInput[]): GoodsItemInput[] {
  const stamp = Date.now()
  return value.map((item, index) => {
    // ИТП принадлежит карточке товара, а не самому коду ТН ВЭД. Старые
    // черновики могли сохранить ИТП после автоматического сопоставления кода,
    // поэтому очищаем его у строк без явно выбранного товара.
    const hasProduct = Number(item.productDirectoryId ?? 0) > 0
    const directorySnapshot = item.directorySnapshot ? { ...item.directorySnapshot } : undefined
    if (!hasProduct && directorySnapshot) delete directorySnapshot.companyCoefficient
    const normalized: GoodsItemInput = {
      ...item,
      // The editable price on the result screen is temporary. The input rows
      // must keep the original source data so a new calculation starts from it:
      // the price from the Excel file, if the row had one.
      pricePerPair: item.sourcePricePerPair ?? '',
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
