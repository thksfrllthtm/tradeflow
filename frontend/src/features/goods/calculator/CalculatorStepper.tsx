import { GoodsCalculationResult, GoodsItemInput, GoodsParams } from '@/shared/types/goods'
import { plural } from '@/shared/lib/plural'
import { rowProblems } from '../lib/errorGroups'
import { ReviewCheck } from '../lib/reviewCheck'
import { reviewStatus, rowMissing } from '../lib/reviewStatus'

type Step = 1 | 2 | 3

type Props = {
  step: Step
  setStep: (step: Step) => void
  items: GoodsItemInput[]
  result: GoodsCalculationResult | null
  params: GoodsParams
  reviewCheck: ReviewCheck | null
  hasSource: boolean
}

// Этапы сметы в строке заголовка: «✓ Файл · ✓ Проверка · Результат».
export function CalculatorStepper({ step, setStep, items, result, params, reviewCheck, hasSource }: Props) {
  // На этапе проверки строк — красный счётчик строк, требующих внимания (те же проверки, что в таблице).
  const reviewStep = step === 2
  const reviewAttention = reviewStep ? items.filter((item, index) => reviewStatus(item, result?.items[index], result ? undefined : reviewCheck?.rows[item.id ?? ''], rowMissing(item, params.packageWeightKg)).attention).length : 0
  const codeProblemRows = result ? result.items.filter((item, index) => !item.excluded && rowProblems(item, item.code || items[index]?.code || '').some((problem) => problem.kind === 'code')).length : 0
  const stepList: { value: Step; label: string; available: boolean }[] = [
    { value: 1, label: 'Файл', available: true },
    { value: 2, label: 'Проверка строк', available: hasSource },
    { value: 3, label: 'Результат', available: Boolean(result) },
  ]
  return <nav className="est-steps compact" aria-label="Этапы расчёта товаров">
    {stepList.map(({ value, label, available }) => {
      // «Проверка» завершена, только если у всех строк есть код и ставки; иначе — ⚠ и число таких строк.
      const unfinished = value === 2 && step === 3 && codeProblemRows > 0
      const done = value < step && available && !unfinished
      const title = unfinished ? `${label}: ${codeProblemRows} ${plural(codeProblemRows, 'строка', 'строки', 'строк')} без кода ТН ВЭД или ставок` : label
      return <button type="button" key={value} className={step === value ? 'active' : done ? 'done' : unfinished ? 'unfinished' : undefined} aria-current={step === value ? 'step' : undefined} onClick={() => available && setStep(value)} disabled={!available} title={title}>
        <span className="est-step-num">{unfinished ? '⚠' : done ? '✓' : value}</span><span className="est-step-label">{label}</span>
        {unfinished && <b className="est-step-count" aria-label={title}>{codeProblemRows}</b>}
        {value === 2 && reviewStep && reviewAttention > 0 && <b className="est-step-count bad" title={`Требуют внимания: ${reviewAttention}`}>{reviewAttention}</b>}
      </button>
    })}
  </nav>
}
