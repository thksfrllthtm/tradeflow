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
