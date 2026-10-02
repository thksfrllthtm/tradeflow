import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'
import { photosOf } from '@/shared/lib/itemPhotos'

// Строки для выгрузки в Excel: цены — как на экране результата (ручные правки, вычет,
// распределение бюджета), и одно, первое, фото строки — оно вставляется в файл.
export function exportItems(items: GoodsItemInput[], result: GoodsCalculationResult | null): GoodsItemInput[] {
  return items.map((item, index) => {
    const { photoUrls: _photoUrls, ...rest } = item
    return { ...rest, photoUrl: photosOf(item)[0] ?? '', pricePerPair: result?.items[index]?.pricePerPair || item.pricePerPair }
  })
}
