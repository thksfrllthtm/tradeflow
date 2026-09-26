import { it } from 'vitest'
import { distributeBudget, BudgetRowAccess } from './distributeBudget'
type R = { code: string; qty: number | null; price: string; product: string }
const access: BudgetRowAccess<R> = {
  getCode: (r) => r.code, getQuantity: (r) => r.qty, getPrice: (r) => { const n = Number(r.price); return r.price !== '' && Number.isFinite(n) ? n : null },
  setPrice: (r, price) => ({ ...r, price }),
}
it('fuzz', async () => {
  let seed = 1
  const rnd = () => { seed = (seed * 16807) % 2147483647; return seed / 2147483647 }
  for (let t = 0; t < 20000; t++) {
    const n = 2 + Math.floor(rnd() * 8)
    const rows: R[] = Array.from({ length: n }, (_, i) => ({ code: ['A', 'B', 'C'][Math.floor(rnd() * 3)], qty: rnd() < 0.1 ? 0 : Math.floor(rnd() * 1000) + 1, price: rnd() < 0.3 ? (rnd() * 5).toFixed(3) : '', product: 'p' + Math.floor(rnd() * 4) + i % 2 }))
    const prices: Record<string, number> = {}
    rows.forEach((r) => { if (rnd() < 0.7) prices[r.code + r.product] = rnd() < 0.1 ? rnd() * 1e-6 : rnd() * 10 })
    let bad = ''
    await distributeBudget(rows, 100 + rnd() * 10000, access, {
      productOf: (r) => r.code + r.product, productPrices: prices, weights: { 'code:A': rnd() * 3, 'code:B': rnd() },
      evaluate: async (cand) => {
        for (const r of cand) if ((r.qty ?? 0) > 0 && (r.price === "" || !Number.isFinite(Number(r.price)))) bad = JSON.stringify({ rows, prices, cand })
        return cand.reduce((s, r) => s + (Number(r.price) || 0) * (r.qty ?? 0) * 0.3, 0)
      },
    })
    if (bad) { console.log(bad); throw new Error('bad price') }
  }
})
