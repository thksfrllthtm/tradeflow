import { CopyCode } from '@/shared/components/CopyCode/CopyCode'
import { useEffect, useRef, useState } from 'react'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { decimalText, moneyText } from '@/shared/lib/rounding'
import { BudgetDistributionState } from '../../hooks/useBudgetDistribution'
import { BUDGET_MAX_GAP, BUDGET_MIN_GAP } from '../../lib/distributeBudget'
import './BudgetTool.css'

const money = (value: number) => formatRuNumber(moneyText(value))

const hint = (min: string, max: string, over: boolean) => `Цены подбираются так, чтобы «Растаможка к оплате» была на ${min}–${max} USD ${over ? 'больше бюджета (перерасход)' : 'меньше бюджета (недостача)'}. По умолчанию — недостача ${BUDGET_MIN_GAP}–${BUDGET_MAX_GAP} USD. `
  + 'Строки с ценой (из Excel, по ИТП, вручную) не меняются; строки без цены с тем же кодом ТН ВЭД получают ту же цену. '
  + 'Откат — «Отменить».'

// Бюджет растаможки и две кнопки подбора цен — компактно, для панели инструментов таблицы.
// Разница с бюджетом (недостача или перерасход, по умолчанию недостача 35–45) нужна редко — она
// в окошке за значком рядом с бюджетом. Цены проставляются сразу; откат — «Отменить».
export function BudgetControls({ budget, disabled }: { budget: BudgetDistributionState; disabled: boolean }) {
  const amount = Number(budget.budget.replace(',', '.'))
  const valid = /^\d+([.,]\d+)?$/.test(budget.budget.trim()) && amount > 0
  const busy = disabled || budget.running !== null
  const over = budget.gapMode === 'over'
  const HINT = hint(budget.gapMin, budget.gapMax, over)
  const gapValid = budget.gap !== null
  return <div className="budget-controls">
    <label className="budget-field" title={HINT}>
      <span>Бюджет</span>
      <span className="budget-input"><input inputMode="decimal" value={budget.budget} onChange={(event) => budget.setBudget(event.target.value.replace(',', '.'))} onKeyDown={(event) => { if (event.key === 'Enter' && valid && gapValid && !busy) { event.preventDefault(); void budget.run('math') } }} placeholder="сумма" aria-label="Бюджет растаможки, USD; Enter — подобрать цены" /><i>USD</i></span>
    </label>
    <GapPicker budget={budget} amount={valid ? amount : null} />
    <button type="button" className="budget-run" onClick={() => void budget.run('math')} disabled={!valid || !gapValid || busy} title={`${HINT}\nПропорции: история цен из прошлых смет, вес нетто единицы или поровну.`}>{budget.running === 'math' ? `Подбираем… ${budget.evaluations}` : 'Подобрать цены'}</button>
    <button type="button" className="budget-ai" onClick={() => void budget.run('ai')} disabled={!valid || !gapValid || busy} title={`${HINT}\n«По товарам»: каждый товар оценивается отдельно (DeepSeek), у цены — пояснение. Разные товары одного кода получают разные цены, одинаковый товар (тот же код и название) — одну. Обычно 2–5 секунд; повторный подбор для тех же товаров — сразу.`}>{budget.running === 'ai' ? (budget.evaluations ? `Подбираем… ${budget.evaluations}` : 'Оцениваем товары…') : 'По товарам'}</button>
  </div>
}

const usd = (value: number) => formatRuNumber(decimalText(value, 2))

// Значок «недостача 35–45 ▾» и окошко настройки разницы с бюджетом.
function GapPicker({ budget, amount }: { budget: BudgetDistributionState; amount: number | null }) {
  // Окошко — поверх страницы (position: fixed) под значком: карточка таблицы обрезает всё, что
  // выходит за её край. Прокрутка или изменение окна закрывают его.
  const [open, setOpen] = useState<{ left: number; top: number } | null>(null)
  const box = useRef<HTMLDivElement>(null)
  const chip = useRef<HTMLButtonElement>(null)
  const toggle = () => {
    if (open) { setOpen(null); return }
    const rect = chip.current?.getBoundingClientRect()
    if (rect) setOpen({ left: Math.max(8, Math.min(rect.left, window.innerWidth - 296)), top: rect.bottom + 6 })
  }
  const over = budget.gapMode === 'over'
  const gap = budget.gap
  const isDefault = budget.gapMin === String(BUDGET_MIN_GAP) && budget.gapMax === String(BUDGET_MAX_GAP) && !over

  useEffect(() => {
    if (!open) return
    const close = (event: Event) => { if (!box.current?.contains(event.target as Node)) setOpen(null) }
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') setOpen(null) }
    const dismiss = () => setOpen(null)
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', escape)
    document.addEventListener('scroll', close, true)
    window.addEventListener('resize', dismiss)
    return () => {
      document.removeEventListener('mousedown', close); document.removeEventListener('keydown', escape)
      document.removeEventListener('scroll', close, true); window.removeEventListener('resize', dismiss)
    }
  }, [open])

  // Пример для понимания: бюджет → итог (или условный бюджет 11 000, пока сумма не введена).
  const example = amount ?? 11000
  const range = gap ? (over ? [example + gap.min, example + gap.max] : [example - gap.max, example - gap.min]) : null
  const label = gap ? `${over ? 'перерасход' : 'недостача'} ${budget.gapMin}–${budget.gapMax}` : 'разница?'
  return <div className="budget-gap" ref={box}>
    <button type="button" ref={chip} className={['budget-gap-chip', open ? 'open' : '', gap ? '' : 'invalid', isDefault ? '' : 'changed'].filter(Boolean).join(' ')} onClick={toggle} aria-expanded={Boolean(open)} aria-haspopup="dialog" title="Разница «Растаможки к оплате» с бюджетом">
      {label}<svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><path d="M2.5 4l2.5 2.5L7.5 4" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" /></svg>
    </button>
    {open && <div className="budget-gap-pop" role="dialog" aria-label="Разница с бюджетом" style={{ left: open.left, top: open.top }}>
      <p className="budget-gap-title">Итог «Растаможка к оплате»</p>
      <div className="budget-gap-seg" role="radiogroup" aria-label="Итог меньше или больше бюджета">
        <button type="button" role="radio" aria-checked={!over} className={!over ? 'active' : undefined} onClick={() => budget.setGapMode('under')}>Меньше бюджета</button>
        <button type="button" role="radio" aria-checked={over} className={over ? 'active' : undefined} onClick={() => budget.setGapMode('over')}>Больше бюджета</button>
      </div>
      <div className="budget-gap-range">
        <span>на</span>
        <input inputMode="decimal" value={budget.gapMin} onChange={(event) => budget.setGapMin(event.target.value.replace(',', '.'))} aria-label="Разница с бюджетом от, USD" />
        <span>–</span>
        <input inputMode="decimal" value={budget.gapMax} onChange={(event) => budget.setGapMax(event.target.value.replace(',', '.'))} aria-label="Разница с бюджетом до, USD" />
        <span>USD</span>
      </div>
      <p className={gap ? 'budget-gap-example' : 'budget-gap-example invalid'}>
        {gap && range ? <>{amount ? 'Бюджет' : 'Например, бюджет'} {usd(example)} → итог {usd(range[0])}–{usd(range[1])}</> : 'Числа не меньше нуля, «от» не больше «до»'}
      </p>
      <div className="budget-gap-foot">
        <button type="button" className="budget-gap-link" onClick={budget.resetGap} disabled={isDefault}>По умолчанию: меньше на {BUDGET_MIN_GAP}–{BUDGET_MAX_GAP}</button>
        <button type="button" className="budget-gap-done" onClick={() => setOpen(null)}>Готово</button>
      </div>
    </div>}
  </div>
}

// Итог подбора, пояснение и предупреждение; пусто, пока подбора не было.
export function BudgetStatus({ budget }: { budget: BudgetDistributionState }) {
  const { summary } = budget
  const { explanations } = budget
  if (!summary && !budget.notice && !budget.warning && explanations.length === 0) return null
  const count = explanations.length % 100 >= 11 && explanations.length % 100 <= 14 ? 5 : explanations.length % 10
  const productsLabel = count === 1 ? 'товар' : count >= 2 && count <= 4 ? 'товара' : 'товаров'
  return <div className="budget-status">
    {summary && <p className="budget-summary" role="status">
      Растаможка к оплате <b>{money(summary.total)}</b> USD из {money(summary.budget)} · {summary.gap >= 0 ? 'недостача' : 'перерасход'} <b>{money(Math.abs(summary.gap))}</b> USD
      <span> · строк с заданной ценой: {summary.fixedRows} · пробных расчётов: {summary.evaluations}</span>
    </p>}
    {budget.notice && <p className="budget-notice">{budget.notice}</p>}
    {budget.warning && <p className="budget-warning" role="alert">{budget.warning}</p>}
    {explanations.length > 0 && <details className="ai-explanations" open>
      <summary>Пояснения к ценам <span>· {explanations.length} {productsLabel}</span></summary>
      <ul>
        {explanations.map((explanation) => <li key={`${explanation.code}-${explanation.rows.join(',')}`}>
          <CopyCode code={explanation.code} />
          <span className="ai-names" title={explanation.names}>{explanation.names}</span>
          <b>{explanation.price} USD</b>
          <span className="ai-reason">{explanation.reason}</span>
          <small>{explanation.rows.length === 1 ? `строка ${explanation.rows[0]}` : `строки ${explanation.rows.join(', ')}`}</small>
        </li>)}
      </ul>
    </details>}
  </div>
}

// Метка «оценка» у цены из подбора по товарам; пояснение — при наведении и в списке «Пояснения к ценам».
export function BudgetNote({ note }: { note?: string }) {
  return note ? <span className="rs-tag ai" title={`Цена — подбор по товарам: ${note}`} aria-label={`Цена — подбор по товарам: ${note}`}>оценка</span> : null
}
