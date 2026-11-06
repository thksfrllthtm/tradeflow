import { AgentSession } from '@/features/goods/agent/agentApi'

export const isWorking = (session: AgentSession | null) => session?.status === 'running' || session?.status === 'searching'

// Что озвучить экранному диктору при смене состояния вопроса.
export function liveText(session: AgentSession | null) {
  if (!session) return ''
  if (session.status === 'searching') return 'Ищу в базе решений'
  if (session.status === 'running') return session.trademarkCheck ? 'Проверяю бренд по ТРОИС' : 'Агент ищет код'
  if (session.status === 'waiting') return 'Агент задал уточняющий вопрос'
  if (session.status === 'error') return 'Подбор прервался'
  if (session.status === 'done') return 'Ответ готов'
  return ''
}

// Имя автора чужого чата в родительном падеже для подписей («Чат Айгерим», «от коллеги»): нет имени — «коллеги».
export function authorName(name?: string) {
  return name?.trim() || 'коллеги'
}

// Первая буква имени для кружка-аватара в плашке чужого чата.
export function initial(name?: string) {
  return (name?.trim()[0] ?? '?').toUpperCase()
}

// Открытый вопрос и прокрутка ленты — в sessionStorage вкладки (может быть недоступен — тогда просто не помним).
const openQuestionKey = 'rc-open-question'
type OpenQuestion = { id: number; scroll: number }

export function readOpenQuestion(): OpenQuestion | null {
  try {
    const value = JSON.parse(sessionStorage.getItem(openQuestionKey) ?? 'null') as Partial<OpenQuestion> | null
    return value && Number.isInteger(value.id) && value.id! > 0 ? { id: value.id!, scroll: Number(value.scroll) || 0 } : null
  } catch { return null }
}

export function saveOpenQuestion(value: OpenQuestion | null) {
  try {
    if (value) sessionStorage.setItem(openQuestionKey, JSON.stringify(value))
    else sessionStorage.removeItem(openQuestionKey)
  } catch { /* хранилище недоступно */ }
}
