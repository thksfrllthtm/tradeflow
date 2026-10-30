import type { APIRequestContext } from '@playwright/test'
import { E2E, uid } from './users'

// Точная десятичная арифметика для ожидаемых сумм (BigInt, 20 знаков после запятой).
// Округление — «половина от нуля», как decimal.Round в shopspring (RoundMoney).
const SCALE = 20n
const ONE = 10n ** SCALE

export class Dec {
  private constructor(readonly v: bigint) {}
  static of(value: string | number | Dec): Dec {
    if (value instanceof Dec) return value
    const text = String(value).trim()
    const negative = text.startsWith('-')
    const [int, frac = ''] = text.replace(/^[-+]/, '').split('.')
    const v = BigInt(int || '0') * ONE + BigInt((frac + '0'.repeat(Number(SCALE))).slice(0, Number(SCALE)) || '0')
    return new Dec(negative ? -v : v)
  }
  add(o: string | number | Dec) { return new Dec(this.v + Dec.of(o).v) }
  sub(o: string | number | Dec) { return new Dec(this.v - Dec.of(o).v) }
  mul(o: string | number | Dec) { return new Dec((this.v * Dec.of(o).v) / ONE) }
  div(o: string | number | Dec) { return new Dec((this.v * ONE) / Dec.of(o).v) }
  round(places: number) {
    const unit = 10n ** (SCALE - BigInt(places))
    const abs = this.v < 0n ? -this.v : this.v
    const rounded = ((abs + unit / 2n) / unit) * unit
    return new Dec(this.v < 0n ? -rounded : rounded)
  }
  floor(places: number) {
    const unit = 10n ** (SCALE - BigInt(places))
    return new Dec(this.v >= 0n ? (this.v / unit) * unit : -(((-this.v + unit - 1n) / unit) * unit))
  }
  max(o: Dec) { return this.v >= o.v ? this : o }
  gt(o: string | number | Dec) { return this.v > Dec.of(o).v }
  isZero() { return this.v === 0n }
  // Строка с фиксированным числом знаков («1234.50»), как StringFixed.
  fixed(places: number) {
    const r = this.round(places).v
    const negative = r < 0n
    const abs = negative ? -r : r
    const int = abs / ONE
    const frac = (abs % ONE).toString().padStart(Number(SCALE), '0').slice(0, places)
    return `${negative ? '-' : ''}${int}${places > 0 ? '.' + frac : ''}`
  }
}
export const D = Dec.of

export type Duty = { type: string; percent: string; amount: string; currency: string; unit: string }
export type NomenclatureItem = { code: string; duty: Duty; vatPercent: string; antiDumping?: unknown[]; sir?: { priceUsd: string; profile: string } | null }

export async function lookupCodes(api: APIRequestContext, codes: string[]): Promise<Map<string, NomenclatureItem>> {
  const response = await api.post('/api/tnved-nomenclature/lookup', { data: { codes } })
  if (!response.ok()) throw new Error(`lookup ${response.status()}: ${await response.text()}`)
  const items = (await response.json()).items as NomenclatureItem[]
  return new Map(items.map((item) => [item.code, item]))
}

// Пошлина строки по правилам расчёта (duty.go): процент от ТС; специфическая — ставка × единицы ×
// курс валюты ставки (кг и т — по нетто, пары и шт — по количеству, «1000 шт» — количество / 1000);
// «не менее» — большее из двух, «плюс» — сумма.
export function expectedDuty(duty: Duty, customs: Dec, net: Dec, qty: Dec, usd: Dec, eur: Dec): Dec {
  const byValue = () => customs.mul(duty.percent).div(100)
  const byUnits = () => {
    let unit = duty.unit.trim()
    let per = D(1)
    const match = /^(\d+)\s+(.+)$/.exec(unit)
    if (match) { per = D(match[1]); unit = match[2] }
    const units = unit === 'кг' ? net : unit === 'т' ? net.div(1000) : ['пар', 'пара', 'пару', 'шт'].includes(unit) ? qty : null
    if (!units) throw new Error(`единица ${duty.unit} расчётом не поддерживается`)
    const rate = duty.currency === 'EUR' ? eur : duty.currency === 'USD' ? usd : D(1)
    return D(duty.amount).mul(units.div(per)).mul(rate)
  }
  switch (duty.type) {
    case 'percentage': return byValue()
    case 'specific': return byUnits()
    case 'combined_minimum': return byValue().max(byUnits())
    case 'combined_plus': return byValue().add(byUnits())
  }
  throw new Error(`схема ${duty.type}`)
}

// Строка «универсальной» сметы: код, количество, брутто/нетто в кг, цена за единицу в USD.
export function row(code: string, qty: string, price: string, grossKg: string, netKg: string, extra: Record<string, unknown> = {}) {
  return {
    id: uid(), name: `${E2E} товар ${code}`, supplierName: `${E2E} товар ${code}`, code,
    pairs: qty, places: '1', grossKg, grossWeightUnit: 'kg', netKg, netWeightUnit: 'kg', netSource: 'excel',
    pricePerPair: price, antiDumpingChoice: 'none', ...extra,
  }
}

export function calcInput(items: unknown[], extra: Record<string, unknown> = {}) {
  return { assessmentDate: '2026-10-20', usdKzt: '471.25', eurKzt: '548.60', deliveryMode: 'none', genericItems: true, items, ...extra }
}

export async function calculate(api: APIRequestContext, input: unknown) {
  const response = await api.post('/api/goods/calculate', { data: input })
  return { response, body: await response.json().catch(() => ({})) }
}

export async function createEstimate(api: APIRequestContext, extra: Record<string, unknown> = {}) {
  const response = await api.post('/api/goods-estimates', { data: { name: `${E2E}-${uid()}`, client: `${E2E} клиент`, assessmentDate: '2026-10-20', ...extra } })
  if (response.status() !== 201) throw new Error(`create ${response.status()}: ${await response.text()}`)
  return (await response.json()).estimate as { id: number; name: string; client: string; assessmentDate: string; status: string }
}

// Минимальный PNG 1×1 (валидный, для фото строк и картинок в Excel).
export const tinyPng = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64')
export const tinyPngDataUrl = `data:image/png;base64,${tinyPng.toString('base64')}`
