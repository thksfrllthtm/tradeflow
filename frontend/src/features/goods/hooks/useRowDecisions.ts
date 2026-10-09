import { useCallback, useEffect, useRef, useState } from 'react'
import { decisionsApi } from '@/features/decisions/decisionsApi'
import { photosOf } from '@/shared/lib/itemPhotos'
import { resizePhoto } from '@/shared/lib/resizePhoto'
import { GoodsItemInput } from '@/shared/types/goods'

// Строки сметы → база решений (кнопка «В базу» в строке и для отмеченных строк). Повторы внутри
// сметы отсекает сервер: тот же код, то же наименование и похожее чёрно-белое фото — та же запись.

// Код и наименование строки: изменили одно из них после сохранения — это уже другой вариант.
export const rowDecisionKey = (item: GoodsItemInput) =>
  `${item.code.replace(/\D/g, '')}|${(item.standardName || item.name || '').trim().toLowerCase().replace(/ё/g, 'е').replace(/\s+/g, ' ')}`

export const canSaveRowDecision = (item: GoodsItemInput) => /^\d{10}$/.test(item.code.replace(/\D/g, '')) && Boolean((item.standardName || item.name || '').trim())

// Строка — в базу решений как «Не проверено». duplicate — этот товар сметы уже был в базе.
export async function saveRowDecision(item: GoodsItemInput, estimateId: number) {
  // Как в чате подбора: до трёх фото, уменьшенные.
  const photos = await Promise.all(photosOf(item).slice(0, 3).map((photo) => resizePhoto(photo)))
  const thumb = photosOf(item)[0] ? await resizePhoto(photosOf(item)[0], 160, 0.7) : ''
  return decisionsApi.create({
    invoiceName: item.name ?? '', supplierName: item.supplierName !== item.name ? item.supplierName : (item.otherNames ?? []).join(' / '),
    finalName: item.standardName || '', code: item.code, photos, thumb, estimateId, itemId: item.id ?? '',
  })
}

type Saved = { itemId: string; key: string }
// Состояние кнопки строки: saving — идёт запись; saved — в базе из этой строки; число — из строки с этим номером.
export type RowDecisionState = 'saving' | 'saved' | number | undefined

export function useRowDecisions(estimateId: number, items: GoodsItemInput[], enabled: boolean) {
  const [saved, setSaved] = useState<Record<string, Saved>>({})
  const [saving, setSaving] = useState<string[]>([])
  const itemsRef = useRef(items)
  itemsRef.current = items

  // Что уже в базе — по строкам, сохранённым на сервере (автосохранение сметы). Сохранённое из этого
  // окна остаётся: строка могла ещё не дойти до сервера.
  const refresh = useCallback(async () => {
    const status = await decisionsApi.estimateStatus(estimateId).catch(() => null)
    if (!status) return
    const byId = new Map(itemsRef.current.map((item) => [item.id ?? '', item]))
    setSaved((current) => ({ ...current, ...Object.fromEntries(Object.entries(status).flatMap(([id, entry]) => {
      const item = byId.get(id)
      return item ? [[id, { itemId: entry.itemId || id, key: rowDecisionKey(item) }]] : []
    })) }))
  }, [estimateId])
  useEffect(() => { if (enabled) void refresh() }, [enabled, refresh])

  function stateOf(item: GoodsItemInput): RowDecisionState {
    const id = item.id ?? ''
    if (saving.includes(id)) return 'saving'
    const entry = saved[id]
    if (!entry || entry.key !== rowDecisionKey(item)) return undefined
    if (entry.itemId === id) return 'saved'
    const index = items.findIndex((other) => other.id === entry.itemId)
    return index >= 0 ? index + 1 : 'saved'
  }

  // Отмеченные строки по очереди: так сервер видит уже сохранённые и не пишет тот же товар дважды.
  async function save(ids: string[]) {
    const targets = itemsRef.current.filter((item) => ids.includes(item.id ?? '') && canSaveRowDecision(item) && stateOf(item) === undefined)
    const counts = { created: 0, duplicates: 0, skipped: ids.length - targets.length, failed: 0 }
    setSaving((current) => [...current, ...targets.map((item) => item.id ?? '')])
    for (const item of targets) {
      const id = item.id ?? ''
      try {
        const decision = await saveRowDecision(item, estimateId)
        if (decision.duplicate) counts.duplicates++
        else counts.created++
        setSaved((current) => ({ ...current, [id]: { itemId: decision.itemId || id, key: rowDecisionKey(item) } }))
      } catch {
        counts.failed++
      } finally {
        setSaving((current) => current.filter((other) => other !== id))
      }
    }
    void refresh()
    return counts
  }

  return { stateOf, save, refresh }
}
