// Текст сегмента — XML-фрагмент с backend: основное форматирование абзаца (g0) не размечено,
// фрагменты с другим — <gN>…</gN>, элементы Word без текста (табуляция, картинка, поле) — <xN/>.
// Спецсимволы экранированы: &amp; &lt; &gt;.

export type TaggedPart = { text: string; tag: string } | { inline: string }

const tagPattern = /<(g\d+)>([\s\S]*?)<\/\1>|<(x\d+)\/>/g
const entities: Record<string, string> = { '&lt;': '<', '&gt;': '>', '&amp;': '&' }
const unescape = (value: string) => value.replace(/&lt;|&gt;|&amp;/g, (entity) => entities[entity])

// Разбирает текст на части; сломанные теги (например, в машинном переводе) остаются текстом.
export function parseTagged(text: string): TaggedPart[] {
  const parts: TaggedPart[] = []
  let last = 0
  for (const match of text.matchAll(tagPattern)) {
    if (match.index > last) parts.push({ text: unescape(text.slice(last, match.index)), tag: 'g0' })
    parts.push(match[3] ? { inline: match[3] } : { text: unescape(match[2]), tag: match[1] })
    last = match.index + match[0].length
  }
  if (last < text.length) parts.push({ text: unescape(text.slice(last)), tag: 'g0' })
  return parts
}

// Документ редактора TipTap: один абзац; фрагмент <gN> — текст с меткой g (id), <xN/> — узел x.
type EditorMark = { type: string; attrs?: { id?: string } }
export type EditorNode = { type: string; text?: string; marks?: EditorMark[]; attrs?: { id?: string }; content?: EditorNode[] }

export function toEditorDoc(text: string): EditorNode {
  const content: EditorNode[] = parseTagged(text).flatMap((part): EditorNode[] => {
    if ('inline' in part) return [{ type: 'x', attrs: { id: part.inline } }]
    if (!part.text) return []
    return [{ type: 'text', text: part.text, ...(part.tag === 'g0' ? {} : { marks: [{ type: 'g', attrs: { id: part.tag } }] }) }]
  })
  return { type: 'doc', content: [{ type: 'paragraph', content }] }
}

const escape = (value: string) => value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')

// Обратно в текст с тегами: соседний текст с одной меткой — один тег.
export function fromEditorDoc(doc: EditorNode): string {
  let result = ''
  let open = ''
  const close = () => { if (open) result += `</${open}>`; open = '' }
  for (const paragraph of doc.content ?? []) {
    if (result && !open) result += ' ' // вставка из нескольких абзацев — один сегмент
    for (const node of paragraph.content ?? []) {
      if (node.type === 'x') {
        close()
        result += `<${node.attrs?.id}/>`
        continue
      }
      const tag = node.marks?.find((mark) => mark.type === 'g')?.attrs?.id ?? ''
      if (tag !== open) {
        close()
        if (tag) result += `<${tag}>`
        open = tag
      }
      result += escape(node.text ?? '')
    }
    close()
  }
  return result
}
