import { useEffect, useState } from 'react'

// Значение, которое обновляется только после паузы во вводе (например, чтобы поиск
// не отправлял запрос на каждую букву).
export function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delayMs)
    return () => window.clearTimeout(timer)
  }, [value, delayMs])
  return debounced
}
