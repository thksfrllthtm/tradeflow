import { useEffect, useRef } from 'react'
import './ColumnFieldMenu.css'

export type ColumnField = { value: string; label: string }
export type ColumnMenuAnchor = { header: string; left: number; top: number; width: number }

type Props = {
  anchor: ColumnMenuAnchor
  fields: ColumnField[]
  chosen: string
  // Колонка, у которой поле уже назначено: выбор здесь переносит его сюда.
  holder: (field: string) => string | undefined
  // Клик по этим элементам меню не закрывает: их обработчик сам переключит меню.
  ignoreSelector: string
  onChoose: (field: string) => void
  onClose: () => void
}

// Список «что в этой колонке» под заголовком колонки файла — один для обычной таблицы и для
// вида «Как в Excel». Закрывается кликом мимо, Esc и прокруткой (он привязан к месту заголовка).
export function ColumnFieldMenu({ anchor, fields, chosen, holder, ignoreSelector, onChoose, onClose }: Props) {
  const menuRef = useRef<HTMLDivElement>(null)
  const closeRef = useRef(onClose)
  closeRef.current = onClose

  useEffect(() => {
    const close = (event: Event) => {
      const target = event.target as HTMLElement
      if (menuRef.current?.contains(target)) return
      if (event.type === 'mousedown' && target.closest?.(ignoreSelector)) return
      closeRef.current()
    }
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') closeRef.current() }
    document.addEventListener('mousedown', close)
    document.addEventListener('scroll', close, true)
    window.addEventListener('resize', close)
    document.addEventListener('keydown', escape)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('scroll', close, true)
      window.removeEventListener('resize', close)
      document.removeEventListener('keydown', escape)
    }
  }, [ignoreSelector])

  return <div ref={menuRef} className="xp-menu" role="listbox" aria-label={`Что в колонке «${anchor.header}»`}
    style={{ left: Math.max(8, Math.min(anchor.left, window.innerWidth - 268)), top: anchor.top + 2, minWidth: Math.max(anchor.width, 220) }}>
    <div className="xp-menu-head">{anchor.header}</div>
    <button type="button" role="option" aria-selected={!chosen} className={!chosen ? 'active' : undefined} onClick={() => onChoose('')}>Не использовать</button>
    {fields.map((field) => {
      const other = holder(field.value)
      return <button key={field.value} type="button" role="option" aria-selected={chosen === field.value} className={chosen === field.value ? 'active' : undefined} onClick={() => onChoose(field.value)}>
        <span>{field.label}</span>{other && <small>сейчас: {other}</small>}
      </button>
    })}
  </div>
}
