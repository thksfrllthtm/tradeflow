import { useCallback, useEffect, useState } from 'react'
import { AgentRowStatus, agentApi } from './agentApi'

const pollInterval = 3000

// Состояние подбора ИИ по строкам сметы (для значков в таблице). Пока где-то идёт подбор —
// обновляется каждые 3 с. Неизменённые строки сохраняют тот же объект, чтобы memo-строки
// таблицы не перерисовывались.
export function useAgentRows(estimateId: number, enabled: boolean) {
  const [statuses, setStatuses] = useState<Record<string, AgentRowStatus>>({})
  const [tick, setTick] = useState(0)
  const refresh = useCallback(() => setTick((value) => value + 1), [])

  useEffect(() => {
    if (!enabled) return
    let active = true
    agentApi.rows(estimateId).then((response) => {
      if (!active) return
      setStatuses((current) => {
        const next: Record<string, AgentRowStatus> = {}
        let changed = Object.keys(current).length !== response.items.length
        for (const row of response.items) {
          const previous = current[row.itemId]
          const same = previous && previous.sessionId === row.sessionId && previous.status === row.status && previous.bestCode === row.bestCode && previous.verdict === row.verdict
          next[row.itemId] = same ? previous : row
          if (!same) changed = true
        }
        return changed ? next : current
      })
    }).catch(() => undefined)
    return () => { active = false }
  }, [estimateId, enabled, tick])

  const anyRunning = Object.values(statuses).some((row) => row.status === 'running')
  useEffect(() => {
    if (!enabled || !anyRunning) return
    const timer = window.setInterval(refresh, pollInterval)
    return () => window.clearInterval(timer)
  }, [enabled, anyRunning, refresh])

  return { statuses, refresh }
}
