import { useState } from 'react'
import { GoodsCalculationResult } from '@/shared/types/goods'
import { errorGroups, plural, rowRangeParts } from './errorGroups'
import './CalculationErrors.css'

type Props = { result: GoodsCalculationResult; onBackToReview: () => void; onShowRow: (row: number) => void }

// Плашка над итогами: ошибки (строка не рассчитана) и предупреждения, одинаковые сообщения —
// одной строкой, номера строк — кнопки, которые прокручивают таблицу к строке. Длинный список
// свёрнут в одну строку, чтобы не забирать место у таблицы.
export function CalculationErrors({ result, onBackToReview, onShowRow }: Props) {
  const errors = errorGroups(result, 'errors')
  const warnings = errorGroups(result, 'warnings')
  const [open, setOpen] = useState(errors.length + warnings.length <= 2)
  if (errors.length === 0 && warnings.length === 0) return null
  const rowCount = (groups: typeof errors) => new Set(groups.flatMap((group) => group.rows)).size
  const errorRows = rowCount(errors)
  const warningRows = rowCount(warnings)
  const total = result.items.filter((item) => !item.excluded).length
  const counts = [
    errors.length > 0 && (errorRows > 0 ? `${errorRows} из ${total} ${plural(total, ['строки', 'строк', 'строк'])} с ошибками` : `${errors.length} ${plural(errors.length, ['общая ошибка', 'общие ошибки', 'общих ошибок'])}`),
    warningRows > 0 && `${warningRows} с предупреждениями`,
  ].filter(Boolean).join(' · ')

  return (
    <section className={errors.length > 0 ? 'rs-issues bad' : 'rs-issues warn'} aria-label="Ошибки и предупреждения расчёта">
      <div className="rs-issues-head">
        <p role={errors.length > 0 ? 'alert' : 'status'}>
          <strong>{errors.length > 0 ? 'Расчёт не завершён' : 'Проверьте предупреждения'}</strong>
          <span className="rs-issues-count">{counts}</span>
        </p>
        <button type="button" className="rs-issues-toggle" aria-expanded={open} onClick={() => setOpen(!open)}>
          {open ? 'Скрыть список' : `Показать список · ${errors.length + warnings.length}`}
        </button>
        {errors.length > 0 && <button type="button" className="button-link rs-issues-back" onClick={onBackToReview} title="Вернуться к проверке строк. Цены, изменённые на этапе результата, вернутся к ценам из Excel">К проверке строк (цены этапа 3 сбросятся)</button>}
      </div>
      {open && <ul className="rs-issues-list">
        {[...errors.map((group) => ({ ...group, kind: 'bad' })), ...warnings.map((group) => ({ ...group, kind: 'warn' }))].map((group) => (
          <li key={`${group.kind}:${group.text}`} className={group.kind}>
            <span className="rs-issues-text">{group.text}</span>
            <span className="rs-issues-rows">{group.rows.length > 0
              ? <><b>{group.rows.length} {plural(group.rows.length, ['строка', 'строки', 'строк'])}</b> №{' '}
                {rowRangeParts(group.rows).map((part, index) => <span key={part.label}>{index > 0 && ', '}<button type="button" onClick={() => onShowRow(part.first)} title={`Показать строку ${part.first} в таблице`}>{part.label}</button></span>)}</>
              : 'общая ошибка'}</span>
          </li>
        ))}
      </ul>}
    </section>
  )
}
