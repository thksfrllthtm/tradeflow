import { request } from '@/shared/api/client'
import { NomenclatureCode } from '@/shared/tnved/nomenclature'
import { CodeDecision } from '@/shared/tnved/decisions'

// ИИ-агент подбора кода ТН ВЭД для строки сметы: подбор идёт на сервере, браузер забирает
// новые сообщения чата опросом.

export type AgentStatus = { configured: boolean; model: string; vision: boolean; dailyLimit: number; usedToday: number }
export type AgentSessionStatus = 'running' | 'waiting' | 'done' | 'error' | 'cancelled'

export type AgentItem = {
  name: string
  supplierName: string
  code: string
  quantity: string
  places: string
  grossKg: string
  netKg: string
  pricePerUnit: string
  unit: string
}

export type AgentCandidate = {
  code: string
  verdict: 'fits' | 'doubtful' | 'no'
  quote: string
  quoteSource: '' | 'page' | 'name'
  reason: string
  check: string
  pageOpened: boolean
  nomenclature: NomenclatureCode | null
}

export type AgentResult = {
  description: string
  candidates: AgentCandidate[]
  name: string
  notes: string
  droppedCodes: string[]
  quotesRemoved: number
}

export type SameProductMatch = { decision: CodeDecision; byPhoto: boolean; byName: boolean }

export type AgentMessage =
  | { id: number; kind: 'input'; content: { item: AgentItem; photos: number; ai: boolean }; createdAt: string }
  | { id: number; kind: 'text' | 'question' | 'user' | 'notice'; content: { text: string }; createdAt: string }
  | { id: number; kind: 'tool'; content: { tool: string; label: string; detail: string; url?: string; failed?: boolean }; createdAt: string }
  | { id: number; kind: 'result'; content: AgentResult; createdAt: string }
  | { id: number; kind: 'match'; content: { matches: SameProductMatch[]; skipped: boolean }; createdAt: string }
  | { id: number; kind: 'error'; content: { message: string }; createdAt: string }
  | { id: number; kind: 'rerun'; content: Record<string, never>; createdAt: string }

export type AgentSession = {
  id: number
  estimateId: number | null
  itemId: string
  status: AgentSessionStatus
  error: string
  item: AgentItem
  photos?: string[]
  result: AgentResult | null
  messages: AgentMessage[]
  steps: number
  runs: number
  costUsd: string
  tokens: number
  model: string
  createdAt: string
  updatedAt: string
}

export type AgentRowStatus = { sessionId: number; itemId: string; status: AgentSessionStatus; bestCode: string; verdict: string }

export const agentApi = {
  status: () => request<AgentStatus>('/api/tnved-agent/status'),
  usage: () => request<{ sessions: number; runs: number; promptTokens: number; cachedTokens: number; completionTokens: number; costUsd: string }>('/api/tnved-agent/usage'),
  rows: (estimateId: number) => request<{ items: AgentRowStatus[] }>(`/api/tnved-agent/sessions?estimateId=${estimateId}`),
  start: (input: { estimateId: number; itemId: string; item: AgentItem; photos: string[] }) => request<AgentSession>('/api/tnved-agent/sessions', { method: 'POST', body: JSON.stringify(input) }),
  get: (id: number, after = 0) => request<AgentSession>(`/api/tnved-agent/sessions/${id}?after=${after}`),
  message: (id: number, text: string) => request<AgentSession>(`/api/tnved-agent/sessions/${id}/messages`, { method: 'POST', body: JSON.stringify({ text }) }),
  rerun: (id: number) => request<AgentSession>(`/api/tnved-agent/sessions/${id}/rerun`, { method: 'POST' }),
  cancel: (id: number) => request<AgentSession>(`/api/tnved-agent/sessions/${id}/cancel`, { method: 'POST' }),
  apply: (id: number, code: string, name: string) => request<CodeDecision>(`/api/tnved-agent/sessions/${id}/apply`, { method: 'POST', body: JSON.stringify({ code, name }) }),
}

