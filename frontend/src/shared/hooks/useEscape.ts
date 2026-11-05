import { useEffect } from 'react'
import { useStableCallback } from './useStableCallback'

// Открытые окна и панели — стек: Esc закрывает только последнее открытое (верхнее).
// ponytail: порядок = порядок монтирования; окно, открытое одновременно с родителем, окажется под ним.
const stack: Array<() => void> = []

type EscapeEvent = Pick<KeyboardEvent, 'key' | 'defaultPrevented' | 'preventDefault'>

export function handleEscape(event: EscapeEvent) {
  if (event.key !== 'Escape' || event.defaultPrevented || stack.length === 0) return
  event.preventDefault()
  stack[stack.length - 1]()
}

// Кладёт обработчик наверх стека; возвращает функцию, которая его убирает.
export function pushEscape(handler: () => void) {
  if (stack.length === 0 && typeof window !== 'undefined') window.addEventListener('keydown', handleEscape)
  stack.push(handler)
  return () => {
    const index = stack.lastIndexOf(handler)
    if (index >= 0) stack.splice(index, 1)
    if (stack.length === 0 && typeof window !== 'undefined') window.removeEventListener('keydown', handleEscape)
  }
}

export function useEscape(onEscape: () => void) {
  const handler = useStableCallback(onEscape)
  useEffect(() => pushEscape(handler), [handler])
}
