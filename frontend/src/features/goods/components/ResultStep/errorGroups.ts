import { GoodsCalculationResult } from '@/shared/types/goods'

type ErrorGroup = { text: string; rows: number[] }

// Одинаковые сообщения собираются в одну строку со списком номеров строк (№ в таблице):
// errors — ошибки (строка не рассчитана), warnings — предупреждения. Исключённые строки не входят.
export function errorGroups(result: GoodsCalculationResult, kind: 'errors' | 'warnings' = 'errors'): ErrorGroup[] {
  const byText = new Map<string, number[]>()
  result.items.forEach((item, index) => {
    if (item.excluded) return
    for (const value of item[kind] ?? []) {
      const text = value.trim()
      if (!text) continue
      const rows = byText.get(text) ?? []
      if (!rows.includes(index + 1)) rows.push(index + 1)
      byText.set(text, rows)
    }
  })
  // blockingErrors may also hold global errors; row errors are already listed above.
  if (kind === 'errors') {
    for (const value of result.totals.blockingErrors ?? []) {
      const text = value.trim()
      if (text && !byText.has(text)) byText.set(text, [])
    }
  }
  return Array.from(byText, ([text, rows]) => ({ text, rows })).sort((a, b) => b.rows.length - a.rows.length)
}

// [2,3,4,5,9] → [{ label: "2–5", first: 2 }, { label: "9", first: 9 }]
export function rowRangeParts(rows: number[]) {
  const sorted = [...rows].sort((a, b) => a - b)
  const parts: { label: string; first: number }[] = []
  for (let i = 0; i < sorted.length; i++) {
    const start = sorted[i]
    while (sorted[i + 1] === sorted[i] + 1) i++
    parts.push({ label: start === sorted[i] ? String(start) : `${start}–${sorted[i]}`, first: start })
  }
  return parts
}

// Русское склонение: 1 строка, 2 строки, 5 строк.
export function plural(count: number, forms: [string, string, string]) {
  const mod10 = count % 10
  const mod100 = count % 100
  if (mod10 === 1 && mod100 !== 11) return forms[0]
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return forms[1]
  return forms[2]
}
