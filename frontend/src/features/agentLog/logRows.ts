import { formatCode } from '@/shared/tnved/codes'
import { LogModel, LogRow } from './agentLogApi'

// Подписи строк журнала: итог модели, сравнение, доля и время — без React, чтобы проверять тестами.

export type Tone = 'ok' | 'warn' | 'bad' | 'muted'

// Статусы подбора и тени по-русски.
export const statusLabels: Record<string, string> = {
  searching: 'поиск в базе', ready: 'готов к запуску', running: 'идёт', waiting: 'ждёт ответа', done: 'готово',
  error: 'сбой', cancelled: 'остановлен', skipped: 'пропущено',
}

// Доля в процентах; нет знаменателя — «—».
export function percent(part: number, whole: number) {
  return whole > 0 ? `${Math.round((part * 100) / whole)}%` : '—'
}

// Итог модели одной строкой: код, либо почему кода нет.
export function modelOutcome(model: LogModel | null): { text: string; tone: Tone } {
  if (!model) return { text: '—', tone: 'muted' }
  switch (model.status) {
    case 'skipped': return { text: 'пропущено', tone: 'muted' }
    case 'running': case 'searching': case 'ready': return { text: 'идёт', tone: 'muted' }
    case 'waiting': return { text: 'ждёт ответа', tone: 'muted' }
    case 'error': return { text: 'сбой', tone: 'bad' }
    case 'cancelled': return { text: 'остановлен', tone: 'muted' }
  }
  return model.bestCode ? { text: formatCode(model.bestCode), tone: 'ok' } : { text: 'без итога', tone: 'warn' }
}

// Согласие моделей по коду и по документам, правота по проверенной записи.
export function comparison(row: Pick<LogRow, 'agree' | 'mainCorrect' | 'shadowCorrect' | 'decision' | 'documentsAgree'>): { text: string; tone: Tone }[] {
  const marks: { text: string; tone: Tone }[] = []
  if (row.agree !== null) marks.push(row.agree ? { text: 'код совпал', tone: 'ok' } : { text: 'код разошёлся', tone: 'warn' })
  if (row.decision?.status === 'verified') {
    if (row.mainCorrect !== null) marks.push(row.mainCorrect ? { text: 'основная права', tone: 'ok' } : { text: 'основная ошиблась', tone: 'bad' })
    if (row.shadowCorrect !== null) marks.push(row.shadowCorrect ? { text: 'тень права', tone: 'ok' } : { text: 'тень ошиблась', tone: 'bad' })
  }
  if (row.documentsAgree !== null && row.documentsAgree !== undefined) marks.push(row.documentsAgree ? { text: 'документы совпали', tone: 'ok' } : { text: 'документы разошлись', tone: 'bad' })
  return marks
}

// «1 мин 42 с», «42 с»; нет — «—».
export function durationText(seconds: number | null | undefined) {
  if (seconds === null || seconds === undefined || !Number.isFinite(seconds)) return '—'
  const whole = Math.round(seconds)
  const minutes = Math.floor(whole / 60)
  return minutes > 0 ? `${minutes} мин ${whole % 60} с` : `${whole} с`
}

// Стоимость коротко: «$0.54», мелочь — «$0.0046», ноль — «$0».
export function costText(costUsd: string) {
  const value = Number(costUsd)
  if (!Number.isFinite(value) || value === 0) return '$0'
  return value >= 0.01 ? `$${value.toFixed(2)}` : `$${value.toFixed(4)}`
}

// Строка под кодом в таблице: стоимость, время, шаги.
export function metaText(model: LogModel) {
  return `${costText(model.costUsd)} · ${durationText(model.seconds)} · ${model.steps} ш.`
}

// «1–50 из 136» для подвала таблицы.
export function rangeText(page: number, limit: number, total: number) {
  if (total === 0) return 'нет подборов'
  const from = (page - 1) * limit + 1
  return `${from}–${Math.min(page * limit, total)} из ${total}`
}

// Дата N дней назад в виде ГГГГ-ММ-ДД по местному времени (для полей периода).
export function isoDaysAgo(days: number, now = new Date()) {
  const date = new Date(now.getFullYear(), now.getMonth(), now.getDate() - days)
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}
