import { useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { GlovesCalculationResult, GlovesItemInput } from '../types'
import { pricingApi, PriceGroupInput } from '../api/pricingApi'
import { BudgetGroup, budgetGroups, BudgetRowAccess, distributeBudget } from '../lib/distributeBudget'
import { GroupFacts, resolveWeights } from '../lib/budgetWeights'

export type BudgetMode = 'math' | 'ai'
export type BudgetSummary = { status: 'ok' | 'closest'; budget: number; total: number; gap: number; fixedRows: number; evaluations: number }
// Пояснение ИИ по одному коду ТН ВЭД: товары группы, итоговая цена единицы после подбора и причина.
export type AiExplanation = { code: string; names: string; price: string; reason: string; rows: number[] }

type ResultItem = GlovesCalculationResult['items'][number]
type Row = { result: ResultItem; item: GlovesItemInput }

const numberOf = (value: unknown) => {
  const text = String(value ?? '').replace(/\s/g, '').replace(',', '.')
  const parsed = Number(text)
  return text !== '' && Number.isFinite(parsed) ? parsed : null
}

// Строки результата. Цена строки — цена единицы в текущем результате: из Excel, введённая вручную
// или посчитанная по ИТП карточки товара. Такие строки распределение не меняет. Количество и вес
// берутся из результата (там они уже досчитаны сервером).
const access: BudgetRowAccess<Row> = {
  getCode: (row) => String(row.result.code || row.item.code || ''),
  getQuantity: (row) => numberOf(row.result.pairs),
  getPrice: (row) => numberOf(row.result.pricePerPair) || numberOf(row.result.pairPriceUsd) || numberOf(row.result.pairPrice),
  isIncluded: (row) => !row.result.excluded && !row.item.excluded,
  setPrice: (row, price) => ({ ...row, item: { ...row.item, pricePerPair: price } }),
}

// «Растаможка к оплате» в USD. null — итог неполный: часть строк не рассчитана.
function customsPaymentUsd(calculated: GlovesCalculationResult) {
  const { totals } = calculated
  const kzt = numberOf(totals.customsPaymentsKzt)
  const rate = numberOf(calculated.usdKzt)
  if (kzt === null || rate === null || !(rate > 0) || totals.calculatedCount < totals.totalCount) return null
  return kzt / rate
}

function sumOver(rows: Row[], group: BudgetGroup, value: (row: Row) => number | null) {
  return group.indexes.reduce((sum, index) => sum + (value(rows[index]) ?? 0), 0)
}

function describe(rows: Row[], group: BudgetGroup) {
  const names = [...new Set(group.indexes.flatMap((index) => [rows[index].item.name, rows[index].result.standardName].map((value) => String(value ?? '').trim()).filter(Boolean)))]
  return names.slice(0, 4).join('; ')
}

// Названия товаров группы для списка пояснений: только названия строк, без описания кода и повторов.
function groupNames(rows: Row[], group: BudgetGroup) {
  const names = [...new Set(group.indexes.map((index) => String(rows[index].item.name ?? '').replace(/\s+/g, ' ').trim()).filter(Boolean))]
  return names.join('; ')
}

// Распределение бюджета на этапе результата. apply получает строки с новыми ценами:
// калькулятор запоминает прежний результат для отката и пересчитывает без сохранения черновика.
export function useBudgetDistribution({ kind, estimateId, unit, result, items, preview, apply }: {
  kind: 'goods' | 'gloves'
  estimateId: number
  unit: string
  result: GlovesCalculationResult | null
  items: GlovesItemInput[]
  // Пробный расчёт строк без показа и сохранения — по нему подбираются цены.
  preview: (nextItems: GlovesItemInput[]) => Promise<GlovesCalculationResult>
  apply: (nextItems: GlovesItemInput[]) => void
}) {
  const [budget, setBudget] = useState('')
  const [running, setRunning] = useState<BudgetMode | null>(null)
  const [summary, setSummary] = useState<BudgetSummary | null>(null)
  const [warning, setWarning] = useState('')
  const [notice, setNotice] = useState('')
  const [notes, setNotes] = useState<Record<number, string>>({})
  const [explanations, setExplanations] = useState<AiExplanation[]>([])
  const [evaluations, setEvaluations] = useState(0)

  function reset() {
    setSummary(null); setWarning(''); setNotice(''); setNotes({}); setExplanations([])
  }

  async function run(mode: BudgetMode) {
    if (!result || running) return
    const amount = Number(budget.replace(',', '.'))
    reset()
    setEvaluations(0)
    setRunning(mode)
    try {
      // Строки, которых не коснётся распределение, сохраняют цену из текущего результата.
      const rows: Row[] = result.items.map((resultItem, index) => {
        const item = items[index] ?? ({} as GlovesItemInput)
        return { result: resultItem, item: { ...item, pricePerPair: resultItem.pricePerPair || item.pricePerPair || '' } }
      })
      const free = budgetGroups(rows, access).filter((group) => group.fixedPrice === null)
      const facts = new Map<string, GroupFacts>(free.map((group) => {
        const quantity = group.quantity
        const net = sumOver(rows, group, (row) => numberOf(row.result.netKg))
        return [group.key, { key: group.key, netPerUnit: net > 0 && quantity > 0 ? net / quantity : null }]
      }))

      // История цен из прошлых смет — для групп с кодом.
      const historyCodes = free.filter((group) => group.code).map((group) => group.code)
      if (historyCodes.length > 0) {
        try {
          const history = new Map((await pricingApi.priceHistory(kind, estimateId, historyCodes)).map((price) => [price.code, price.unitPrice]))
          free.forEach((group) => {
            const known = history.get(group.code)
            const fact = facts.get(group.key)
            if (fact && !fact.price && known) fact.price = known
          })
        } catch { /* без истории — вес нетто или поровну */ }
      }

      const nextNotes: Record<number, string> = {}
      const reasons = new Map<string, string>()
      if (mode === 'ai' && free.length > 0) {
        const aiGroups: PriceGroupInput[] = free.map((group) => ({
          code: group.code || group.key,
          description: describe(rows, group),
          quantity: group.quantity,
          unit,
          places: sumOver(rows, group, (row) => numberOf(row.result.places)) || undefined,
          grossKg: sumOver(rows, group, (row) => numberOf(row.result.grossKg)) || undefined,
          netKg: sumOver(rows, group, (row) => numberOf(row.result.netKg)) || undefined,
        }))
        try {
          const suggested = new Map((await pricingApi.suggestPrices(aiGroups)).map((price) => [price.code, price]))
          free.forEach((group) => {
            const price = suggested.get(group.code || group.key)
            const fact = facts.get(group.key)
            if (!price || !fact) return
            fact.price = price.unitPrice
            if (price.reason) {
              reasons.set(group.key, price.reason)
              group.indexes.forEach((index) => { nextNotes[index] = price.reason })
            }
          })
        } catch (reason) {
          setNotice(`ИИ недоступен (${errorMessage(reason)}), использовано математическое распределение`)
        }
      }

      const outcome = await distributeBudget(rows, amount, access, {
        weights: resolveWeights([...facts.values()]),
        evaluate: async (candidate) => customsPaymentUsd(await preview(candidate.map((row) => row.item))),
        onProgress: setEvaluations,
      })
      if (outcome.status === 'nothing') {
        setWarning(outcome.message ?? 'Распределять нечего')
        return
      }
      setNotes(nextNotes)
      setExplanations(free.filter((group) => reasons.has(group.key)).map((group) => ({
        code: group.code || '—',
        names: groupNames(rows, group),
        price: outcome.items[group.indexes[0]]?.item.pricePerPair ?? '',
        reason: reasons.get(group.key) ?? '',
        rows: group.indexes.map((index) => index + 1),
      })))
      setSummary({ status: outcome.status, budget: amount, total: outcome.total, gap: outcome.gap, fixedRows: outcome.fixedRows, evaluations: outcome.evaluations })
      if (outcome.status === 'closest') setWarning(outcome.message ?? '')
      apply(outcome.items.map((row) => row.item))
    } catch (reason) {
      setWarning(`Не удалось подобрать цены: ${errorMessage(reason)}`)
    } finally {
      setRunning(null)
    }
  }

  return { budget, setBudget, running, evaluations, summary, warning, notice, notes, explanations, run, reset }
}

export type BudgetDistributionState = ReturnType<typeof useBudgetDistribution>
