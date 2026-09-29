// Слово после числа: plural(3, 'код', 'кода', 'кодов') → «кода», plural(13293, …) → «кода».
export function plural(count: number, one: string, few: string, many: string) {
  const mod10 = count % 10, mod100 = count % 100
  if (mod10 === 1 && mod100 !== 11) return one
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return few
  return many
}
