import { request } from '@/shared/api/client'
import { AgentResult, AgentSession } from '@/features/goods/agent/agentApi'

// Журнал агента ТН ВЭД (только администратор): подборы всех сотрудников, рядом итог теневой модели и
// запись базы решений — согласны ли модели и кто оказался прав после проверки.

export type LogModel = { status: string; model: string; error?: string; bestCode: string; costUsd: string; steps: number; seconds: number | null }
export type LogDecision = { id: number; code: string; status: 'pending' | 'verified' | 'rejected'; agentCorrect: boolean | null }
export type LogRow = {
  id: number
  userId: number
  user: string
  estimateId: number | null
  itemId: string
  mode: 'row' | 'reference'
  name: string
  createdAt: string
  runs: number
  main: LogModel
  shadow: LogModel | null
  decision: LogDecision | null
  // agree — у обеих моделей есть код и он один; правота — по проверенной записи (null — не с чем сравнить);
  // documentsAgree — сошлись ли модели по «Документам для ввоза» (null — не сравнивалось).
  agree: boolean | null
  mainCorrect: boolean | null
  shadowCorrect: boolean | null
  documentsAgree: boolean | null
}
export type LogSummary = {
  total: number; withResult: number; shadowResults: number; agree: number; disagree: number
  verified: number; mainVerified: number; mainCorrect: number; shadowVerified: number; shadowCorrect: number
  documentsCompared: number; documentsAgree: number
  costUsd: string; shadowCostUsd: string; seconds: number; shadowSeconds: number
}
export type ShadowStatus = { enabled: boolean; model: string; vision: boolean; budgetUsd: string; spentUsd: string; dailyLimit: number; today: number }
export type LogPage = { rows: LogRow[]; summary: LogSummary; shadow: ShadowStatus }

// Сообщение переписки с моделью (OpenAI-формат): текст или части (текст, ссылка на фото), вызовы
// инструментов, ответ инструмента, размышления.
export type TranscriptPart = { type: 'text'; text: string } | { type: 'image_ref'; index: number } | { type: 'image_url' }
export type TranscriptMessage = {
  role: 'system' | 'user' | 'assistant' | 'tool'
  content: string | TranscriptPart[] | null
  reasoning_content?: string
  tool_calls?: { id: string; function: { name: string; arguments: string } }[]
  tool_call_id?: string
}
export type ShadowMessage = { kind: string; content: unknown; createdAt: string }
export type ShadowRun = {
  status: string; model: string; error?: string; result: AgentResult | null; messages: ShadowMessage[]; transcript: TranscriptMessage[]
  steps: number; costUsd: string; tokens: number; startedAt: string; finishedAt: string | null
}
export type LogEntry = { row: LogRow; session: AgentSession; transcript: TranscriptMessage[]; shadow: ShadowRun | null }

export type LogFilter = { from: string; to: string; userId: string; mode: string; only: '' | 'disagree' | 'verified' | 'wrong' | 'shadow' | 'documents' }

export const agentLogApi = {
  list: (filter: LogFilter, page: number, limit: number) => {
    const params = new URLSearchParams({ page: String(page), limit: String(limit) })
    for (const [key, value] of Object.entries(filter)) if (value) params.set(key, value)
    return request<LogPage>(`/api/tnved-agent/log?${params}`)
  },
  entry: (id: number) => request<LogEntry>(`/api/tnved-agent/log/${id}`),
}
