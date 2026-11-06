import { useEffect, useLayoutEffect, useState } from 'react'

// Отступ панели сверху: под строкой сметы с названием и кнопками (этап проверки), иначе под шапкой
// сайта. Берётся место на странице, а не в окне, — при прокрутке панель не двигается (раньше
// догоняла уезжающую шапку и дёргалась).
// Уже 1440 px (как в agent.css) таблица не сдвигается и панель ложится поверх неё — тогда верх панели под
// рядом фильтров над таблицей: кнопки расчёта, фильтры, «Подобрать коды ИИ» и поиск остаются доступны.
export function usePanelTop() {
  const [top, setTop] = useState(0)
  useLayoutEffect(() => {
    const wide = window.matchMedia('(min-width: 1440px)')
    const update = () => {
      const anchor = (!wide.matches && (document.querySelector<HTMLElement>('.rv-step .rv-panel') ?? document.querySelector<HTMLElement>('.rv-calcbar')))
        || document.querySelector<HTMLElement>('.rv-titlebar') || document.querySelector<HTMLElement>('.app-header')
      if (anchor) setTop(anchor.getBoundingClientRect().bottom + window.scrollY)
    }
    update()
    const observer = new ResizeObserver(update)
    document.querySelectorAll<HTMLElement>('.app-header, .rv-titlebar, .rv-calcbar, .rv-step .rv-panel').forEach((element) => observer.observe(element))
    wide.addEventListener('change', update)
    return () => { observer.disconnect(); wide.removeEventListener('change', update) }
  }, [])
  return top
}

// Ширина панели: тянется за левый край (или ←/→ на ручке), запоминается. Пока панель открыта, ширина —
// в --ag-width у <html>: по ней шаги сметы оставляют место справа.
const widthKey = 'tradeflow.agentPanelWidth'
export const minWidth = 360, maxWidth = 640
const defaultWidth = 440
export const clampWidth = (value: number) => Math.min(maxWidth, Math.max(minWidth, Math.round(value)))

export function usePanelWidth() {
  const [width, setWidth] = useState(() => {
    try { const saved = Number(localStorage.getItem(widthKey)); return saved ? clampWidth(saved) : defaultWidth } catch { return defaultWidth }
  })
  useLayoutEffect(() => {
    const root = document.documentElement
    root.style.setProperty('--ag-width', `${width}px`)
    return () => { root.style.removeProperty('--ag-width') }
  }, [width])
  useEffect(() => {
    try { localStorage.setItem(widthKey, String(width)) } catch { /* без сохранения */ }
  }, [width])
  return [width, setWidth] as const
}
