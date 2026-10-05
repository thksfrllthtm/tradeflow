import { GoodsCalculationResult, GoodsItemInput } from '@/shared/types/goods'
import { autoNetFor, effectivePackageWeight, netHint } from '../../lib/netWeight'
import { RowCheck, rowSignature } from '../../lib/reviewCheck'

type ResultItem = GoodsCalculationResult['items'][number]
// todo — кода ещё нет; changed — строку изменили после расчёта, нужна новая проверка.
export type ReviewTone = 'ok' | 'warn' | 'bad' | 'off' | 'todo' | 'changed'
// current — результат расчёта соответствует строкам; past — ошибки прошлого расчёта (строки с тех
// пор менялись); none — расчёта не было или строка добавлена после него.
export type ReviewSource = 'current' | 'past' | 'none'
export type ReviewStatus = {
  tone: ReviewTone
  source: ReviewSource
  label: string
  // Сообщения, которые показываются под названием: ошибки и предупреждения (кроме цены) или
  // недостающее для расчёта.
  issues: string[]
  attention: boolean
  noCode: boolean
  withErrors: boolean
}

// Цена и стоимость вводятся на этапе 3 — их ошибки здесь не показываем,
// иначе строка выглядит сломанной, хотя на этом этапе исправить нечего.
const PRICE_ISSUE = /цен[аеуы]|стоимост/i
const reviewOnly = (list?: string[]) => (list ?? []).map((text) => text.trim()).filter((text) => text && !PRICE_ISSUE.test(text))

export const hasNomenclatureRates = (item: GoodsItemInput) => item.directorySnapshot?.rateSource === 'nomenclature'

// Статус строки на этапе 2. Источник ошибок — актуальный результат расчёта, а если его нет —
// копия ошибок прошлого расчёта по id строки. Изменённая после расчёта строка не считается
// исправленной: «Изменено — требуется пересчёт», и она остаётся среди требующих внимания.
// missing — что видно и без расчёта (например, нет брутто для авто-нетто).
export function reviewStatus(item: GoodsItemInput, current?: ResultItem, past?: RowCheck, missing?: string): ReviewStatus {
  const noCode = !item.excluded && !item.code
  if (item.excluded) return { tone: 'off', source: 'none', label: 'Исключена из расчёта', issues: [], attention: false, noCode: false, withErrors: false }

  const changed = !current && past !== undefined && past.signature !== rowSignature(item)
  const check = current ? { errors: current.errors, warnings: current.warnings, calculated: current.status === 'calculated' } : past
  const source: ReviewSource = current ? 'current' : past ? 'past' : 'none'
  const errors = reviewOnly(check?.errors)
  const warnings = reviewOnly(check?.warnings)
  const verified = hasNomenclatureRates(item)
  const local = [!item.code ? 'Нужен код ТН ВЭД' : !verified && !check?.calculated ? 'Кода нет в номенклатуре ТН ВЭД ЕАЭС — проверьте код' : '', missing ?? ''].filter(Boolean)

  if (changed) {
    const hadIssues = errors.length + warnings.length > 0
    return { tone: 'changed', source, label: 'Изменено — требуется пересчёт', issues: [...errors, ...warnings, ...local], attention: hadIssues || local.length > 0, noCode, withErrors: errors.length > 0 }
  }
  if (errors.length) return { tone: 'bad', source, label: source === 'past' ? 'Ошибка прошлого расчёта' : 'Ошибка расчёта', issues: [...errors, ...warnings], attention: true, noCode, withErrors: true }
  if (local.length && !item.code) return { tone: 'todo', source, label: 'Нужен код ТН ВЭД', issues: [...local.slice(1), ...warnings], attention: true, noCode, withErrors: false }
  if (warnings.length || local.length) return { tone: 'warn', source, label: warnings.length ? (source === 'past' ? 'Предупреждение прошлого расчёта' : 'Предупреждение расчёта') : 'Проверьте строку', issues: [...warnings, ...local], attention: true, noCode, withErrors: false }
  // Без ошибок: «рассчитано» — только по расчёту; до расчёта честно — «код готов».
  const label = source === 'current' ? 'Рассчитано' : source === 'past' ? 'Без ошибок в прошлом расчёте' : 'Код готов, расчёта ещё не было'
  return { tone: 'ok', source, label, issues: [], attention: false, noCode, withErrors: false }
}

// Что видно и без расчёта: нетто не число или больше брутто, нет брутто и мест для авто-нетто.
// Те же проверки, что у подсказки поля «Нетто»; числа — через запятую (только текст для показа).
export function rowMissing(item: GoodsItemInput, defaultPackageWeight: string): string | undefined {
  if (item.excluded) return undefined
  const netItem = { ...item, packageWeightKg: effectivePackageWeight(item, defaultPackageWeight) }
  const hint = netHint(netItem)
  const text = hint.className === 'net-bad' ? hint.text : !item.netKg.trim() && autoNetFor(netItem) === null ? hint.text : ''
  return text ? text.replace(/^✕\s*/, '').replace(/(\d)\.(\d)/g, '$1,$2') : undefined
}
