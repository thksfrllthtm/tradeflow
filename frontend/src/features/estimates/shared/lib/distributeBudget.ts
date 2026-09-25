// Распределение бюджета по ценам единицы товаров: цены подбираются так, чтобы итог
// «Растаможка к оплате» (пошлина + антидемпинг + НДС + сбор) не доходил до бюджета на
// minGap…maxGap USD (20–30). Итог считает сервер (ставки бывают за кг, пару, комбинированные),
// поэтому функция получает evaluate — пробный расчёт строк с ценами, — и подбирает общий
// масштаб цен, не зная формул. Строки любые (перчатки, товары): доступ через аксессоры.
//
// Правила:
// 1. Строка с ценой > 0 (из Excel, по ИТП, вручную) — фиксированная, её цена не меняется. Если в группе
//    (один код ТН ВЭД) цена есть хотя бы у одной строки, строки группы без цены получают эту цену
//    (первую найденную), и группа тоже фиксированная.
// 2. Свободные строки группируются по коду: одинаковый код → одинаковая цена.
// 3. Цена группы = round(вес × масштаб). Масштаб ищется методом секущих с удержанием интервала
//    (итог растёт вместе с ценами) до попадания в окно minGap…maxGap под бюджетом.
// 4. Остаток от округления добирается ценой группы с наименьшим количеством (шаг 10^−decimals),
//    перерасход штрафуется сильно. Не попали в окно — 'closest' и предупреждение.
// 5. Товары внутри кода (productOf + productPrices, режим ИИ): цена кода делится между товарами
//    в пропорции их цен, сумма кода (цена × количество) не меняется. Одинаковый товар — одна цена.
//    Без этих опций у всех строк кода одна цена.

const BUDGET_MIN_GAP = 20
const BUDGET_MAX_GAP = 30
const BUDGET_PRICE_DECIMALS = 4
const BUDGET_MAX_EVALUATIONS = 14

export type BudgetRowAccess<T> = {
  getCode: (row: T) => string
  getQuantity: (row: T) => number | null
  getPrice: (row: T) => number | null
  setPrice: (row: T, price: string) => T
  // Исключённые из расчёта строки не участвуют (по умолчанию участвуют все).
  isIncluded?: (row: T) => boolean
}

// pricedIndexes — строки со своей ценой: их цена никогда не меняется.
export type BudgetGroup = { key: string; code: string; indexes: number[]; pricedIndexes: number[]; quantity: number; fixedPrice: number | null }

// Итог к оплате в USD для строк с заданными ценами; null — итог не посчитан (ошибки в строках).
export type BudgetEvaluator<T> = (rows: T[]) => Promise<number | null>

export type BudgetOptions<T> = {
  evaluate: BudgetEvaluator<T>
  // Вес группы по ключу группы (budgetGroupKey); если веса нет — 1.
  weights?: Record<string, number>
  // Товар строки внутри кода и его цена (важна только пропорция внутри кода). Товар без цены
  // получает среднюю цену кода.
  productOf?: (row: T, index: number) => string
  productPrices?: Record<string, number>
  minGap?: number
  maxGap?: number
  decimals?: number
  maxEvaluations?: number
  onProgress?: (evaluations: number) => void
}

export type BudgetDistribution<T> = {
  items: T[]
  status: 'ok' | 'closest' | 'nothing'
  // Итог к оплате при выбранных ценах и недостача до бюджета (отрицательная — перерасход).
  total: number
  gap: number
  fixedRows: number
  evaluations: number
  message?: string
}

// Строки без кода не объединяются: у каждой своя группа.
export function budgetGroupKey(code: string, index: number) {
  const trimmed = code.trim()
  return trimmed ? `code:${trimmed}` : `row:${index}`
}

// Группы строк с количеством > 0: общий код, суммарное количество и цена группы, если она задана.
export function budgetGroups<T>(rows: T[], access: BudgetRowAccess<T>): BudgetGroup[] {
  const groups = new Map<string, BudgetGroup>()
  rows.forEach((row, index) => {
    if (access.isIncluded && !access.isIncluded(row)) return
    const quantity = access.getQuantity(row)
    if (quantity === null || !(quantity > 0)) return
    const code = access.getCode(row).trim()
    const key = budgetGroupKey(code, index)
    const group = groups.get(key) ?? { key, code, indexes: [], pricedIndexes: [], quantity: 0, fixedPrice: null }
    group.indexes.push(index)
    group.quantity += quantity
    const price = access.getPrice(row)
    if (price !== null && price > 0) {
      group.pricedIndexes.push(index)
      if (group.fixedPrice === null) group.fixedPrice = price
    }
    groups.set(key, group)
  })
  return [...groups.values()]
}

// Доля цены строки относительно средней цены её кода (взвешенной по количеству): Σ доля × количество
// по коду равна количеству кода, поэтому сумма кода при делении не меняется. Товар без цены — 1.
function productShares<T>(rows: T[], groups: BudgetGroup[], access: BudgetRowAccess<T>, productOf?: (row: T, index: number) => string, prices?: Record<string, number>) {
  const shares = new Map<number, number>()
  if (!productOf || !prices) return shares
  const priceOf = (index: number) => {
    const price = prices[productOf(rows[index], index)]
    return price !== undefined && Number.isFinite(price) && price > 0 ? price : null
  }
  for (const group of groups) {
    let quantity = 0
    let value = 0
    group.indexes.forEach((index) => {
      const price = priceOf(index)
      if (price === null) return
      const rowQuantity = access.getQuantity(rows[index]) ?? 0
      quantity += rowQuantity
      value += price * rowQuantity
    })
    if (!(quantity > 0 && value > 0)) continue
    const average = value / quantity
    group.indexes.forEach((index) => shares.set(index, (priceOf(index) ?? average) / average))
  }
  return shares
}

const money = (value: number) => Math.round(value * 100) / 100
const format = (value: number) => value.toFixed(2)

export async function distributeBudget<T>(rows: T[], budget: number, access: BudgetRowAccess<T>, options: BudgetOptions<T>): Promise<BudgetDistribution<T>> {
  const minGap = options.minGap ?? BUDGET_MIN_GAP
  const maxGap = options.maxGap ?? BUDGET_MAX_GAP
  const decimals = options.decimals ?? BUDGET_PRICE_DECIMALS
  const maxEvaluations = options.maxEvaluations ?? BUDGET_MAX_EVALUATIONS
  const scale = 10 ** decimals
  const groups = budgetGroups(rows, access)
  const fixed = groups.filter((group) => group.fixedPrice !== null)
  const free = groups.filter((group) => group.fixedPrice === null)
  const fixedRows = fixed.reduce((count, group) => count + group.indexes.length, 0)
  let evaluations = 0
  const nothing = (message: string, total = 0): BudgetDistribution<T> => ({ items: rows, status: 'nothing', total, gap: money(budget - total), fixedRows, evaluations, message })

  if (!(budget > minGap)) return nothing('Укажите бюджет больше нуля')
  if (groups.length === 0) return nothing('Нет строк с количеством больше нуля — распределять нечего')
  if (free.length === 0) return nothing('У всех строк цена уже задана. Чтобы распределить заново, нажмите «Сбросить цены»')

  const target = budget - (minGap + maxGap) / 2
  const weightOf = (group: BudgetGroup) => {
    const weight = options.weights?.[group.key]
    return weight !== undefined && Number.isFinite(weight) && weight > 0 ? weight : 1
  }
  const priceText = (value: number) => value.toFixed(decimals).replace(/\.?0+$/, '') || '0'
  const relative = productShares(rows, groups, access, options.productOf, options.productPrices)
  const productOf = options.productOf
  // Строка без своей цены в группе с ценой: тот же товар — его цена; другой товар — цена первой
  // строки с ценой в пропорции цен товаров; без товаров — цена группы.
  const fixedPriceFor = (group: BudgetGroup, index: number) => {
    const price = group.fixedPrice ?? 0
    if (!productOf) return price
    const product = productOf(rows[index], index)
    const same = group.pricedIndexes.find((priced) => productOf(rows[priced], priced) === product)
    if (same !== undefined) return access.getPrice(rows[same]) ?? price
    const share = relative.get(index)
    const anchorShare = relative.get(group.pricedIndexes[0])
    return share && anchorShare ? price * share / anchorShare : price
  }
  // Цены свободных групп — в целых «шагах» 10^−decimals.
  const stepsFor = (factor: number) => new Map(free.map((group) => [group.key, Math.max(0, Math.round(weightOf(group) * factor * scale))]))
  const rowsFor = (steps: Map<string, number>) => {
    const priceByIndex = new Map<number, string>()
    // В группе с ценой строки без цены получают цену группы; строки со своей ценой не трогаем.
    fixed.forEach((group) => group.indexes.filter((index) => !group.pricedIndexes.includes(index)).forEach((index) => priceByIndex.set(index, priceText(fixedPriceFor(group, index)))))
    free.forEach((group) => group.indexes.forEach((index) => priceByIndex.set(index, priceText(Math.round((steps.get(group.key) ?? 0) * (relative.get(index) ?? 1)) / scale))))
    return rows.map((row, index) => priceByIndex.has(index) ? access.setPrice(row, priceByIndex.get(index) ?? '') : row)
  }
  const measure = async (steps: Map<string, number>) => {
    evaluations += 1
    options.onProgress?.(evaluations)
    const total = await options.evaluate(rowsFor(steps))
    return total === null || !Number.isFinite(total) ? null : money(total)
  }
  const penalty = (gap: number) => {
    if (gap < minGap) return (minGap - gap) + (gap < 0 ? 1e9 + -gap * 1e3 : 0)
    if (gap > maxGap) return gap - maxGap
    return 0
  }

  type Trial = { factor: number; steps: Map<string, number>; total: number }
  let best: Trial | null = null
  const consider = (trial: Trial) => {
    if (!best || penalty(budget - trial.total) < penalty(budget - best.total)) best = trial
  }
  const tryFactor = async (factor: number): Promise<Trial | null> => {
    const steps = stepsFor(factor)
    const total = await measure(steps)
    if (total === null) return null
    const trial = { factor, steps, total }
    consider(trial)
    return trial
  }
  const failed = () => nothing('Итог «Растаможка к оплате» не посчитан: сначала исправьте ошибки в строках (код ТН ВЭД, количество, вес) и повторите')

  // Начальная догадка: масштаб, при котором стоимость свободных товаров равна цели.
  const weightedQuantity = free.reduce((sum, group) => sum + weightOf(group) * group.quantity, 0)
  let upper: Trial | null = null
  let lower: Trial | null = null
  const first = await tryFactor(target / weightedQuantity)
  if (!first) return failed()
  if (first.total > target) upper = first; else lower = first
  let previous: Trial = first

  // Второй замер — пропорционально (итог ≈ масштаб × коэффициент), дальше секущие.
  let next = first.total > 0 ? first.factor * target / first.total : first.factor * 2
  while (evaluations < maxEvaluations && penalty(budget - (best as Trial | null ?? first).total) > 0) {
    if (lower && upper && !(next > lower.factor && next < upper.factor)) next = (lower.factor + upper.factor) / 2
    if (!(next >= 0) || !Number.isFinite(next)) next = 0
    const trial = await tryFactor(next)
    if (!trial) return failed()
    if (trial.total > target) upper = trial; else lower = trial
    if (trial.factor === 0 && trial.total > budget - minGap) {
      return nothing(`Даже без цен растаможка к оплате ${format(trial.total)} USD — это больше бюджета. Распределять нечего`, trial.total)
    }
    const slope = (trial.total - previous.total) / (trial.factor - previous.factor)
    const secant = Number.isFinite(slope) && slope > 0 ? trial.factor + (target - trial.total) / slope : NaN
    // Итог не растёт с ценами (или не удалось посчитать наклон) — сужаем интервал пополам.
    next = Number.isFinite(secant) ? secant : lower && upper ? (lower.factor + upper.factor) / 2 : trial.factor * 2
    previous = trial
    if (upper && upper.factor === 0) break
  }

  // Добор округления: цена группы с наименьшим количеством — самый мелкий шаг итога.
  let chosen = best as Trial | null ?? first
  if (penalty(budget - chosen.total) > 0 && evaluations + 2 <= maxEvaluations) {
    const adjust = free.reduce((smallest, group) => group.quantity < smallest.quantity ? group : smallest)
    const base = chosen.steps.get(adjust.key) ?? 0
    const middle = budget - target
    const probeSize = Math.max(1, Math.round(base * 0.01))
    const probe = new Map(chosen.steps)
    probe.set(adjust.key, base + probeSize)
    const probeTotal = await measure(probe)
    if (probeTotal !== null) {
      consider({ factor: chosen.factor, steps: probe, total: probeTotal })
      const perStep = (probeTotal - chosen.total) / probeSize
      if (perStep > 0) {
        const gap = budget - chosen.total
        const shifted = new Map(chosen.steps)
        shifted.set(adjust.key, Math.max(0, base + Math.round((gap - middle) / perStep)))
        const shiftedTotal = await measure(shifted)
        if (shiftedTotal !== null) consider({ factor: chosen.factor, steps: shifted, total: shiftedTotal })
      }
    }
    chosen = best as Trial | null ?? chosen
  }

  const gap = money(budget - chosen.total)
  const inWindow = penalty(gap) === 0
  return {
    items: rowsFor(chosen.steps),
    status: inWindow ? 'ok' : 'closest',
    total: chosen.total,
    gap,
    fixedRows,
    evaluations,
    message: inWindow ? undefined : `Не удалось точно попасть в бюджет: ${gap >= 0 ? 'недостача' : 'перерасход'} ${format(Math.abs(gap))} USD. Проверьте цены вручную`,
  }
}
