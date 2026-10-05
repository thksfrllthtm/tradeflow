// Ссылки в тексте ответа агента: обычный текст и адреса http(s), чтобы показать их кликабельными.
// Знаки препинания в конце («…см. https://adilet.zan.kz/rus/docs/X.») — не часть адреса.
export type TextPart = { text: string; url?: string }

const urlPattern = /https?:\/\/[^\s<>«»"']+/g

export function splitLinks(text: string): TextPart[] {
  const parts: TextPart[] = []
  let last = 0
  for (const match of text.matchAll(urlPattern)) {
    const url = match[0].replace(/[.,;:!?)\]]+$/, '')
    const start = match.index ?? 0
    if (start > last) parts.push({ text: text.slice(last, start) })
    parts.push({ text: url, url })
    last = start + url.length
  }
  if (last < text.length) parts.push({ text: text.slice(last) })
  return parts
}
