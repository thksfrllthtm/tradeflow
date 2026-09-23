import { useEffect, useRef } from 'react'

// Дополнительная полоса прокрутки над широкой таблицей строк: синхронизирована
// с таблицей и показывается только когда таблица шире контейнера.
export function useSyncedHorizontalScroll(active: boolean) {
  const tableWrapRef = useRef<HTMLDivElement | null>(null)
  const topScrollRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!active) return
    const wrap = tableWrapRef.current
    const top = topScrollRef.current
    const table = wrap?.querySelector('table')
    const spacer = top?.firstElementChild as HTMLElement | null
    if (!wrap || !top || !table || !spacer) return
    const update = () => {
      spacer.style.width = `${table.scrollWidth}px`
      top.style.display = table.scrollWidth > wrap.clientWidth + 1 ? 'block' : 'none'
    }
    update()
    const observer = new ResizeObserver(update)
    observer.observe(table)
    observer.observe(wrap)
    return () => observer.disconnect()
  }, [active])

  function syncHorizontalScroll(source: 'top' | 'body') {
    if (!tableWrapRef.current || !topScrollRef.current) return
    if (source === 'top') tableWrapRef.current.scrollLeft = topScrollRef.current.scrollLeft
    else topScrollRef.current.scrollLeft = tableWrapRef.current.scrollLeft
  }

  return { tableWrapRef, topScrollRef, syncHorizontalScroll }
}
