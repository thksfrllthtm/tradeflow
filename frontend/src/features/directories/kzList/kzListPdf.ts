// Чтение «Перечня РК» (PDF ЕЭК) в браузере. Русский текст в PDF набран шрифтами без таблицы
// символов: простые программы его не видят, а pdf.js восстанавливает. Раскладка страницы:
// код у левого края (x ≈ 104), наименование (x 185–415), ставка (x от 415); номер сноски —
// мелкая цифра над строкой ставки; тексты сносок — в конце документа под чертой «_____».

export type PdfItem = { str: string; x: number; y: number; size: number; w: number }
export type KzListRow = { code: string; name: string; rate: string; notes: string[]; page: number }
export type KzListExtract = { fileName: string; rows: KzListRow[]; footnotes: Record<string, string> }

const codeX = [95, 115]
const nameX = 185
const rateX = 415
const smallFont = 12

function joinLine(items: PdfItem[]) {
  let text = ''
  let end: number | null = null
  for (const item of [...items].sort((a, b) => a.x - b.x)) {
    if (!item.str) continue
    if (end !== null && item.x - end > 1.5 && !text.endsWith(' ') && !item.str.startsWith(' ')) text += ' '
    text += item.str
    end = item.x + item.w
  }
  return text.replace(/\s+/g, ' ').trim()
}

function groupByY(items: PdfItem[], tolerance: number) {
  const lines = new Map<number, PdfItem[]>()
  for (const item of items) {
    const key = [...lines.keys()].find((y) => Math.abs(y - item.y) < tolerance) ?? item.y
    lines.set(key, [...(lines.get(key) ?? []), item])
  }
  return [...lines.entries()].sort((a, b) => b[0] - a[0]).map(([, list]) => list)
}

const linesOf = (items: PdfItem[]) => groupByY(items, 3).map(joinLine).filter(Boolean)
const isRule = (item: PdfItem) => /^_{5,}$/.test(item.str.trim())

// Разбор уже прочитанных страниц (элементы текста с координатами) — без pdf.js, для тестов.
export function parseKzPages(pages: PdfItem[][], fileName = ''): KzListExtract {
  const rows: KzListRow[] = []
  const footnotes: Record<string, string> = {}
  let footnoteMode = false
  let currentNote: string | null = null
  pages.forEach((all, index) => {
    const page = index + 1
    const items = all.filter((item) => item.str !== '')
    const rule = footnoteMode ? undefined : items.find(isRule)
    const bottom = footnoteMode ? Infinity : rule ? rule.y : -Infinity
    if (rule || footnoteMode) {
      footnoteMode = true
      // Номер страницы вверху (y > 780) — не сноска.
      const notes = items.filter((item) => item.y < (rule ? rule.y : Infinity) && !isRule(item) && item.y <= 780)
      for (const line of groupByY(notes, 7)) {
        const mark = line.find((item) => item.size < smallFont && /^\d+$/.test(item.str.trim()) && item.x < 100)
        const text = joinLine(line.filter((item) => item !== mark))
        if (mark) { currentNote = mark.str.trim(); footnotes[currentNote] = text }
        else if (currentNote && text) footnotes[currentNote] = `${footnotes[currentNote]} ${text}`.trim()
      }
    }
    const body = items.filter((item) => item.y > bottom + 2)
    // Начало строки — код у левого края; он бывает разбит на куски («44» + «07 96 100 3»).
    const starts: { y: number; code: string }[] = []
    for (const item of body) {
      if (item.x > codeX[0] && item.x < codeX[1] && /^\d/.test(item.str.trim()) && item.size >= smallFont) {
        const code = joinLine(body.filter((other) => Math.abs(other.y - item.y) < 2 && other.x >= item.x - 1 && other.x < nameX)).replace(/\s/g, '')
        if (/^\d{10}$/.test(code) && !starts.some((start) => Math.abs(start.y - item.y) < 2)) starts.push({ y: item.y, code })
      }
    }
    starts.sort((a, b) => b.y - a.y)
    starts.forEach((start, position) => {
      const nextY = starts[position + 1]?.y ?? bottom
      const inRow = (item: PdfItem) => item.y <= start.y + 2 && item.y > nextY + 2
      const nameItems = body.filter((item) => inRow(item) && item.x >= nameX && item.x < rateX && item.size >= smallFont)
      const rateItems = body.filter((item) => inRow(item) && item.x >= rateX && item.size >= smallFont)
      // Номер сноски — мелкая цифра над строкой ставки: относится к строке, в которой эта ставка.
      const marks = body.filter((item) => item.x >= rateX && item.size < smallFont && /^\d+$/.test(item.str.trim())).filter((mark) => {
        const under = body.filter((item) => item.x >= rateX && item.size >= smallFont && item.y < mark.y - 1 && item.y > mark.y - 9).sort((a, b) => b.y - a.y)[0]
        return under !== undefined && inRow(under)
      })
      // Двузначный номер приходит по цифрам («1», «3» = 13): соседние цифры склеиваются.
      const notes: string[] = []
      let previous: PdfItem | null = null
      for (const mark of marks.sort((a, b) => b.y - a.y || a.x - b.x)) {
        if (previous && Math.abs(previous.y - mark.y) < 1.5 && mark.x - (previous.x + previous.w) < 1.5) notes[notes.length - 1] += mark.str.trim()
        else notes.push(mark.str.trim())
        previous = mark
      }
      rows.push({ page, code: start.code, name: linesOf(nameItems).join(' '), rate: linesOf(rateItems).join(' '), notes })
    })
  })
  return { fileName, rows, footnotes }
}

// Читает PDF целиком; pdf.js загружается только здесь, чтобы не утяжелять остальной сайт.
export async function readKzPdf(file: File, onProgress?: (page: number, total: number) => void): Promise<KzListExtract> {
  const [pdfjs, worker] = await Promise.all([import('pdfjs-dist/legacy/build/pdf.mjs'), import('pdfjs-dist/legacy/build/pdf.worker.min.mjs?url')])
  pdfjs.GlobalWorkerOptions.workerSrc = worker.default
  const document = await pdfjs.getDocument({ data: new Uint8Array(await file.arrayBuffer()), verbosity: 0 }).promise
  const pages: PdfItem[][] = []
  for (let number = 1; number <= document.numPages; number++) {
    const content = await (await document.getPage(number)).getTextContent()
    pages.push(content.items.flatMap((item) => 'str' in item ? [{ str: item.str, x: item.transform[4], y: item.transform[5], size: Math.round(item.transform[0]), w: item.width }] : []))
    onProgress?.(number, document.numPages)
  }
  await document.destroy()
  return parseKzPages(pages, file.name)
}
