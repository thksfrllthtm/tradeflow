export function formatDisplayNumber(value: string | number | undefined | null): string {
  if (value === undefined || value === null) return ''

  const text = String(value).trim()
  if (!text) return ''

  const normalized = text.replace(/\s/g, '').replace(',', '.')
  const match = normalized.match(/^([+-]?)(\d+)(\.\d+)?$/)
  if (!match) return text

  const sign = match[1]
  const integer = match[2].replace(/\B(?=(\d{3})+(?!\d))/g, ' ')
  return `${sign}${integer}${match[3] ?? ''}`
}

// То же для показа по-русски: дробная часть через запятую («12 345,67»). Только отображение —
// значения, поля ввода и расчёт по-прежнему работают с точкой.
export function formatRuNumber(value: string | number | undefined | null): string {
  return formatDisplayNumber(value).replace(/^([+-]?[\d ]+)\.(\d+)$/, '$1,$2')
}

// Сумма с валютой через неразрывный пробел: «128 432,04 KZT», «9 456,49 USD». Пусто — «—».
export function formatMoney(value: string | number | undefined | null, currency: string): string {
  const shown = formatRuNumber(value)
  return shown && shown !== '—' ? `${shown} ${currency}` : '—'
}
