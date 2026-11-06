import { useState } from 'react'
import { ProblemGroup, plural, rowRangeParts } from './errorGroups'
import './CalculationErrors.css'

type Props = {
  groups: ProblemGroup[]
  onBackToReview: () => void
  // «Подобрать цены»; нет — кнопки нет (режим СУР).
  onPickPrices?: () => void
  onShowRow: (row: number) => void
}

const rowsText = (count: number) => `${count} ${plural(count, ['строка', 'строки', 'строк'])}`

const ICON: Record<ProblemGroup['tone'], string> = { error: '!', todo: '•', warn: '?' }

// Баннер над итогами — одна строка: первая (главная) проблема со счётчиком строк, «и ещё N» и кнопка действия.
// «Показать» раскрывает все проблемы с номерами строк (номер — переход к строке). Ошибки (нет кода, ставок) —
// красные, нет цены — следующий шаг (жёлтый), предупреждения — жёлтые.
export function CalculationErrors({ groups, onBackToReview, onPickPrices, onShowRow }: Props) {
  const [open, setOpen] = useState(false)
  if (groups.length === 0) return null
  const blocking = groups.some((group) => group.tone === 'error')
  const [first, ...rest] = groups
  const expandable = rest.length > 0 || first.rows.length > 0

  function action(group: ProblemGroup) {
    // Нет кода — главное действие экрана: зелёная кнопка.
    if (group.kind === 'code') return <button type="button" className="sm" onClick={onBackToReview}>Перейти к проверке строк</button>
    // Не «Подобрать цены ИИ»: кнопка с этим именем на экране одна — в полосе итогов.
    if (group.kind === 'price' && onPickPrices) return <button type="button" className="secondary sm" onClick={onPickPrices}>Подобрать цены</button>
    return null
  }

  const text = (group: ProblemGroup) => <><b>{group.text}</b>{group.rows.length > 0 && <> — {rowsText(group.rows.length)}</>}</>
  const rows = (group: ProblemGroup) => group.rows.length > 0 && <span className="rs-problem-rows">
    {rowRangeParts(group.rows).map((part, index) => <span key={part.label}>{index > 0 && ', '}<button type="button" className="button-link" onClick={() => onShowRow(part.first)} title={`Показать строку ${part.first}`}>№ {part.label}</button></span>)}
  </span>

  return (
    <section className="rs-problems" aria-label="Что мешает завершить расчёт" role={blocking ? 'alert' : 'status'}>
      <div className={`rs-problem-line ${first.tone}`}>
        <span className="rs-problem-icon" aria-hidden="true">{ICON[first.tone]}</span>
        <span className="rs-problem-text">
          {text(first)}
          {rest.length > 0 && !open && <> · и ещё {rest.length}</>}
          {expandable && <> · <button type="button" className="button-link rs-problem-toggle" onClick={() => setOpen(!open)} aria-expanded={open}>{open ? 'Скрыть' : 'Показать'}</button></>}
        </span>
        {action(first)}
      </div>
      {open && <ul className="rs-problems-list">
        {first.rows.length > 0 && <li className={first.tone}><span className="rs-problem-text">Строки: {rows(first)}</span></li>}
        {rest.map((group) => <li key={group.text} className={group.tone}>
          <span className="rs-problem-icon" aria-hidden="true">{ICON[group.tone]}</span>
          <span className="rs-problem-text">{text(group)}{group.rows.length > 0 && <>: {rows(group)}</>}</span>
          {action(group)}
        </li>)}
      </ul>}
    </section>
  )
}
