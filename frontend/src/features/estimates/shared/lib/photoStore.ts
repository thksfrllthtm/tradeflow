import { GlovesItemInput } from '../types'

// Фото строк (base64, мегабайты) хранятся в IndexedDB, а не в localStorage.
// У каждой сметы своя база, поэтому хранилище создаётся с её именем.
const PHOTO_STORE_NAME = 'estimatePhotos'

type StoredPhotos = { estimateId: number; photos: Record<string, string[]> }

type PhotoStore = {
  load: (estimateId: number) => Promise<Record<string, string[]>>
  save: (estimateId: number, items: GlovesItemInput[]) => Promise<void>
  remove: (estimateId: number) => Promise<void>
}

export function createPhotoStore(databaseName: string): PhotoStore {
  function open(): Promise<IDBDatabase> {
    if (typeof indexedDB === 'undefined') return Promise.reject(new Error('IndexedDB недоступен'))
    return new Promise((resolve, reject) => {
      const request = indexedDB.open(databaseName, 1)
      request.onupgradeneeded = () => {
        const database = request.result
        if (!database.objectStoreNames.contains(PHOTO_STORE_NAME)) database.createObjectStore(PHOTO_STORE_NAME, { keyPath: 'estimateId' })
      }
      request.onsuccess = () => resolve(request.result)
      request.onerror = () => reject(request.error ?? new Error('Не удалось открыть хранилище фотографий'))
    })
  }

  async function load(estimateId: number): Promise<Record<string, string[]>> {
    const database = await open()
    return new Promise((resolve, reject) => {
      const request = database.transaction(PHOTO_STORE_NAME, 'readonly').objectStore(PHOTO_STORE_NAME).get(estimateId)
      request.onsuccess = () => {
        database.close()
        const stored = request.result as StoredPhotos | undefined
        resolve(stored?.photos ?? {})
      }
      request.onerror = () => {
        database.close()
        reject(request.error ?? new Error('Не удалось загрузить фотографии'))
      }
    })
  }

  async function save(estimateId: number, items: GlovesItemInput[]): Promise<void> {
    const photos: Record<string, string[]> = {}
    items.forEach((item) => {
      if (!item.id) return
      const values = item.photoUrls?.length ? item.photoUrls : item.photoUrl ? [item.photoUrl] : []
      if (values.length > 0) photos[item.id] = values
    })
    const database = await open()
    return new Promise((resolve, reject) => {
      const transaction = database.transaction(PHOTO_STORE_NAME, 'readwrite')
      transaction.objectStore(PHOTO_STORE_NAME).put({ estimateId, photos } satisfies StoredPhotos)
      transaction.oncomplete = () => { database.close(); resolve() }
      transaction.onerror = () => { database.close(); reject(transaction.error ?? new Error('Не удалось сохранить фотографии')) }
      transaction.onabort = () => { database.close(); reject(transaction.error ?? new Error('Не удалось сохранить фотографии')) }
    })
  }

  async function remove(estimateId: number): Promise<void> {
    try {
      const database = await open()
      await new Promise<void>((resolve, reject) => {
        const transaction = database.transaction(PHOTO_STORE_NAME, 'readwrite')
        transaction.objectStore(PHOTO_STORE_NAME).delete(estimateId)
        transaction.oncomplete = () => { database.close(); resolve() }
        transaction.onerror = () => { database.close(); reject(transaction.error) }
        transaction.onabort = () => { database.close(); reject(transaction.error) }
      })
    } catch {
      // The browser cache is optional; server draft storage remains available.
    }
  }

  return { load, save, remove }
}
