// Ключи localStorage черновика: у каждой сметы свой набор с суффиксом id.
export function storageKey(base: string, estimateId: number) {
  return `${base}.${estimateId}`
}

// Предел для запасного сохранения строк вместе с фото в localStorage.
export const MAX_LOCAL_STORAGE_CHARS = 4_000_000
