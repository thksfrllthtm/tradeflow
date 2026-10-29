import { useEffect, useMemo, useState } from 'react'
import { GoodsItemInput } from '@/shared/types/goods'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { TrademarkHit, trademarksApi, trademarkText } from '@/shared/trademarks/trademarks'
import type { ReviewProblem } from './reviewProblems'

// Проверка строк сметы по ТРОИС: наименования строки и код уходят на сервер после паузы в правках;
// найденные знаки — проблема «ТРОИС» у наименования (чип-фильтр и подсказка ячейки).

type CheckRow = { id: string; texts: string[]; code: string }

const rowsOf = (items: GoodsItemInput[]): CheckRow[] => items
  .filter((item) => !item.excluded)
  .map((item) => ({ id: item.id ?? '', texts: [item.name, item.supplierName, ...(item.otherNames ?? [])].map((text) => text.trim()).filter(Boolean), code: item.code }))
  .filter((row) => row.id && row.texts.length > 0)

// Знаки ТРОИС по id строки (строки без знаков не попадают).
export function useTrademarkHits(items: GoodsItemInput[]): Record<string, TrademarkHit[]> {
  const key = useDebouncedValue(useMemo(() => JSON.stringify(rowsOf(items)), [items]), 800)
  const [hits, setHits] = useState<Record<string, TrademarkHit[]>>({})
  useEffect(() => {
    const rows = JSON.parse(key) as CheckRow[]
    if (rows.length === 0) { setHits({}); return }
    let active = true
    // Реестр не загружен или сервер недоступен — проверка просто не показывается.
    trademarksApi.check(rows.map(({ texts, code }) => ({ texts, code }))).then((results) => {
      if (!active) return
      const next: Record<string, TrademarkHit[]> = {}
      rows.forEach((row, index) => { if (results[index]?.length) next[row.id] = results[index] })
      setHits(next)
    }).catch(() => undefined)
    return () => { active = false }
  }, [key])
  return hits
}

export function trademarkProblems(hits: TrademarkHit[] | undefined): ReviewProblem[] {
  return (hits ?? []).map((hit) => ({ type: 'trademark', label: 'ТРОИС', tone: 'warn', column: 'name', text: trademarkText(hit) }))
}
