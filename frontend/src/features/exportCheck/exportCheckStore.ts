import { Declaration } from './declarationPdf'
import { Report } from './exportCheckApi'

// Текущая сверка хранится в IndexedDB браузера — вместе с самими файлами (File сохраняется как есть),
// поэтому переживает обновление страницы и переходы между разделами. Своя запись у каждого
// пользователя; стирается только кнопкой «Очистить сверку».
export type SavedCheck = {
  invoice: File | null
  loaded: { file: File; declaration?: Declaration; error?: string }[]
  report: Report | null
  tab: 'error' | 'warn' | 'all'
}

const DATABASE = 'exportCheck'
const STORE = 'current'

function open(): Promise<IDBDatabase> {
  if (typeof indexedDB === 'undefined') return Promise.reject(new Error('IndexedDB недоступен'))
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DATABASE, 1)
    request.onupgradeneeded = () => { if (!request.result.objectStoreNames.contains(STORE)) request.result.createObjectStore(STORE) }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error ?? new Error('Не удалось открыть хранилище сверки'))
  })
}

async function run<T>(mode: IDBTransactionMode, action: (store: IDBObjectStore) => IDBRequest): Promise<T> {
  const database = await open()
  return new Promise((resolve, reject) => {
    const transaction = database.transaction(STORE, mode)
    const request = action(transaction.objectStore(STORE))
    transaction.oncomplete = () => { database.close(); resolve(request.result as T) }
    transaction.onerror = transaction.onabort = () => { database.close(); reject(transaction.error ?? new Error('Ошибка хранилища сверки')) }
  })
}

const key = (userId: number) => `user-${userId}`

export const exportCheckStore = {
  load: (userId: number) => run<SavedCheck | undefined>('readonly', (store) => store.get(key(userId))),
  save: (userId: number, value: SavedCheck) => run<unknown>('readwrite', (store) => store.put(value, key(userId))),
  clear: (userId: number) => run<unknown>('readwrite', (store) => store.delete(key(userId))),
}
