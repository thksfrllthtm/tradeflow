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

// Один баннер над итогами: проблема — строка со счётчиком строк и кнопкой действия; номера строк —
// в раскрывающейся части (номер — переход к строке). Ошибки (нет кода, ставок) — красные, нет цены —
// следующий шаг (жёлтый), предупреждения — жёлтые.
export function CalculationErrors({ groups, onBackToReview, onPickPrices, onShowRow }: Props) {
  if (groups.length === 0) return null
  const blocking = groups.some((group) => group.tone === 'error')
  const withRows = groups.filter((group) => group.rows.length > 0)

  function action(group: ProblemGroup) {
    // Нет кода — главное действие экрана: зелёная кнопка.
    if (group.kind === 'code') return <button type="button" onClick={onBackToReview}>Перейти к проверке строк</button>
    if (group.kind === 'price' && onPickPrices) return <button type="button" className="secondary" onClick={onPickPrices}>Подобрать цены ИИ</button>
    return null
  }

  return (
    <section className="rs-problems" aria-label="Что мешает завершить расчёт" role={blocking ? 'alert' : 'status'}>
      <ul className="rs-problems-list">
        {groups.map((group) => <li key={group.text} className={group.tone}>
          <span className="rs-problem-icon" aria-hidden="true">{ICON[group.tone]}</span>
          <span className="rs-problem-text"><b>{group.text}</b>{group.rows.length > 0 && <> — {rowsText(group.rows.length)}</>}</span>
          {action(group)}
        </li>)}
      </ul>
      {withRows.length > 0 && <details className="rs-problems-rows">
        <summary>Показать строки</summary>
        <ul>
          {withRows.map((group) => <li key={group.text}>
            <span>{group.text}:</span>{' '}
            {rowRangeParts(group.rows).map((part, index) => <span key={part.label}>{index > 0 && ', '}<button type="button" className="button-link" onClick={() => onShowRow(part.first)} title={`Показать строку ${part.first}`}>№ {part.label}</button></span>)}
          </li>)}
        </ul>
      </details>}
    </section>
  )
}
