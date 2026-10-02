import { request } from '@/shared/api/client'
import { CodeDecision } from '@/shared/tnved/decisions'

// База решений «товар из инвойса → код ТН ВЭД»: проверенные примеры с фото и обоснованием.

export type DecisionStats = { pending: number; verified: number; rejected: number; agentChecked: number; agentCorrect: number }

export type DecisionInput = {
  invoiceName: string
  supplierName?: string
  finalName?: string
  code: string
  reason?: string
  quote?: string
  sourceUrl?: string
  unitNetKg?: string
  unitPriceUsd?: string
  photos?: string[]
  thumb?: string
  estimateId?: number
  itemId?: string
  note?: string
}

type DecisionEdit = Partial<{ code: string; finalName: string; invoiceName: string; supplierName: string; reason: string; note: string }>


export const decisionSourceLabels: Record<CodeDecision['source'], string> = { agent: 'подбор ИИ', manual: 'вручную', import: 'импорт' }

export const decisionsApi = {
  list: (params: { status?: string; q?: string; agentWrong?: boolean; offset?: number; limit?: number }) => {
    const query = new URLSearchParams()
    if (params.status) query.set('status', params.status)
    if (params.q) query.set('q', params.q)
    if (params.agentWrong) query.set('agentWrong', 'true')
    query.set('offset', String(params.offset ?? 0))
    query.set('limit', String(params.limit ?? 50))
    return request<{ items: CodeDecision[]; total: number; stats: DecisionStats }>(`/api/code-decisions?${query}`)
  },
  stats: () => request<DecisionStats>('/api/code-decisions/stats'),
  get: (id: number) => request<CodeDecision>(`/api/code-decisions/${id}`),
  create: (input: DecisionInput) => request<CodeDecision>('/api/code-decisions', { method: 'POST', body: JSON.stringify(input) }),
  update: (id: number, edit: DecisionEdit) => request<CodeDecision>(`/api/code-decisions/${id}`, { method: 'PATCH', body: JSON.stringify(edit) }),
  verify: (id: number, edit: DecisionEdit = {}) => request<CodeDecision>(`/api/code-decisions/${id}/verify`, { method: 'POST', body: JSON.stringify(edit) }),
  reject: (id: number, note = '') => request<CodeDecision>(`/api/code-decisions/${id}/reject`, { method: 'POST', body: JSON.stringify({ note }) }),
  remove: (id: number) => request<void>(`/api/code-decisions/${id}`, { method: 'DELETE' }),
  importRows: (rows: DecisionInput[]) => request<{ created: number; skipped: number; errors: { row: number; message: string }[] }>('/api/code-decisions/import', { method: 'POST', body: JSON.stringify({ rows }) }),
}
