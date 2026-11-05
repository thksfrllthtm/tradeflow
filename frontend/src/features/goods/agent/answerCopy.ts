import { formatRuDate } from '@/shared/lib/dates'
import { formatCode } from '@/shared/tnved/codes'
import { goodsNote, shortHolder, TrademarkHit, trademarkName } from '@/shared/trademarks/trademarks'
import type { AgentDocument, AgentResult, BrandCheck } from './agentApi'

// Ответ ИИ обычным текстом — чтобы переслать в WhatsApp или почту: без хода поиска и служебных пометок,
// только то, что нужно коллеге или клиенту. Абзацы — через пустую строку.

const verdictText = { fits: 'подходит', doubtful: 'возможно', no: 'не подходит' } as const
const kindText: Record<NonNullable<AgentDocument['kind']>, string> = { cc: 'СС', dc: 'ДС', sgr: 'СГР', conditional: 'Условный выпуск', other: '' }

type Part = string | false | null | undefined
const join = (parts: Part[], separator: string) => parts.filter((part): part is string => Boolean(part && part.trim())).join(separator)

export function trademarksText(hits: TrademarkHit[] | undefined): string {
  if (!hits?.length) return ''
  return join(['ТРОИС:', ...hits.map((hit) => {
    const what = hit.kind === 'lookalike' ? `похоже на ${trademarkName(hit)} — возможная имитация` : `знак ${trademarkName(hit)}`
    const note = goodsNote(hit)
    const details = join([`№ ${hit.regNo}`, hit.holder && shortHolder(hit.holder), hit.protectedUntil && `защита до ${formatRuDate(hit.protectedUntil)}`], ', ')
    return `• ${what}${note ? ` (${note.toLowerCase()})` : ''}; ${details}`
  })], '\n')
}

// Подбор кода: товар, коды с вердиктом, ставками и причиной, наименование для декларации, заметки, ТРОИС.
export function resultText(result: AgentResult): string {
  const main = result.candidates.filter((candidate) => candidate.verdict !== 'no')
  const codes = main.map((candidate, index) => join([
    `${index === 0 ? 'Код ТН ВЭД' : 'Ещё вариант'}: ${formatCode(candidate.code)} — ${verdictText[candidate.verdict]}`,
    candidate.nomenclature && `Пошлина ${candidate.nomenclature.tariffText || 'не указана'}, НДС ${candidate.nomenclature.vatPercent || '16'}%`,
    candidate.reason, candidate.check && `Уточнить: ${candidate.check}`,
  ], '\n'))
  return join([
    result.description && `Товар: ${result.description}`,
    ...codes,
    result.name && `Наименование для декларации: ${result.name}`,
    result.nameFull && result.nameFull !== result.name && `Подробно: ${result.nameFull}`,
    result.notes && `Примечание: ${result.notes}`,
    trademarksText(result.trademarks),
  ], '\n\n')
}

// «Документы для ввоза»: нужно и возможно — с требованием, не относящиеся — списком названий.
export function documentsText(result: AgentResult): string {
  const documents = result.documents ?? []
  const row = (item: AgentDocument) => {
    const kind = item.kind ? kindText[item.kind] : ''
    return join([`• ${kind ? `${kind}: ` : ''}${item.measure}${item.act ? ` (${item.act})` : ''}`, item.requirement && `  ${item.requirement}`, item.nameHint && `  ${item.nameHint}`], '\n')
  }
  const group = (title: string, items: AgentDocument[]) => items.length > 0 && join([title, ...items.map(row)], '\n')
  const rejected = documents.filter((item) => item.applies === 'no')
  return join([
    `Документы для ввоза${result.documentsCode ? ` · ${formatCode(result.documentsCode)}` : ''}`,
    result.description,
    group('Нужно:', documents.filter((item) => item.applies === 'yes')),
    group('Возможно, уточнить:', documents.filter((item) => item.applies === 'maybe')),
    rejected.length > 0 && `Не относятся к товару: ${rejected.map((item) => item.measure).join('; ')}`,
    documents.length === 0 && 'Мер для ввоза не найдено.',
    'Подсказка по справке tnved.info и актам — перед оформлением сверьте с актом.',
  ], '\n\n')
}

// Проверка «/троис»: бренды с фото и найденные знаки.
export function brandText(check: BrandCheck): string {
  return join([
    check.brands.length > 0 && `На фото: ${check.brands.join(', ')}`,
    check.hits.length > 0 ? trademarksText(check.hits) : 'В реестре ТРОИС по написанию не найдено.',
    check.hits.length > 0 && 'Решение за декларантом: совпадение — повод проверить, нужен ли товарный знак и согласие правообладателя.',
  ], '\n\n')
}

// Короткие варианты — только главное: код, наименование, ставки; по документам — названия нужных мер;
// по ТРОИС — есть ли знак. Без обоснований, заметок и правообладателей.

export function resultShortText(result: AgentResult): string {
  const main = result.candidates.filter((candidate) => candidate.verdict !== 'no')
  const [best, ...others] = main
  if (!best) return ''
  return join([
    `Код ТН ВЭД: ${formatCode(best.code)}${best.verdict === 'doubtful' ? ' (возможно)' : ''}`,
    others.length > 0 && `Другие варианты: ${others.map((candidate) => formatCode(candidate.code)).join(', ')}`,
    result.name && `Наименование: ${result.name}`,
    best.nomenclature && `Пошлина ${best.nomenclature.tariffText || 'не указана'}, НДС ${best.nomenclature.vatPercent || '16'}%`,
    (result.trademarks?.length ?? 0) > 0 && `ТРОИС: ${result.trademarks!.map(trademarkName).join(', ')} — есть в реестре`,
  ], '\n')
}

export function documentsShortText(result: AgentResult): string {
  const documents = result.documents ?? []
  const title = (item: AgentDocument) => {
    const kind = item.kind ? kindText[item.kind] : ''
    return kind && kind !== 'Условный выпуск' ? `${kind} — ${item.measure}` : item.measure
  }
  const yes = documents.filter((item) => item.applies === 'yes').map(title)
  const maybe = documents.filter((item) => item.applies === 'maybe').map(title)
  return join([
    `Документы для ввоза${result.documentsCode ? ` · ${formatCode(result.documentsCode)}` : ''}`,
    yes.length > 0 ? `Нужно: ${yes.join('; ')}` : 'Обязательных документов не найдено',
    maybe.length > 0 && `Возможно: ${maybe.join('; ')}`,
  ], '\n')
}

export function brandShortText(check: BrandCheck): string {
  return check.hits.length > 0 ? `ТРОИС: ${check.hits.map(trademarkName).join(', ')} — есть в реестре` : 'ТРОИС: в реестре не найдено'
}
