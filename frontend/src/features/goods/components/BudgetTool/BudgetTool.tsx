import { useEffect, useRef, useState } from 'react'
import { formatRuNumber } from '@/shared/lib/formatDisplayNumber'
import { decimalText, moneyText } from '@/shared/lib/rounding'
import { BudgetDistributionState } from '../../hooks/useBudgetDistribution'
import { NumberField } from '@/shared/components/NumberField/NumberField'
import { BUDGET_MAX_GAP, BUDGET_MIN_GAP } from '../../lib/distributeBudget'
import './BudgetTool.css'

const money = (value: number) => formatRuNumber(moneyText(value))

const hint = (min: string, max: string, over: boolean) => `Цены подбираются так, чтобы «Растаможка к оплате» была на ${min}–${max} USD ${over ? 'больше кассы (перерасход)' : 'меньше кассы (недостача)'}. По умолчанию — недостача ${BUDGET_MIN_GAP}–${BUDGET_MAX_GAP} USD. `
  + 'Строки с ценой (из Excel, по ИТП, вручную) не меняются; строки без цены с тем же кодом ТН ВЭД получают ту же цену. '
  + 'Откат — «Отменить».'

// Сумма бюджета, если введена правильно; иначе null.
export function budgetAmount(budget: BudgetDistributionState) {
  const text = budget.budget.trim()
  return /^\d+([.,]\d+)?$/.test(text) && Number(text.replace(',', '.')) > 0 ? Number(text.replace(',', '.')) : null
}

export const budgetHint = (budget: BudgetDistributionState) => hint(budget.gapMin, budget.gapMax, budget.gapMode === 'over')

const AI_RUN_HINT = '«По товарам»: каждый товар оценивается отдельно (DeepSeek), у цены — пояснение. Разные товары одного кода получают разные цены, одинаковый товар (тот же код и название) — одну. Обычно 2–5 секунд; повторный подбор для тех же товаров — сразу.'

// Касса в карточке итогов: «9 456,49 / [9 500] USD» и остаток; итог последнего подбора — в подсказке у остатка.
// Подбор цен — кнопка «Подобрать цены ИИ», распределение под бюджет и откат — в её стрелке.
export function BudgetMeter({ budget, totalUsd }: { budget: BudgetDistributionState; totalUsd: number | null }) {
  const amount = budgetAmount(budget)
  const rest = amount !== null && totalUsd !== null ? amount - totalUsd : null
  const { summary } = budget
  const summaryText = summary
    ? `Последний подбор: ${money(summary.total)} USD из ${money(summary.budget)} · ${summary.gap >= 0 ? 'недостача' : 'перерасход'} ${money(Math.abs(summary.gap))} USD · строк с заданной ценой: ${summary.fixedRows} · пробных расчётов: ${summary.evaluations}`
    : budgetHint(budget)
  return <div className="budget-meter">
    <div className="budget-meter-top">
      <label className="budget-field" title={budgetHint(budget)}>
        <span>Касса</span>
        {totalUsd !== null && amount !== null && <b>{money(totalUsd)} /</b>}
        <span className="budget-input"><NumberField id="rs-budget" value={budget.budget} onChange={budget.setBudget} placeholder="сумма" aria-label="Касса растаможки, USD" /><i>USD</i></span>
      </label>
      {rest !== null && <span className={rest < 0 ? 'budget-rest over' : 'budget-rest'} title={summaryText}>{rest >= 0 ? `ещё ${money(rest)}` : `перерасход ${money(-rest)}`}</span>}
    </div>
  </div>
}

// Текущие настройки подбора одной строкой: «Итог меньше бюджета на 35–45 USD · 4 знака».
export const budgetSettingsText = (budget: BudgetDistributionState) =>
  `Итог ${budget.gapMode === 'over' ? 'больше' : 'меньше'} кассы на ${budget.gapMin}–${budget.gapMax} USD · ${budget.priceDecimals} знака`

const usd = (value: number) => formatRuNumber(decimalText(value, 2))

export type RunMode = 'ai' | 'math'

type RunProps = {
  budget: BudgetDistributionState
  disabled: boolean
  // Главная (зелёная) кнопка — когда подбор цен и есть следующий шаг; иначе вторичная (белая).
  primary?: boolean
  // Почему подбор сейчас невозможен (нет кодов, у всех строк своя цена) — кнопки неактивны, причина в подсказке.
  blockedReason?: string
  // Запуск подбора: проверка бюджета и подтверждение замены цен — у вызывающего.
  onRun: (mode: RunMode) => void
  onUndo: () => void
  canUndo: boolean
  onRestore: () => void
  // Есть цены подбора, которые вернутся к ценам из файла; нет — «Вернуть цены из файла» неактивна.
  canRestore: boolean
}

const sparkle = <svg width="15" height="15" viewBox="0 0 16 16" aria-hidden="true" fill="currentColor"><path d="M8 1.5l1.3 3.6 3.7 1.4-3.7 1.4L8 11.5 6.7 7.9 3 6.5l3.7-1.4zM12.8 10.5l.6 1.6 1.6.6-1.6.6-.6 1.7-.6-1.7-1.6-.6 1.6-.6z" /></svg>

// Разделённая кнопка [✨ Подобрать цены ИИ | ▾]: основная часть — подбор по товарам (ИИ); стрелка — одна панель:
// «Распределить под бюджет» по пропорциям с настройками (итог меньше или больше бюджета, на сколько, пример,
// знаков в цене), «Отменить подбор цен», «Вернуть цены из файла». Закрывается щелчком мимо и Esc.
export function BudgetRunButton({ budget, disabled, primary, blockedReason, onRun, onUndo, canUndo, onRestore, canRestore }: RunProps) {
  const [open, setOpen] = useState(false)
  const box = useRef<HTMLDivElement>(null)
  const caret = useRef<HTMLButtonElement>(null)
  const over = budget.gapMode === 'over'
  const gap = budget.gap
  const twoDecimals = budget.priceDecimals === 2
  const isDefault = budget.gapMin === String(BUDGET_MIN_GAP) && budget.gapMax === String(BUDGET_MAX_GAP) && !over && twoDecimals
  const amount = budgetAmount(budget)
  const settings = budgetSettingsText(budget)
  const busy = disabled || budget.running !== null
  const runBlocked = Boolean(blockedReason) || busy || !gap

  useEffect(() => {
    if (!open) return
    const close = (event: MouseEvent) => { if (!box.current?.contains(event.target as Node)) setOpen(false) }
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') { setOpen(false); caret.current?.focus() } }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', escape)
    return () => { document.removeEventListener('mousedown', close); document.removeEventListener('keydown', escape) }
  }, [open])

  // Пример для понимания: бюджет → итог (или условный бюджет 11 000, пока сумма не введена).
  const example = amount ?? 11000
  const range = gap ? (over ? [example + gap.min, example + gap.max] : [example - gap.max, example - gap.min]) : null
  const act = (action: () => void) => () => { setOpen(false); action() }

  return <div className={primary ? 'budget-split primary' : 'budget-split'} ref={box}>
    <button type="button" className="budget-run" onClick={() => onRun('ai')} disabled={runBlocked} title={blockedReason ?? `${AI_RUN_HINT}\n${settings}`}>
      {sparkle}{budget.running === 'ai' ? (budget.evaluations ? `Подбираем… ${budget.evaluations}` : 'Оцениваем товары…') : budget.running === 'math' ? `Подбираем… ${budget.evaluations}` : 'Подобрать цены ИИ'}
    </button>
    <button type="button" className={open ? 'budget-caret open' : 'budget-caret'} ref={caret} onClick={() => setOpen(!open)} disabled={disabled} aria-expanded={open} aria-haspopup="dialog" title={`Распределить под кассу, отменить подбор, вернуть цены из файла. ${settings}`} aria-label="Другие способы подбора цен">
      <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true"><path d="M2.5 4l2.5 2.5L7.5 4" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" /></svg>
    </button>
    {open && <div className="budget-settings" role="dialog" aria-label="Подбор цен">
      <p className="budget-settings-title">Распределить под кассу</p>
      <p className="budget-settings-note">Без ИИ: по истории цен, весу нетто или поровну</p>
      <p className="budget-settings-label">Итог</p>
      <div className="budget-gap-seg" role="radiogroup" aria-label="Итог меньше или больше кассы">
        <button type="button" role="radio" aria-checked={!over} className={!over ? 'active' : undefined} onClick={() => budget.setGapMode('under')}>Меньше кассы</button>
        <button type="button" role="radio" aria-checked={over} className={over ? 'active' : undefined} onClick={() => budget.setGapMode('over')}>Больше кассы</button>
      </div>
      <div className="budget-gap-range">
        <span>на</span>
        <NumberField value={budget.gapMin} onChange={budget.setGapMin} aria-label="Разница с кассой от, USD" />
        <span>–</span>
        <NumberField value={budget.gapMax} onChange={budget.setGapMax} aria-label="Разница с кассой до, USD" />
        <span>USD</span>
      </div>
      <p className={gap ? 'budget-gap-example' : 'budget-gap-example invalid'}>
        {gap && range ? <>{amount ? 'Касса' : 'Например, касса'} {usd(example)} → итог {usd(range[0])}–{usd(range[1])}</> : 'Числа не меньше нуля, «от» не больше «до»'}
      </p>
      <div className="budget-settings-row">
        <span>Знаков в цене</span>
        <div className="budget-gap-seg" role="radiogroup" aria-label="Знаков после запятой в подобранной цене">
          <button type="button" role="radio" aria-checked={twoDecimals} className={twoDecimals ? 'active' : undefined} onClick={() => budget.setPriceDecimals(2)} title="Например, 0,62">2</button>
          <button type="button" role="radio" aria-checked={!twoDecimals} className={!twoDecimals ? 'active' : undefined} onClick={() => budget.setPriceDecimals(4)} title="Например, 0,6194">4</button>
        </div>
      </div>
      <div className="budget-gap-foot">
        <button type="button" className="budget-gap-link" onClick={budget.resetGap} disabled={isDefault}title={`По умолчанию: итог меньше кассы на ${BUDGET_MIN_GAP}–${BUDGET_MAX_GAP} USD, 2 знака`}>Сбросить к умолчанию</button>
        <button type="button" className="budget-gap-done" onClick={act(() => onRun('math'))} disabled={runBlocked} title={blockedReason ?? `${budgetHint(budget)}\n${settings}`}>Математический подбор</button>
      </div>
      <hr />
      <button type="button" className="budget-settings-item" onClick={act(onUndo)} disabled={!canUndo || busy} title={canUndo ? 'Вернуть цены до последнего подбора или сброса' : 'Подбора цен ещё не было'}>Отменить подбор цен</button>
      <button type="button" className="budget-settings-item" onClick={act(onRestore)} disabled={!canRestore || busy} title={canRestore ? 'Отменяет подбор под кассу: цены строк сметы — из файла и введённые вручную' : 'Подбора не было — цены и так из файла'}>Вернуть цены из файла</button>
    </div>}
  </div>
}

// Сообщение и предупреждение подбора. Итог — в подсказке полосы бюджета, пояснения цен — в панели строки.
export function BudgetStatus({ budget }: { budget: BudgetDistributionState }) {
  // «У всех строк цена уже задана…» не показываем: замену цен спрашивает подтверждение у кнопки подбора.
  const warning = budget.warning.startsWith('У всех строк цена уже задана') ? '' : budget.warning
  if (!budget.notice && !warning) return null
  return <div className="budget-status">
    {budget.notice && <p className="budget-notice">{budget.notice}</p>}
    {warning && <p className="budget-warning" role="alert">{warning}</p>}
  </div>
}

// Точка у цены, оценённой ИИ («По товарам»); пояснение — при наведении и в панели строки.
export function AiDot({ note }: { note?: string }) {
  return note ? <span className="ai-dot" title={`Цена оценена ИИ: ${note}`} role="img" aria-label="Цена оценена ИИ" /> : null
}
