import { MutableRefObject, useEffect, useRef } from 'react'
import { MAX_LOCAL_STORAGE_CHARS, storageKey } from '../lib/localDraft'
import { GoodsCalculationResult, GoodsItemInput, GoodsParams } from '@/shared/types/goods'
import { draftSnapshot, goodsPhotoStore, ITEMS_KEY, LOCAL_UPDATED_KEY, PARAMS_KEY, RESULT_KEY } from '../lib/draftStorage'

// Сохранение черновика в браузер откладывается до паузы в вводе, чтобы не
// сериализовать все строки и фотографии на каждое нажатие клавиши.
const PERSIST_DELAY_MS = 400

type Options = {
  estimateId: number
  params: GoodsParams
  items: GoodsItemInput[]
  result: GoodsCalculationResult | null
  // Фото можно писать в IndexedDB только после того, как сохранённые фото загружены.
  photoStorageReady: boolean
  // Черновик загружен с сервера: с этого момента отмечаем правки, которых нет на сервере.
  hydrationRef: MutableRefObject<boolean>
  // Снимок черновика, сохранённого на сервере (draftSnapshot); savedVersion меняется после
  // каждого сохранения, чтобы сразу сравнить текущий черновик с новым снимком.
  savedSnapshotRef: MutableRefObject<string | null>
  savedVersion: number
  onNotice: (message: string) => void
}

// Автосохранение черновика сметы товаров в браузер. Фото (мегабайты base64)
// пишутся в IndexedDB только если они действительно изменились; при закрытии
// вкладки или переключении сметы отложенное сохранение выполняется сразу.
export function useDraftAutosave({ estimateId, params, items, result, photoStorageReady, hydrationRef, savedSnapshotRef, savedVersion, onNotice }: Options) {
  const paramsStorageKey = storageKey(PARAMS_KEY, estimateId)
  const itemsStorageKey = storageKey(ITEMS_KEY, estimateId)
  const resultStorageKey = storageKey(RESULT_KEY, estimateId)
  const localUpdatedStorageKey = storageKey(LOCAL_UPDATED_KEY, estimateId)
  const persistRef = useRef<(() => void) | null>(null)
  const savedPhotosRef = useRef(new Map<string, unknown>())

  useEffect(() => {
    persistRef.current = () => {
      persistRef.current = null
      try {
        localStorage.setItem(paramsStorageKey, JSON.stringify(params))
        localStorage.setItem(itemsStorageKey, JSON.stringify(items.map((item) => ({ ...item, photoUrl: '', photoUrls: [] }))))
        if (result) localStorage.setItem(resultStorageKey, JSON.stringify(result))
        else localStorage.removeItem(resultStorageKey)
        const saved = savedPhotosRef.current
        const photosChanged = saved.size !== items.length || items.some((item) => saved.get(item.id ?? '') !== (item.photoUrls ?? item.photoUrl))
        if (photoStorageReady && photosChanged) {
          savedPhotosRef.current = new Map(items.map((item) => [item.id ?? '', item.photoUrls ?? item.photoUrl]))
          void goodsPhotoStore.save(estimateId, items).catch(() => {
            savedPhotosRef.current = new Map()
            const serialized = JSON.stringify(items)
            if (serialized.length <= MAX_LOCAL_STORAGE_CHARS) localStorage.setItem(itemsStorageKey, serialized)
            else onNotice('Не удалось сохранить фотографии в браузере. Сохраните черновик на сервере.')
          })
        }
        // Отметка «в браузере новее сервера» — только при настоящих отличиях: иначе простое
        // открытие сметы делало бы браузерную копию главной, и результат с сервера терялся.
        if (hydrationRef.current) {
          if (draftSnapshot(params, items, result) === savedSnapshotRef.current) localStorage.removeItem(localUpdatedStorageKey)
          else localStorage.setItem(localUpdatedStorageKey, String(Date.now()))
        }
      } catch {
        onNotice('Не удалось сохранить черновик в браузере. Экспортируйте результат перед закрытием страницы.')
      }
    }
    const timer = window.setTimeout(() => persistRef.current?.(), PERSIST_DELAY_MS)
    return () => window.clearTimeout(timer)
  }, [params, items, result, estimateId, paramsStorageKey, itemsStorageKey, resultStorageKey, localUpdatedStorageKey, photoStorageReady, savedVersion])

  useEffect(() => {
    const flush = () => persistRef.current?.()
    window.addEventListener('pagehide', flush)
    return () => { window.removeEventListener('pagehide', flush); flush() }
  }, [])
}
