import { useEffect, useRef } from 'react'

// JSON с ключами по алфавиту: сервер (jsonb) возвращает ключи в своём порядке, а сравнивать надо значения.
export function stableJson(value: unknown): string {
  return JSON.stringify(value ?? null, (_key, item: unknown) => item && typeof item === 'object' && !Array.isArray(item)
    ? Object.fromEntries(Object.entries(item as Record<string, unknown>).sort(([a], [b]) => a.localeCompare(b)))
    : item)
}

// Состояние инструмента, которое хранится в смете. current — что сейчас на экране (undefined — нечего
// хранить): изменения уходят в onSave. saved — что лежит в смете: пришедшее извне (черновик с сервера
// после входа) загружается в инструмент через load. Своё же сохранение обратно не загружается.
export function useSavedSnapshot<T>(saved: T | undefined, current: T | undefined, onSave: (value: T | undefined) => void, load: (value: T | undefined) => void) {
  const savedJson = stableJson(saved)
  const currentJson = stableJson(current)
  const lastRef = useRef(savedJson)
  const seenSavedRef = useRef(savedJson)
  const handlers = useRef({ onSave, load, saved, current })
  handlers.current = { onSave, load, saved, current }

  useEffect(() => {
    if (savedJson !== seenSavedRef.current) {
      seenSavedRef.current = savedJson
      if (savedJson !== lastRef.current) {
        lastRef.current = savedJson
        handlers.current.load(handlers.current.saved)
      }
      return
    }
    if (currentJson !== lastRef.current) {
      lastRef.current = currentJson
      seenSavedRef.current = currentJson
      handlers.current.onSave(handlers.current.current)
    }
  }, [savedJson, currentJson])
}
