import { formatDisplayNumber } from '@/shared/lib/formatDisplayNumber'
import { BudgetDistributionState } from '../../hooks/useBudgetDistribution'
import './BudgetTool.css'

const money = (value: number) => formatDisplayNumber(value.toFixed(2))

const HINT = 'Цены подбираются так, чтобы «Растаможка к оплате» была на 20–30 USD меньше бюджета. '
  + 'Строки с ценой (из Excel, по ИТП, вручную) не меняются; строки без цены с тем же кодом ТН ВЭД получают ту же цену. '
  + 'Откат — «Отменить».'

// Бюджет растаможки и две кнопки подбора цен — компактно, для панели инструментов таблицы.
// Цены проставляются сразу; откат — кнопкой возврата к предыдущему результату.
export function BudgetControls({ budget, disabled }: { budget: BudgetDistributionState; disabled: boolean }) {
  const amount = Number(budget.budget.replace(',', '.'))
  const valid = /^\d+([.,]\d+)?$/.test(budget.budget.trim()) && amount > 0
  const busy = disabled || budget.running !== null
  return <div className="budget-controls">
    <label title={HINT}>Бюджет растаможки, USD<input inputMode="decimal" value={budget.budget} onChange={(event) => budget.setBudget(event.target.value.replace(',', '.'))} placeholder="сумма" aria-label="Бюджет растаможки, USD" /></label>
    <button type="button" onClick={() => void budget.run('math')} disabled={!valid || busy} title={`${HINT}\nПропорции: история цен из прошлых смет, вес нетто единицы или поровну.`}>{budget.running === 'math' ? `Подбираем… ${budget.evaluations}` : 'Подобрать цены'}</button>
    <button type="button" className="budget-ai" onClick={() => void budget.run('ai')} disabled={!valid || busy} title={`${HINT}\nПропорции между кодами предлагает ИИ (DeepSeek).`}>{budget.running === 'ai' ? (budget.evaluations ? `Подбираем… ${budget.evaluations}` : 'Спрашиваем ИИ…') : 'Подобрать с ИИ'}</button>
  </div>
}

// Итог подбора, пояснение и предупреждение; пусто, пока подбора не было.
export function BudgetStatus({ budget }: { budget: BudgetDistributionState }) {
  const { summary } = budget
  const { explanations } = budget
  if (!summary && !budget.notice && !budget.warning && explanations.length === 0) return null
  const codesLabel = explanations.length === 1 ? 'код' : explanations.length < 5 ? 'кода' : 'кодов'
  return <div className="budget-status">
    {summary && <p className="budget-summary" role="status">
      Растаможка к оплате <b>{money(summary.total)}</b> USD из {money(summary.budget)} · {summary.gap >= 0 ? 'недостача' : 'перерасход'} <b>{money(Math.abs(summary.gap))}</b> USD
      <span> · строк с заданной ценой: {summary.fixedRows} · пробных расчётов: {summary.evaluations}</span>
    </p>}
    {budget.notice && <p className="budget-notice">{budget.notice}</p>}
    {budget.warning && <p className="budget-warning" role="alert">{budget.warning}</p>}
    {explanations.length > 0 && <details className="ai-explanations" open>
      <summary>Пояснения ИИ <span>· {explanations.length} {codesLabel} ТН ВЭД</span></summary>
      <ul>
        {explanations.map((explanation) => <li key={`${explanation.code}-${explanation.rows.join(',')}`}>
          <code>{explanation.code}</code>
          <span className="ai-names" title={explanation.names}>{explanation.names}</span>
          <b>{explanation.price} USD</b>
          <span className="ai-reason">{explanation.reason}</span>
          <small>{explanation.rows.length === 1 ? `строка ${explanation.rows[0]}` : `строки ${explanation.rows.join(', ')}`}</small>
        </li>)}
      </ul>
    </details>}
  </div>
}

// Полная панель (смета перчаток): управление и статус одним блоком.
export function BudgetTool({ budget, disabled }: { budget: BudgetDistributionState; disabled: boolean }) {
  return <div className="budget-tool">
    <BudgetControls budget={budget} disabled={disabled} />
    <BudgetStatus budget={budget} />
  </div>
}

// Значок «!» в правом верхнем углу поля цены, пришедшей от ИИ; пояснение — при наведении и в списке «Пояснения ИИ».
export function BudgetNote({ note }: { note?: string }) {
  return note ? <span className="budget-ai-mark" title={`ИИ: ${note}`} aria-label={`Пояснение ИИ: ${note}`}>!</span> : null
}
