import { RefObject, useCallback, useEffect, useLayoutEffect } from 'react'
import { trackFooterGap } from '../../lib/footerGap'
import { spaceBelow } from '../../lib/spaceBelow'

// Таблица и панель строки — до низа окна, как лист этапа проверки: строки прокручиваются внутри, шапка
// и «Итого» всегда на виду. Мерим блок «таблица + панель» (.rs-layout): --rs-top — где он начинается,
// --rs-after — что под ним (подпись «Предварительный расчёт» и нижний отступ страницы), --rs-around —
// строка фильтров и рамка карточки вокруг прокрутки.
export function useTableFit(tableWrapRef: RefObject<HTMLDivElement | null>, tableRef: RefObject<HTMLTableElement | null>) {
  const measureTable = useCallback(() => {
    const wrap = tableWrapRef.current
    const layout = wrap?.closest<HTMLElement>('.rs-layout')
    const card = wrap?.closest('.rs-card')
    if (!wrap || !layout || !card) return
    const box = wrap.getBoundingClientRect()
    const area = layout.getBoundingClientRect()
    const frame = card.getBoundingClientRect()
    layout.style.setProperty('--rs-top', `${area.top + window.scrollY}px`)
    layout.style.setProperty('--rs-after', `${spaceBelow(layout)}px`)
    layout.style.setProperty('--rs-around', `${box.top - frame.top + frame.bottom - box.bottom}px`)
  }, [tableWrapRef])
  useLayoutEffect(measureTable)
  // Высота страницы сменилась без рендера этапа (раскрылась плашка ошибок, догрузились шрифты и фото) — тоже
  // перемер: иначе таблица останется по старой высоте и под ней будет пустота. Окно — для высоты окна.
  useEffect(() => {
    const page = tableWrapRef.current?.closest('.goods-page')
    const observer = new ResizeObserver(measureTable)
    if (page) observer.observe(page)
    window.addEventListener('resize', measureTable)
    return () => { observer.disconnect(); window.removeEventListener('resize', measureTable) }
  }, [measureTable, tableWrapRef])
  // «Итого» — внизу области таблицы, даже когда строк мало.
  useEffect(() => {
    const wrap = tableWrapRef.current, table = tableRef.current
    return wrap && table ? trackFooterGap(wrap, table) : undefined
  }, [tableWrapRef, tableRef])
}
