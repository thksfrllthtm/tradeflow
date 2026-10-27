// Ответ на уточнение считается повтором прошлого, если тексты почти совпадают по словам (Жаккар ≥ 0.8):
// тогда вместо пересказа показывается «Ответ тот же».
export function sameAnswer(current: string, previous: string): boolean {
  const words = (text: string) => new Set(text.toLowerCase().replace(/[^\p{L}\p{N}\s]/gu, ' ').split(/\s+/).filter((word) => word.length > 2))
  const left = words(current)
  const right = words(previous)
  if (left.size === 0 || right.size === 0) return false
  let common = 0
  for (const word of left) if (right.has(word)) common++
  return common / (left.size + right.size - common) >= 0.8
}
