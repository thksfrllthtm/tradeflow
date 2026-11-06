import { GoodsCalculationResult } from '@/shared/types/goods'
import { formatCode } from '@/shared/tnved/codes'

// Коды ТН ВЭД в тексте сервера — как в таблице: «8716903000» → «8716 90 300 0».
const withFormattedCodes = (text: string) => text.replace(/\b\d{10}\b/g, formatCode)

// Строка без индикатора СУР считается по инвойсу («СУР: … — ТС по инвойсу») — это не ошибка.
export const isSirFallback = (text: string) => /^СУР: .*— ТС по инвойсу$/.test(text.trim())
// Нет индикатора для кода — видно по «нет» в колонке СУР и счётчику в полосе СУР, отдельно не показываем.
const sirNoIndicator = (text: string) => /^СУР: для кода .* индикатора нет/.test(text.trim())

// [2,3,4,5,9] → [{ label: "2–5", first: 2 }, { label: "9", first: 9 }]
export function rowRangeParts(rows: number[]) {
  const sorted = [...rows].sort((a, b) => a - b)
  const parts: { label: string; first: number }[] = []
  for (let i = 0; i < sorted.length; i++) {
    const start = sorted[i]
    while (sorted[i + 1] === sorted[i] + 1) i++
    parts.push({ label: start === sorted[i] ? String(start) : `${start}–${sorted[i]}`, first: start })
  }
  return parts
}

// Тип проблемы строки по тексту сервера (backend calculation/item.go): нет кода или ставок, нет цены, прочее.
type IssueKind = 'code' | 'price' | 'other'
function issueKind(text: string): IssueKind {
  if (/^Не выбран[аы] (карточка ТН ВЭД|ставки)|код ТН ВЭД/i.test(text)) return 'code'
  if (/^Укажите цену за единицу/i.test(text)) return 'price'
  return 'other'
}

// error — расчёт строки невозможен (красный), todo — следующий шаг этапа 3 (нет цены), warn — предупреждение.
export type ProblemTone = 'error' | 'todo' | 'warn'
export type RowProblem = { kind: IssueKind; tone: ProblemTone; text: string }

type ProblemItem = { errors?: string[]; warnings?: string[] }

// Проблемы строки без дублей: «Не выбрана карточка ТН ВЭД» и «Не выбраны ставки» — одна проблема кода.
export function rowProblems(item: ProblemItem, code: string): RowProblem[] {
  const found = new Map<string, RowProblem>()
  const lists: [string[] | undefined, ProblemTone][] = [[item.errors, 'error'], [item.warnings, 'warn']]
  for (const [list, tone] of lists) {
    for (const raw of list ?? []) {
      const text = raw.trim()
      if (!text || sirNoIndicator(text)) continue
      const kind = issueKind(text)
      const problem: RowProblem = kind === 'code' ? { kind, tone: 'error', text: code.trim() ? 'Нет ставок для кода ТН ВЭД' : 'Не указан код ТН ВЭД' }
        : kind === 'price' ? { kind, tone: 'todo', text: 'Не указана цена' }
        : { kind, tone, text: withFormattedCodes(text) }
      if (!found.has(problem.text)) found.set(problem.text, problem)
    }
  }
  return [...found.values()]
}

export type ProblemGroup = RowProblem & { rows: number[] }
const TONE_ORDER: Record<ProblemTone, number> = { error: 0, todo: 1, warn: 2 }

// Проблемы сметы для баннера: одна строка на проблему с номерами строк (№ в таблице). Общие ошибки
// сервера (blockingErrors, которых нет у строк) — без номеров. Исключённые строки не входят.
export function problemGroups(result: GoodsCalculationResult, codes: string[]): ProblemGroup[] {
  const groups = new Map<string, ProblemGroup>()
  const rowTexts = new Set<string>()
  result.items.forEach((item, index) => {
    if (item.excluded) return
    for (const text of [...(item.errors ?? []), ...(item.warnings ?? [])]) rowTexts.add(text.trim())
    for (const problem of rowProblems(item, codes[index] ?? '')) {
      const group = groups.get(problem.text) ?? { ...problem, rows: [] }
      group.rows.push(index + 1)
      groups.set(problem.text, group)
    }
  })
  for (const raw of result.totals.blockingErrors ?? []) {
    const text = raw.trim()
    if (text && !rowTexts.has(text) && !groups.has(text)) groups.set(text, { kind: issueKind(text), tone: 'error', text, rows: [] })
  }
  return [...groups.values()].sort((a, b) => TONE_ORDER[a.tone] - TONE_ORDER[b.tone] || b.rows.length - a.rows.length)
}

// Проблемы больше чем у половины строк (rows — включённые строки, в том числе без проблем): они уже
// в баннере, и у строк не повторяются ни значком, ни текстом — видно только то, чем строка отличается.
export function commonProblems(rows: string[][]) {
  const counts = new Map<string, number>()
  for (const texts of rows) for (const text of texts) counts.set(text, (counts.get(text) ?? 0) + 1)
  return new Set([...counts].filter(([, count]) => count * 2 > rows.length).map(([text]) => text))
}
