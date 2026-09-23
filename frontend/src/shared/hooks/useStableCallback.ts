import { useCallback, useLayoutEffect, useRef } from 'react'

// Функция с постоянной ссылкой, которая всегда вызывает последнюю версию fn.
// Нужна, чтобы memo-компоненты (строки таблицы) не перерисовывались из-за
// обработчиков, которые родитель пересоздаёт на каждый рендер.
export function useStableCallback<Args extends unknown[], Result>(fn: (...args: Args) => Result): (...args: Args) => Result {
  const ref = useRef(fn)
  useLayoutEffect(() => { ref.current = fn })
  return useCallback((...args: Args) => ref.current(...args), [])
}
