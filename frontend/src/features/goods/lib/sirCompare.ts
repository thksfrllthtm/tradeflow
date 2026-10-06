import { GoodsCalculationResult, SirWeight } from '@/shared/types/goods'

// Сравнение двух расчётов одной сметы: по инвойсу и по СИР. Строки сопоставляются по номеру —
// оба расчёта сделаны из одних и тех же строк сметы.

const number = (value: unknown) => {
  const parsed = Number(String(value ?? '').replace(/\s/g, '').replace(',', '.'))
  return String(value ?? '').trim() !== '' && Number.isFinite(parsed) ? parsed : null
}
const sum = (...values: (number | null)[]) => values.every((value) => value === null) ? null : values.reduce<number>((total, value) => total + (value ?? 0), 0)
const diff = (a: number | null, b: number | null) => a === null || b === null ? null : b - a

export type SirCompareRow = {
  index: number
  weightKg: number | null
  // Цена 1 кг по инвойсу, USD: стоимость в USD / вес; null — стоимость не в USD или нет веса.
  invoicePerKg: number | null
  // Индикатор из файла и со снижением; null — для кода индикатора нет (строка по инвойсу).
  sirPerKg: number | null
  sirDiscountedPerKg: number | null
  profile: string
  // Цена инвойса ниже индикатора (без снижения) — таможня может откорректировать стоимость.
  belowSir: boolean
  customsInvoice: number | null
  customsSir: number | null
  customsDiff: number | null
}

export type SirCompareLine = { label: string; invoice: number | null; sir: number | null; diff: number | null; total?: boolean }

export function compareRows(invoice: GoodsCalculationResult, sir: GoodsCalculationResult, weight: SirWeight): SirCompareRow[] {
  return invoice.items.flatMap((item, index) => {
    const other = sir.items[index]
    if (item.excluded || !other) return []
    const weightKg = number(other.sir?.weightKg) ?? number(weight === 'gross' ? item.grossKg : item.netKg)
    const costUsd = number(item.costUsd)
    const invoicePerKg = costUsd !== null && weightKg ? costUsd / weightKg : null
    const sirPerKg = number(other.sir?.priceUsd)
    const customsInvoice = number(item.customsValueKzt)
    const customsSir = number(other.customsValueKzt)
    return [{
      index, weightKg, invoicePerKg, sirPerKg, sirDiscountedPerKg: number(other.sir?.pricePerKgUsd), profile: other.sir?.profile ?? '',
      belowSir: invoicePerKg !== null && sirPerKg !== null && invoicePerKg < sirPerKg,
      customsInvoice, customsSir, customsDiff: diff(customsInvoice, customsSir),
    }]
  })
}

// Итоги обоих расчётов построчно: ТС, платежи, растаможка к оплате в тенге и в USD.
export function compareTotals(invoice: GoodsCalculationResult, sir: GoodsCalculationResult): SirCompareLine[] {
  const line = (label: string, pick: (result: GoodsCalculationResult) => number | null, total = false): SirCompareLine => {
    const a = pick(invoice)
    const b = pick(sir)
    return { label, invoice: a, sir: b, diff: diff(a, b), total }
  }
  const usd = (result: GoodsCalculationResult) => {
    const payments = number(result.totals.customsPaymentsKzt)
    const rate = number(result.usdKzt)
    return payments !== null && rate ? payments / rate : null
  }
  const lines = [
    line('Таможенная стоимость', (result) => number(result.totals.customsValueKzt)),
    line('Пошлина', (result) => number(result.totals.dutyKzt)),
    line('Антидемпинг', (result) => number(result.totals.antiDumpingKzt)),
    line('НДС', (result) => number(result.totals.vatKzt)),
    line('Сбор', (result) => number(result.totals.feeKzt)),
    line('Растаможка к оплате', (result) => number(result.totals.customsPaymentsKzt), true),
  ].filter((item) => item.label !== 'Антидемпинг' || (item.invoice ?? 0) > 0 || (item.sir ?? 0) > 0)
  return [...lines, { ...line('Растаможка к оплате, USD', usd, true) }]
}

// Сколько строк сравнения посчитано по СИР и сколько из них дешевле индикатора по инвойсу.
export function compareCounts(rows: SirCompareRow[]) {
  return { sir: rows.filter((row) => row.sirPerKg !== null).length, below: rows.filter((row) => row.belowSir).length, total: rows.length }
}

export const totalOf = (values: (number | null)[]) => sum(...values)
