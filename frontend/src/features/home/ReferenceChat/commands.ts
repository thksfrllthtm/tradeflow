import type { AgentMessage } from '@/features/goods/agent/agentApi'

// Команды поля вопроса: «/» в начале — меню, «/док 8205591000» — сразу команда (имя можно сокращать).
export type CommandName = 'подбор' | 'троис' | 'документы' | 'сравнить' | 'новый'
export type Command = { name: CommandName; label: string; hint: string; arg?: string }

const commands: Command[] = [
  { name: 'подбор', label: 'Подбор кода', hint: 'новый вопрос, даже если открыт другой', arg: 'товар' },
  { name: 'троис', label: 'Проверка бренда по ТРОИС', hint: 'фото или название — есть ли знак в реестре, похожие написания', arg: 'бренд или фото' },
  { name: 'документы', label: 'Документы для ввоза', hint: 'по коду ответа или товар и код', arg: 'товар код' },
  { name: 'сравнить', label: 'Сравнить коды', hint: 'чем отличаются и когда какой', arg: 'код и код' },
  { name: 'новый', label: 'Новый вопрос', hint: 'очистить и начать заново' },
]

const prefixed = (query: string) => commands.filter((command) => command.name.startsWith(query.toLowerCase()))

// Меню: пока набрано только «/имя» без пробела — подходящие команды.
export function menuCommands(text: string): Command[] {
  const match = /^\/(\S*)$/.exec(text)
  return match ? prefixed(match[1]) : []
}

// Текст как в пузыре запроса — «Документы для ввоза · 3827 61 000 0» (после ошибки его вставляют
// повторно) — та же команда, а не вопрос агенту.
const documentsText = /^документы(?:\s+для\s+ввоза)?(?:\s*[·:—–-]?\s*([\d\s]+))?\.?$/i

// «/имя аргумент» → команда; null — текст не команда; unknown — после «/» нет такой команды.
export function parseCommand(text: string): { command: Command; arg: string } | 'unknown' | null {
  const documents = documentsText.exec(text.trim())
  if (documents) return { command: commands.find((command) => command.name === 'документы')!, arg: (documents[1] ?? '').trim() }
  const match = /^\/(\S+)\s*([\s\S]*)$/.exec(text.trim())
  if (!match) return null
  const found = prefixed(match[1])
  return found.length === 1 ? { command: found[0], arg: match[2].trim() } : 'unknown'
}

// «/документы Наждачный круг P120 6804 22 500 0» → код (подряд идущие числа, вместе 10 цифр; ищется с
// конца) и товар (остальные слова). Кода нет — все цифры кодом (дальше «Код — 10 цифр»), товар — весь текст.
export function splitDocumentsArg(arg: string): { code: string; product: string } {
  const words = arg.trim().split(/\s+/)
  for (let end = words.length; end > 0; end--) {
    for (let start = end - 1; start >= 0 && /^[\d.]+$/.test(words[start]); start--) {
      const code = words.slice(start, end).join('').replace(/\D/g, '')
      if (code.length === 10) return { code, product: [...words.slice(0, start), ...words.slice(end)].join(' ') }
      if (code.length > 10) break
    }
  }
  return { code: arg.replace(/\D/g, ''), product: arg.trim() }
}

// Код для «документов»: из аргумента (10 цифр, пробелы можно) или лучший код последнего ответа.
export function documentsCode(arg: string, messages: AgentMessage[]): string {
  const typed = splitDocumentsArg(arg).code
  if (typed) return typed
  for (let at = messages.length - 1; at >= 0; at--) {
    const message = messages[at]
    if (message.kind !== 'result' || message.content.documentsCode) continue
    const best = message.content.candidates.find((candidate) => candidate.verdict === 'fits') ?? message.content.candidates.find((candidate) => candidate.verdict === 'doubtful')
    return best?.code ?? ''
  }
  return ''
}
