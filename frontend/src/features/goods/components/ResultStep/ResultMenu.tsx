import { MouseEvent, useEffect, useRef } from 'react'

type Props = {
  loading: boolean
  // «Сверка по ИТП» уже задана в смете.
  itpSet: boolean
  allColumns: boolean
  onCompare: () => void
  onItp: () => void
  onAllColumns: (value: boolean) => void
}

// Меню «⋯» — проверки и вид; подбор и откат цен — в стрелке кнопки «Подобрать цены ИИ».
export function ResultMenu({ loading, itpSet, allColumns, onCompare, onItp, onAllColumns }: Props) {
  const menuRef = useRef<HTMLDetailsElement>(null)

  // Меню «⋯» закрывается щелчком мимо и клавишей Esc.
  useEffect(() => {
    const close = (event: Event) => {
      const menu = menuRef.current
      if (!menu?.open) return
      if (event instanceof globalThis.KeyboardEvent) {
        if (event.key !== 'Escape') return
        menu.open = false
        menu.querySelector('summary')?.focus()
      } else if (!event.composedPath().includes(menu)) menu.open = false
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', close)
    return () => { document.removeEventListener('mousedown', close); document.removeEventListener('keydown', close) }
  }, [])

  function closeMenuOnItem(event: MouseEvent<HTMLElement>) {
    if ((event.target as HTMLElement).closest('.rs-menu-item') && menuRef.current) menuRef.current.open = false
  }

  return <details className="rs-more" ref={menuRef}>
    <summary className="icon-button" title="Ещё действия" aria-label="Ещё действия">
      <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true" fill="currentColor"><circle cx="3.5" cy="8" r="1.3" /><circle cx="8" cy="8" r="1.3" /><circle cx="12.5" cy="8" r="1.3" /></svg>
    </summary>
    <div className="rs-menu" onClick={closeMenuOnItem}>
      <p className="rs-menu-label">Проверки</p>
      <button type="button" className="rs-menu-item" onClick={onCompare} disabled={loading} title="Сравнить инвойс и СУР: итоги и каждая строка рядом">Сравнить с СУР</button>
      <button type="button" className="rs-menu-item" onClick={onItp}>Сверка по ИТП{itpSet ? ' · задана' : ''}</button>
      <hr />
      <p className="rs-menu-label">Вид</p>
      <label className="rs-menu-item rs-menu-check"><input type="checkbox" checked={allColumns} onChange={(event) => onAllColumns(event.target.checked)} />Показать все колонки</label>
    </div>
  </details>
}
