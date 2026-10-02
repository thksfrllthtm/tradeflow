// Округление по правилам заполнения таможенной декларации — то же, что на сервере
// (backend/internal/service/rounding.go). Везде по математическим правилам: 5 и больше — вверх.
//   деньги (гр.12, 22, 42, 45, 47, B) — до двух знаков;
//   вес брутто и нетто (гр.35, 38) — до трёх знаков, меньше одного грамма — до шести.
// toFixed для этого не годится: из-за двоичной записи 1.005.toFixed(2) даёт «1.00».

// roundHalfUp — округление до decimals знаков, половина — от нуля.
export function roundHalfUp(value: number, decimals: number): number {
  if (!Number.isFinite(value)) return value
  const factor = 10 ** decimals
  // Поправка на погрешность двоичной записи: 1.005 × 100 = 100.49999999999999.
  const rounded = Math.round(Math.abs(value) * factor * (1 + Number.EPSILON)) / factor
  return value < 0 ? -rounded : rounded
}

// moneyText — денежная сумма с двумя знаками: «1234.50».
export function moneyText(value: number): string {
  return roundHalfUp(value, 2).toFixed(2)
}

const oneGramKg = 0.001

function weightDecimals(value: number) {
  const six = roundHalfUp(value, 6)
  return six !== 0 && Math.abs(six) < oneGramKg ? 6 : 3
}

// roundWeightKg — вес в килограммах: до трёх знаков, меньше одного грамма — до шести.
export function roundWeightKg(value: number): number {
  return roundHalfUp(value, weightDecimals(value))
}

// weightText — вес строкой: «3073.800» (меньше грамма — «0.000457»); trim — без лишних нулей.
export function weightText(value: number, trim = false): string {
  const text = roundWeightKg(value).toFixed(weightDecimals(value))
  return trim ? text.replace(/\.?0+$/, '') || '0' : text
}

// decimalText — число до decimals знаков без лишних нулей (цена единицы, количество).
export function decimalText(value: number, decimals: number): string {
  return roundHalfUp(value, decimals).toFixed(decimals).replace(/\.?0+$/, '') || '0'
}
