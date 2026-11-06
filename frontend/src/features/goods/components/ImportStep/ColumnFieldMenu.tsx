import { ReactNode, useEffect, useRef, useState } from 'react'
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

// «Данные» (data1…data5) — одним пунктом со своим списком внутри: свойства товара (материал, модель, назначение…),
// которые уходят ИИ при подборе кода с подписью из заголовка колонки.
const DATA = /^data\d+$/
const dataLabel: Record<string, string> = { data1: 'Первое', data2: 'Второе', data3: 'Третье', data4: 'Четвёртое', data5: 'Пятое' }
const dataHint = 'Свойства товара из этой колонки (материал, модель, назначение…) получит ИИ при подборе кода'

// Список «что в этой колонке» под заголовком колонки файла (лист «Как в Excel»). Закрывается кликом мимо,
// Esc и прокруткой (он привязан к месту заголовка).
export function ColumnFieldMenu({ anchor, fields, chosen, holder, ignoreSelector, onChoose, onClose }: Props) {
  const menuRef = useRef<HTMLDivElement>(null)
  const closeRef = useRef(onClose)
  closeRef.current = onClose
  // Список «Данные» открыт сразу, если колонке уже назначено одно из них.
  const [dataOpen, setDataOpen] = useState(DATA.test(chosen))
  const data = fields.filter((field) => DATA.test(field.value))

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

  function option(field: ColumnField, label: ReactNode, title?: string) {
    const other = holder(field.value)
    return <button key={field.value} type="button" role="option" aria-selected={chosen === field.value} className={chosen === field.value ? 'active' : undefined} onClick={() => onChoose(field.value)} title={title}>
      <span>{label}</span>{other && <small>сейчас: {other}</small>}
    </button>
  }

  return <div ref={menuRef} className="xp-menu" role="listbox" aria-label={`Что в колонке «${anchor.header}»`}
    style={{ left: Math.max(8, Math.min(anchor.left, window.innerWidth - 268)), top: anchor.top + 2, minWidth: Math.max(anchor.width, 220) }}>
    <div className="xp-menu-head">{anchor.header}</div>
    <button type="button" role="option" aria-selected={!chosen} className={!chosen ? 'active' : undefined} onClick={() => onChoose('')}>Не использовать</button>
    {fields.map((field) => {
      if (!DATA.test(field.value)) return option(field, field.label)
      if (field !== data[0]) return null
      const inGroup = DATA.test(chosen)
      return <div key="data" className="xp-menu-names">
        <button type="button" className={inGroup ? 'xp-menu-group has-chosen' : 'xp-menu-group'} aria-expanded={dataOpen} onClick={() => setDataOpen((open) => !open)} title={dataHint}>
          <span>Данные</span>
          <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"><path d="M3 4.5l3 3 3-3" /></svg>
        </button>
        {dataOpen && <div className="xp-menu-sub" role="group" aria-label="Данные">
          {data.map((field) => option(field, dataLabel[field.value] ?? field.label, dataHint))}
        </div>}
      </div>
    })}
  </div>
}
