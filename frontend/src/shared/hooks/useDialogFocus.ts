import { useEffect, useRef, useState } from 'react'

// Окно при открытии забирает фокус (если внутри ещё ничего не в фокусе, например поле с autoFocus),
// а при закрытии возвращает его туда, где он был (обычно на кнопку, открывшую окно).
// Прежний фокус запоминается при первом рендере — до того, как autoFocus перенесёт его в поле окна.
export function useDialogFocus<T extends HTMLElement>() {
  const ref = useRef<T>(null)
  const [previous] = useState(() => document.activeElement instanceof HTMLElement ? document.activeElement : null)
  useEffect(() => {
    if (ref.current && !ref.current.contains(document.activeElement)) ref.current.focus({ preventScroll: true })
    return () => { returnFocus(previous) }
  }, [previous])
  return ref
}

// Кнопка в закрытом меню (<details>) невидима — фокус уходит на само меню («⋯»).
function returnFocus(element: HTMLElement | null) {
  if (!element?.isConnected) return
  const menu = element.closest('details')
  const target = menu && !menu.open ? menu.querySelector('summary') ?? element : element
  target.focus({ preventScroll: true })
}
