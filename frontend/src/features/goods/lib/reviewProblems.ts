import { GoodsItemInput } from '@/shared/types/goods'
import type { ReviewStatus } from '../components/ReviewRow/reviewStatus'
import type { SheetKey } from '../components/ReviewSheet/sheetModel'

// Проблемы строки этапа «Проверка строк» по типам — для чипов-фильтров над таблицей и подсветки ячейки,
// к которой проблема относится. Сами проверки не меняются: это разбор того, что уже нашли reviewStatus
// (код, номенклатура, авто-нетто) и прошлый расчёт (тексты ошибок и предупреждений сервера).
export type ProblemTone = 'error' | 'warn'
export type ReviewProblem = { type: string; label: string; tone: ProblemTone; column: SheetKey; text: string }

const CODE = /код|номенклатур|ставк|карточк/i
const WEIGHT = /нетто|брутто|упаковк|\bвес/i
const QUANTITY = /количеств|мест|в месте|\bпар/i
// Нетто не число, больше брутто или авто-нетто не сходится — ошибка; «укажите брутто» — предупреждение.
const WEIGHT_ERROR = /не число|не больше брутто|больше 0|проверьте брутто/i

export function reviewProblems(item: GoodsItemInput, status: ReviewStatus): ReviewProblem[] {
  if (item.excluded) return []
  const bad = status.tone === 'bad'
  const found: ReviewProblem[] = []
  const add = (problem: ReviewProblem) => { if (!found.some((other) => other.type === problem.type && other.text === problem.text)) found.push(problem) }
  if (status.noCode) add({ type: 'noCode', label: 'Нет кода', tone: 'error', column: 'code', text: 'Код ТН ВЭД не указан' })
  for (const raw of status.issues) {
    const text = raw.trim()
    if (!text || (status.noCode && /^Нужен код/i.test(text))) continue
    if (WEIGHT.test(text)) {
      add({ type: 'weight', label: 'Вес не сходится', tone: bad || WEIGHT_ERROR.test(text) ? 'error' : 'warn', column: /брутто/i.test(text) && !/нетто/i.test(text) ? 'grossKg' : 'netKg', text })
    } else if (QUANTITY.test(text)) {
      add({ type: 'quantity', label: 'Количество не сходится', tone: bad ? 'error' : 'warn', column: 'pairs', text })
    } else if (CODE.test(text)) {
      // Строка без кода уже отмечена «Нет кода» — «Проверьте код» её только дублирует.
      if (!status.noCode) add({ type: 'code', label: 'Проверьте код', tone: bad ? 'error' : 'warn', column: 'code', text })
    } else {
      add({ type: `other:${text}`, label: text, tone: bad ? 'error' : 'warn', column: 'name', text })
    }
  }
  return found
}

export type ProblemChip = { type: string; label: string; tone: ProblemTone; rows: number }

// Чипы над таблицей: тип проблемы и сколько строк с ней; ошибки — первыми.
export function problemChips(rows: ReviewProblem[][]): ProblemChip[] {
  const chips = new Map<string, ProblemChip>()
  for (const problems of rows) {
    const seen = new Set<string>()
    for (const problem of problems) {
      if (seen.has(problem.type)) continue
      seen.add(problem.type)
      const chip = chips.get(problem.type) ?? { type: problem.type, label: problem.label, tone: problem.tone, rows: 0 }
      chip.rows++
      if (problem.tone === 'error') chip.tone = 'error'
      chips.set(problem.type, chip)
    }
  }
  return [...chips.values()].sort((a, b) => (a.tone === b.tone ? b.rows - a.rows : a.tone === 'error' ? -1 : 1))
}
