import { useRef, useState } from 'react'
import { errorMessage } from '@/shared/lib/errorMessage'
import { GlovesCalculationResult, GlovesItemInput } from '../types'
import { pricingApi, PriceGroupInput, SuggestedPrice } from '../api/pricingApi'
import { BUDGET_MAX_GAP, BUDGET_MIN_GAP, BudgetGroup, budgetGroups, BudgetRowAccess, distributeBudget } from '../lib/distributeBudget'
import { GroupFacts, resolveWeights } from '../lib/budgetWeights'

export type BudgetMode = 'math' | 'ai'
export type BudgetSummary = { status: 'ok' | 'closest'; budget: number; total: number; gap: number; fixedRows: number; evaluations: number }
// Пояснение ИИ по одному коду ТН ВЭД: товары группы, итоговая цена единицы после подбора и причина.
export type AiExplanation = { code: string; names: string; price: string; reason: string; rows: number[] }

type ResultItem = GlovesCalculationResult['items'][number]

// Разница с бюджетом «от … до …» USD и её направление: недостача (итог меньше бюджета, по умолчанию)
// или перерасход (итог больше). По умолчанию недостача 20–30; браузер запоминает выбор.
export type GapMode = 'under' | 'over'
const gapStorageKey = 'tradeflow.budgetGap'
function storedGap(): { min: string; max: string; mode: GapMode } {
  try {
    const saved = JSON.parse(localStorage.getItem(gapStorageKey) ?? 'null') as { min?: unknown; max?: unknown; mode?: unknown } | null
    if (saved && typeof saved.min === 'string' && typeof saved.max === 'string') return { min: saved.min, max: saved.max, mode: saved.mode === 'over' ? 'over' : 'under' }
  } catch { /* нет хранилища — значения по умолчанию */ }
  return { min: String(BUDGET_MIN_GAP), max: String(BUDGET_MAX_GAP), mode: 'under' }
}

// Проверка недостачи: числа не меньше нуля, «от» не больше «до». null — ввод неверный.
export function parseGap(min: string, max: string): { min: number; max: number } | null {
  const valid = (value: string) => /^\d+([.,]\d+)?$/.test(value.trim())
  if (!valid(min) || !valid(max)) return null
  const low = Number(min.replace(',', '.'))
  const high = Number(max.replace(',', '.'))
  return low <= high ? { min: low, max: high } : null
}
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

// Почему пробный расчёт не дал итог: какие строки не посчитаны и с какой ошибкой (первые три).
function calculationProblem(calculated: GlovesCalculationResult) {
  const rate = numberOf(calculated.usdKzt)
  if (rate === null || !(rate > 0)) return 'не указан курс USD → KZT'
  const rows = calculated.items
    .map((item, index) => ({ item, row: index + 1 }))
    .filter(({ item }) => !item.excluded && item.status !== 'calculated')
  const shown = rows.slice(0, 3).map(({ item, row }) => `№ ${row} — ${[...(item.errors ?? []), ...(item.warnings ?? [])][0] ?? 'не рассчитана'}`)
  if (rows.length > shown.length) shown.push(`и ещё ${rows.length - shown.length}`)
  return shown.join('; ') || calculated.totals.blockingErrors?.[0] || ''
}

function sumOver(rows: Row[], indexes: number[], value: (row: Row) => number | null) {
  return indexes.reduce((sum, index) => sum + (value(rows[index]) ?? 0), 0)
}

const clean = (value: unknown) => String(value ?? '').replace(/\s+/g, ' ').trim()

// Название товара строки: выбранная карточка справочника товаров («русское / китайское»),
// иначе название строки.
function productName(row: Row) {
  const snapshot = row.item.directorySnapshot
  if (row.item.productDirectoryId && snapshot) {
    const both = [clean(snapshot.name), clean(snapshot.nameZh)].filter(Boolean).join(' / ')
    if (both) return both
  }
  return [row.item.name, row.item.supplierName, row.result.standardName].map(clean).find(Boolean) ?? ''
}

// Один и тот же товар — тот же код и та же карточка справочника товаров, а без карточки — то же
// название (без учёта регистра и пробелов). У одного товара одна цена, где бы ни стояли его строки.
function productKey(row: Row, index: number) {
  const code = access.getCode(row).trim()
  if (!code) return `row:${index}`
  if (row.item.productDirectoryId) return `${code}|product:${row.item.productDirectoryId}`
  const name = productName(row).toLowerCase().replace(/ё/g, 'е')
  return name ? `${code}|${name}` : `row:${index}`
}

type AiProduct = { id: string; key: string; code: string; name: string; indexes: number[] }

// Товары для ИИ: коды без цены и коды с ценой, где у другого товара цены нет (ему нужна
// пропорция к товару с ценой). Строки одного товара склеиваются.
function aiProducts(rows: Row[], groups: BudgetGroup[], unit: string) {
  const products: AiProduct[] = []
  const request: PriceGroupInput[] = []
  groups.forEach((group) => {
    const pricedKeys = new Set(group.pricedIndexes.map((index) => productKey(rows[index], index)))
    const needsAi = group.fixedPrice === null || group.indexes.some((index) => !pricedKeys.has(productKey(rows[index], index)))
    if (!needsAi) return
    const byKey = new Map<string, AiProduct>()
    group.indexes.forEach((index) => {
      const key = productKey(rows[index], index)
      const product = byKey.get(key) ?? { id: `p${products.length + byKey.size + 1}`, key, code: group.code || group.key, name: productName(rows[index]), indexes: [] }
      product.indexes.push(index)
      byKey.set(key, product)
    })
    const groupProducts = [...byKey.values()]
    products.push(...groupProducts)
    request.push({
      code: group.code || group.key,
      unit,
      products: groupProducts.map((product) => ({
        id: product.id,
        name: product.name,
        quantity: sumOver(rows, product.indexes, (row) => access.getQuantity(row)),
        places: sumOver(rows, product.indexes, (row) => numberOf(row.result.places)) || undefined,
        grossKg: sumOver(rows, product.indexes, (row) => numberOf(row.result.grossKg)) || undefined,
        netKg: sumOver(rows, product.indexes, (row) => numberOf(row.result.netKg)) || undefined,
      })),
    })
  })
  return { products, request }
}

// Распределение бюджета на этапе результата. apply получает строки с новыми ценами:
// калькулятор запоминает прежний результат для отката и пересчитывает без сохранения черновика.
export function useBudgetDistribution({ estimateId, unit, result, items, preview, apply }: {
  estimateId: number
  unit: string
  result: GlovesCalculationResult | null
  items: GlovesItemInput[]
  // Пробный расчёт строк без показа и сохранения — по нему подбираются цены.
  preview: (nextItems: GlovesItemInput[]) => Promise<GlovesCalculationResult>
  apply: (nextItems: GlovesItemInput[]) => void
}) {
  const [budget, setBudget] = useState('')
  const [gapMin, setGapMinState] = useState(() => storedGap().min)
  const [gapMax, setGapMaxState] = useState(() => storedGap().max)
  const [gapMode, setGapModeState] = useState<GapMode>(() => storedGap().mode)
  const saveGap = (min: string, max: string, mode: GapMode) => {
    try { localStorage.setItem(gapStorageKey, JSON.stringify({ min, max, mode })) } catch { /* не запомнится — не страшно */ }
  }
  const setGapMin = (value: string) => { setGapMinState(value); saveGap(value, gapMax, gapMode) }
  const setGapMax = (value: string) => { setGapMaxState(value); saveGap(gapMin, value, gapMode) }
  const setGapMode = (value: GapMode) => { setGapModeState(value); saveGap(gapMin, gapMax, value) }
  const resetGap = () => { setGapMinState(String(BUDGET_MIN_GAP)); setGapMaxState(String(BUDGET_MAX_GAP)); setGapModeState('under'); saveGap(String(BUDGET_MIN_GAP), String(BUDGET_MAX_GAP), 'under') }
  const gap = parseGap(gapMin, gapMax)
  const [running, setRunning] = useState<BudgetMode | null>(null)
  const [summary, setSummary] = useState<BudgetSummary | null>(null)
  const [warning, setWarning] = useState('')
  const [notice, setNotice] = useState('')
  const [notes, setNotes] = useState<Record<number, string>>({})
  const [explanations, setExplanations] = useState<AiExplanation[]>([])
  const [evaluations, setEvaluations] = useState(0)
  // Ответы ИИ по набору товаров: бюджет в ИИ не отправляется, поэтому при другом бюджете
  // и тех же товарах ответ берётся из памяти мгновенно.
  const aiAnswers = useRef(new Map<string, SuggestedPrice[]>())

  function reset() {
    setSummary(null); setWarning(''); setNotice(''); setNotes({}); setExplanations([])
  }

  async function run(mode: BudgetMode) {
    if (!result || running || !gap) return
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
      const groups = budgetGroups(rows, access)
      const free = groups.filter((group) => group.fixedPrice === null)
      const facts = new Map<string, GroupFacts>(free.map((group) => {
        const quantity = group.quantity
        const net = sumOver(rows, group.indexes, (row) => numberOf(row.result.netKg))
        return [group.key, { key: group.key, netPerUnit: net > 0 && quantity > 0 ? net / quantity : null }]
      }))

      // История цен из прошлых смет (для групп с кодом) и ответ ИИ запрашиваются одновременно.
      const historyCodes = free.filter((group) => group.code).map((group) => group.code)
      const { products, request } = mode === 'ai' ? aiProducts(rows, groups, unit) : { products: [], request: [] }
      const signature = JSON.stringify(request)
      const cached = aiAnswers.current.get(signature)
      const aiAnswer = request.length === 0 ? null
        : (cached ? Promise.resolve(cached) : pricingApi.suggestPrices(request)).then((prices) => ({ prices }), (error: unknown) => ({ error }))
      if (historyCodes.length > 0) {
        try {
          const history = new Map((await pricingApi.priceHistory(estimateId, historyCodes)).map((price) => [price.code, price.unitPrice]))
          free.forEach((group) => {
            const known = history.get(group.code)
            const fact = facts.get(group.key)
            if (fact && !fact.price && known) fact.price = known
          })
        } catch { /* без истории — вес нетто или поровну */ }
      }

      // Режим ИИ: цена каждого товара. Цена кода для подбора — средняя цена его товаров
      // (по количеству), внутри кода цена делится между товарами в пропорции ИИ.
      const productPrices: Record<string, number> = {}
      const answered: { product: AiProduct; reason: string }[] = []
      if (aiAnswer) {
        try {
          const answer = await aiAnswer
          if ('error' in answer) throw answer.error
          const prices = answer.prices
          aiAnswers.current.set(signature, prices)
          const suggested = new Map(prices.map((price) => [price.id, price]))
          products.forEach((product) => {
            const price = suggested.get(product.id)
            if (!price) return
            productPrices[product.key] = price.unitPrice
            answered.push({ product, reason: price.reason })
          })
          free.forEach((group) => {
            const fact = facts.get(group.key)
            let quantity = 0
            let value = 0
            group.indexes.forEach((index) => {
              const price = productPrices[productKey(rows[index], index)]
              const rowQuantity = access.getQuantity(rows[index]) ?? 0
              if (!price) return
              quantity += rowQuantity
              value += price * rowQuantity
            })
            if (fact && quantity > 0) fact.price = value / quantity
          })
        } catch (reason) {
          setNotice(`ИИ недоступен (${errorMessage(reason)}), использовано математическое распределение`)
        }
      }
      const byProduct = answered.length > 0

      let problem = ''
      const outcome = await distributeBudget(rows, amount, access, {
        weights: resolveWeights([...facts.values()]),
        productOf: byProduct ? productKey : undefined,
        productPrices: byProduct ? productPrices : undefined,
        evaluate: async (candidate) => {
          const calculated = await preview(candidate.map((row) => row.item))
          const total = customsPaymentUsd(calculated)
          if (total === null) problem = calculationProblem(calculated)
          return total
        },
        minGap: gap.min,
        maxGap: gap.max,
        over: gapMode === 'over',
        onProgress: setEvaluations,
      })
      if (outcome.status === 'nothing') {
        setWarning(outcome.notCalculated && problem
          ? `Итог «Растаможка к оплате» не посчитан — исправьте строки и повторите: ${problem}`
          : outcome.message ?? 'Распределять нечего')
        return
      }
      // Пояснения — только к строкам, цену которых поставил подбор (строки со своей ценой не менялись).
      const priced = new Set(groups.flatMap((group) => group.pricedIndexes))
      const nextNotes: Record<number, string> = {}
      const nextExplanations: AiExplanation[] = []
      answered.forEach(({ product, reason }) => {
        const changed = product.indexes.filter((index) => !priced.has(index))
        if (changed.length === 0 || !reason) return
        changed.forEach((index) => { nextNotes[index] = reason })
        nextExplanations.push({
          code: product.code.startsWith('row:') ? '—' : product.code,
          names: product.name,
          price: outcome.items[changed[0]]?.item.pricePerPair ?? '',
          reason,
          rows: changed.map((index) => index + 1),
        })
      })
      setNotes(nextNotes)
      setExplanations(nextExplanations)
      setSummary({ status: outcome.status, budget: amount, total: outcome.total, gap: outcome.gap, fixedRows: outcome.fixedRows, evaluations: outcome.evaluations })
      if (outcome.status === 'closest') setWarning(outcome.message ?? '')
      apply(outcome.items.map((row) => row.item))
    } catch (reason) {
      setWarning(`Не удалось подобрать цены: ${errorMessage(reason)}`)
    } finally {
      setRunning(null)
    }
  }

  return { budget, setBudget, gapMin, gapMax, gapMode, setGapMin, setGapMax, setGapMode, resetGap, gap, running, evaluations, summary, warning, notice, notes, explanations, run, reset }
}

export type BudgetDistributionState = ReturnType<typeof useBudgetDistribution>
