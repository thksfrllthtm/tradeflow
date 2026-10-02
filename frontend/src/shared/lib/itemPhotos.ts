import { GoodsItemInput } from '../types/goods'

export function photosOf(item: GoodsItemInput): string[] {
  return item.photoUrls?.length ? item.photoUrls : item.photoUrl ? [item.photoUrl] : []
}

// Возвращает строкам фото, сохранённые в IndexedDB (в localStorage строки хранятся без фото).
export function mergeStoredPhotos(items: GoodsItemInput[], stored: Record<string, string[]>): GoodsItemInput[] {
  return items.map((item) => {
    if (photosOf(item).length > 0 || !item.id || !stored[item.id]?.length) return item
    const photos = stored[item.id]
    return { ...item, photoUrl: item.photoUrl || photos[0], photoUrls: photos }
  })
}
