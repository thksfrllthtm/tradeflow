import type { AgentMessage } from '@/features/goods/agent/agentApi'

// Команды поля вопроса: «/» в начале — меню, «/док 8205591000» — сразу команда (имя можно сокращать).
export type CommandName = 'подбор' | 'документы' | 'сравнить' | 'новый'
export type Command = { name: CommandName; label: string; hint: string; arg?: string }

export const commands: Command[] = [
  { name: 'подбор', label: 'Подбор кода', hint: 'новый вопрос, даже если открыт другой', arg: 'товар' },
  { name: 'документы', label: 'Документы для ввоза', hint: 'по коду ответа или указанному', arg: 'код' },
  { name: 'сравнить', label: 'Сравнить коды', hint: 'чем отличаются и когда какой', arg: 'код и код' },
  { name: 'новый', label: 'Новый вопрос', hint: 'очистить и начать заново' },
]

const prefixed = (query: string) => commands.filter((command) => command.name.startsWith(query.toLowerCase()))

// Меню: пока набрано только «/имя» без пробела — подходящие команды.
export function menuCommands(text: string): Command[] {
  const match = /^\/(\S*)$/.exec(text)
  return match ? prefixed(match[1]) : []
}

// «/имя аргумент» → команда; null — текст не команда; unknown — после «/» нет такой команды.
export function parseCommand(text: string): { command: Command; arg: string } | 'unknown' | null {
  const match = /^\/(\S+)\s*([\s\S]*)$/.exec(text.trim())
  if (!match) return null
  const found = prefixed(match[1])
  return found.length === 1 ? { command: found[0], arg: match[2].trim() } : 'unknown'
}

// Код для «документов»: из аргумента (10 цифр, пробелы можно) или лучший код последнего ответа.
export function documentsCode(arg: string, messages: AgentMessage[]): string {
  const typed = arg.replace(/\D/g, '')
  if (typed) return typed
  for (let at = messages.length - 1; at >= 0; at--) {
    const message = messages[at]
    if (message.kind !== 'result' || message.content.documentsCode) continue
    const best = message.content.candidates.find((candidate) => candidate.verdict === 'fits') ?? message.content.candidates.find((candidate) => candidate.verdict === 'doubtful')
    return best?.code ?? ''
  }
  return ''
}
