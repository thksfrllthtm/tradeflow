import { GlovesCalculationResult } from '../../../shared/types'
import { errorGroups, plural, rowRanges } from './errorGroups'
import './CalculationErrors.css'

// Плашка «Расчёт не завершён»: ошибки сгруппированы по тексту, у каждой — номера строк.
export function CalculationErrors({ result, onBackToReview }: { result: GlovesCalculationResult; onBackToReview: () => void }) {
  const errors = errorGroups(result)
  if (errors.length === 0) return null
  const errorRowCount = new Set(errors.flatMap((group) => group.rows)).size
  return (
    <details className="rs-errors" role="alert" open>
      <summary>
        <strong>Расчёт не завершён</strong>
        <span className="rs-errors-count">{errorRowCount > 0 ? `${errorRowCount} из ${result.items.length} строк с ошибками` : `${errors.length} ${plural(errors.length, ['ошибка', 'ошибки', 'ошибок'])}`}</span>
        <button type="button" className="button-link" onClick={(event) => { event.preventDefault(); onBackToReview() }}>Исправить на этапе 2 →</button>
      </summary>
      <ul className="rs-error-groups">
        {errors.map((group) => (
          <li key={group.text}>
            <span className="rs-error-text">{group.text}</span>
            <span className="rs-error-rows">{group.rows.length > 0 ? <><b>{group.rows.length} {plural(group.rows.length, ['строка', 'строки', 'строк'])}</b> № {rowRanges(group.rows)}</> : 'общая ошибка'}</span>
          </li>
        ))}
      </ul>
    </details>
  )
}
