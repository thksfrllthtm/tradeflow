import { resizePhoto } from '@/shared/lib/resizePhoto'
import { photosOf } from '@/shared/lib/itemPhotos'
import { GoodsItemInput } from '@/shared/types/goods'
import { AgentItem } from './agentApi'

export function agentItemFrom(item: GoodsItemInput): AgentItem {
  return {
    name: item.name ?? '', supplierName: item.supplierName && item.supplierName !== item.name ? item.supplierName : '', otherNames: item.otherNames ?? [], details: item.details ?? [], code: item.code ?? '',
    quantity: item.pairs ?? '', places: item.places ?? '', grossKg: item.grossKg ?? '', netKg: item.netKg ?? '', pricePerUnit: item.pricePerPair ?? '', unit: 'шт',
  }
}

// Фото строки для ИИ: до трёх, уменьшенные до 1024 px.
export async function agentPhotosFrom(item: GoodsItemInput): Promise<string[]> {
  return Promise.all(photosOf(item).slice(0, 3).map((photo) => resizePhoto(photo)))
}

// Нет ни наименования, ни фото — сервер подбор не начнёт (ему не по чему искать).
export const isEmptyItem = (item: GoodsItemInput) => !item.name?.trim() && !item.supplierName?.trim() && photosOf(item).length === 0
