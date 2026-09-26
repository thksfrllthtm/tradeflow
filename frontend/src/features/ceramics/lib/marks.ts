// Ключ марки для точного поиска — так же, как на сервере (NormalizeCeramicMark):
// полуширинные символы, верхний регистр, без пробелов, любое тире — «-».
export function normalizeMark(mark: string) {
  return Array.from(String(mark ?? ''))
    .map((char) => {
      const code = char.charCodeAt(0)
      if (code === 0x3000) return ' '
      if (code >= 0xff01 && code <= 0xff5e) return String.fromCharCode(code - 0xfee0)
      return char
    })
    .join('')
    .replace(/\s+/g, '')
    .replace(/[‐‑‒–—−]/g, '-')
    .toUpperCase()
}
