import { photosOf } from '@/shared/lib/itemPhotos'
import { GoodsItemInput } from '@/shared/types/goods'

// Фото строк на этапе проверки: удалить, сделать главным (первое — миниатюра и первое фото для
// подбора кода), перенести в другую строку — если при загрузке фото попало не к тому товару.
export const MAX_ROW_PHOTOS = 20

const withPhotos = (item: GoodsItemInput, photos: string[]): GoodsItemInput => ({ ...item, photoUrl: photos[0] ?? '', photoUrls: photos })

export function removeRowPhoto(items: GoodsItemInput[], itemId: string, index: number): GoodsItemInput[] {
  return items.map((item) => item.id === itemId ? withPhotos(item, photosOf(item).filter((_, at) => at !== index)) : item)
}

export function makeMainRowPhoto(items: GoodsItemInput[], itemId: string, index: number): GoodsItemInput[] {
  return items.map((item) => {
    if (item.id !== itemId) return item
    const photos = photosOf(item)
    if (index <= 0 || index >= photos.length) return item
    return withPhotos(item, [photos[index], ...photos.filter((_, at) => at !== index)])
  })
}

// Перенос фото в конец списка другой строки; у строки без фото оно становится главным.
// null — перенести нельзя (нет фото, та же строка или у строки уже MAX_ROW_PHOTOS фото).
export function moveRowPhoto(items: GoodsItemInput[], fromId: string, index: number, toId: string): GoodsItemInput[] | null {
  const from = items.find((item) => item.id === fromId)
  const to = items.find((item) => item.id === toId)
  const photo = from ? photosOf(from)[index] : undefined
  if (!from || !to || !photo || fromId === toId || photosOf(to).length >= MAX_ROW_PHOTOS) return null
  return items.map((item) => {
    if (item.id === fromId) return withPhotos(item, photosOf(item).filter((_, at) => at !== index))
    if (item.id === toId) return withPhotos(item, [...photosOf(item), photo])
    return item
  })
}
