import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'

// Ошибки последнего расчёта для этапа «Проверка строк». Правка строки сбрасывает результат
// расчёта (он больше не соответствует строкам), но список ошибок сотруднику нужен до следующего
// расчёта. Поэтому после каждого расчёта сохраняется копия: ошибки и предупреждения по id строки
// (не по номеру — удаление и фильтр не переносят ошибку на другой товар) и подпись строки, по
// которой видно, менялась ли она после расчёта. Копия принадлежит одной смете.
export type RowCheck = { errors: string[]; warnings: string[]; calculated: boolean; signature: string }
export type ReviewCheck = { estimateId: number; at: string; rows: Record<string, RowCheck> }

// Подпись строки — всё, что уходит в расчёт, кроме фото и цены единицы: цена вводится на этапе
// «Результат» и там же временно меняется, на этапе проверки её ошибки не показываются.
export function rowSignature(item: GoodsItemInput) {
  const { photoUrl: _photo, photoUrls: _photos, pricePerPair: _price, ...rest } = item
  return JSON.stringify(Object.keys(rest).sort().map((key) => [key, rest[key as keyof typeof rest]]))
}

// Копия ошибок из результата: строки результата идут в том же порядке, что строки расчёта.
export function checkFromResult(estimateId: number, items: GoodsItemInput[], result: GoodsCalculationResult): ReviewCheck {
  const rows: Record<string, RowCheck> = {}
  items.forEach((item, index) => {
    const resultItem = result.items[index]
    if (!item.id || !resultItem) return
    rows[item.id] = { errors: resultItem.errors ?? [], warnings: resultItem.warnings ?? [], calculated: resultItem.status === 'calculated', signature: rowSignature(item) }
  })
  return { estimateId, at: result.calculatedAt || result.updatedAt || new Date().toISOString(), rows }
}

const KEY = 'tradeflow.review-check'
const keyFor = (estimateId: number) => `${KEY}.${estimateId}`

// Копия хранится в браузере по номеру сметы: после перезагрузки страницы ошибки остаются, другая
// смета их не видит. Без доступа к хранилищу копия живёт до перезагрузки.
export function loadReviewCheck(estimateId: number): ReviewCheck | null {
  try {
    const raw = localStorage.getItem(keyFor(estimateId))
    const value = raw ? JSON.parse(raw) as ReviewCheck : null
    return value && value.estimateId === estimateId && value.rows ? value : null
  } catch {
    return null
  }
}

export function saveReviewCheck(check: ReviewCheck | null, estimateId: number) {
  try {
    if (check) localStorage.setItem(keyFor(estimateId), JSON.stringify(check))
    else localStorage.removeItem(keyFor(estimateId))
  } catch { /* без хранилища — до перезагрузки */ }
}
