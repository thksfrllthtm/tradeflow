// Веса групп для распределения бюджета. У группы может быть известная цена за единицу
// (прошлые сметы или оценка ИИ) и/или вес нетто одной единицы.
//
// Порядок: цена → вес нетто единицы → 1. Цена (USD) и вес (кг) — разные величины, их нельзя
// смешивать в одной пропорции. Поэтому если цены есть хотя бы у части групп, остальные
// переводятся в ту же шкалу: вес нетто × средняя (медианная) цена за кг по группам с ценой,
// а без веса — медианная цена. Если цен нет ни у кого — пропорционально весу нетто единицы.
export type GroupFacts = { key: string; price?: number | null; netPerUnit?: number | null }

const positive = (value: number | null | undefined): value is number => value !== null && value !== undefined && Number.isFinite(value) && value > 0

function median(values: number[]): number | null {
  if (values.length === 0) return null
  const sorted = [...values].sort((a, b) => a - b)
  const middle = Math.floor(sorted.length / 2)
  return sorted.length % 2 ? sorted[middle] : (sorted[middle - 1] + sorted[middle]) / 2
}

export function resolveWeights(groups: GroupFacts[]): Record<string, number> {
  const weights: Record<string, number> = {}
  const priced = groups.filter((group) => positive(group.price))
  if (priced.length > 0) {
    const medianPrice = median(priced.map((group) => group.price as number)) as number
    const pricePerKg = median(priced.filter((group) => positive(group.netPerUnit)).map((group) => (group.price as number) / (group.netPerUnit as number)))
    groups.forEach((group) => {
      weights[group.key] = positive(group.price) ? group.price
        : positive(group.netPerUnit) && pricePerKg !== null ? group.netPerUnit * pricePerKg
        : medianPrice
    })
    return weights
  }
  const medianNet = median(groups.filter((group) => positive(group.netPerUnit)).map((group) => group.netPerUnit as number))
  groups.forEach((group) => {
    weights[group.key] = positive(group.netPerUnit) ? group.netPerUnit : medianNet ?? 1
  })
  return weights
}
