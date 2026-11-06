import { useState } from 'react'
import { GoodsItemInput } from '@/shared/types/goods'
import { requestErrorMessage } from '../lib/requestErrorMessage'
import { AgentRowStatus, agentApi } from './agentApi'
import { agentItemFrom, agentPhotosFrom } from './agentItem'

type AgentRows = { statuses: Record<string, AgentRowStatus>; refresh: () => void }

// Подбор ИИ для всех включённых строк без кода, где подбора ещё не было: запуски идут на
// сервере в фоне, значки в таблице обновляются сами.
export function useAgentBatch(estimateId: number, items: GoodsItemInput[], agentRows: AgentRows, setError: (message: string) => void) {
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null)

  async function start() {
    // Строки, где уже был поиск в базе, но агента не запускали, тоже входят: агент запускается сразу.
    const rows = items.filter((item) => item.id && !item.excluded && !item.code && (!agentRows.statuses[item.id] || agentRows.statuses[item.id].status === 'ready'))
    if (rows.length === 0) return
    if (!window.confirm(`Подобрать коды для ${rows.length} строк без кода? Подбор идёт на сервере, результат — в чате каждой строки (кнопка «Подбор»).`)) return
    setProgress({ done: 0, total: rows.length })
    setError('')
    try {
      for (const [index, item] of rows.entries()) {
        const ready = agentRows.statuses[item.id ?? '']
        if (ready?.status === 'ready') await agentApi.begin(ready.sessionId)
        else await agentApi.start({ estimateId, itemId: item.id ?? '', item: agentItemFrom(item), photos: await agentPhotosFrom(item), auto: true })
        setProgress({ done: index + 1, total: rows.length })
        agentRows.refresh()
      }
    } catch (reason) {
      setError(requestErrorMessage(reason))
    } finally {
      setProgress(null)
      agentRows.refresh()
    }
  }

  return { progress, start }
}
