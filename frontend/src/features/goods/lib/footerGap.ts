// Строка «Итого» — внизу области таблицы, даже когда строк мало: область прокрутки занимает всю высоту до низа
// окна, а итог сдвигается вниз на пустое место под строками. Строк много — сдвига нет, итог прилипает к низу
// при прокрутке (position: sticky у tfoot).
export function footerGap(scrollerHeight: number, tableHeight: number) {
  return Math.max(0, Math.floor(scrollerHeight - tableHeight))
}

// Следит за размерами области и таблицы; возвращает функцию отписки.
export function trackFooterGap(scroller: HTMLElement, table: HTMLTableElement) {
  const update = () => {
    const foot = table.tFoot
    if (!foot) return
    // transform не меняет offsetHeight — высота таблицы без сдвига.
    const gap = footerGap(scroller.clientHeight, table.offsetHeight)
    foot.style.transform = gap > 0 ? `translateY(${gap}px)` : ''
  }
  const observer = new ResizeObserver(update)
  observer.observe(scroller)
  observer.observe(table)
  update()
  return () => observer.disconnect()
}
