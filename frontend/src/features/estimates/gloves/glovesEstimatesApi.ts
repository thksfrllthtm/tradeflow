import { request } from '@/shared/api/client'
import { GlovesDraft, GlovesEstimate, GlovesEstimateInput, GlovesEstimateSummary } from '../shared/types'

// Сметы перчаток: отдельный список смет и черновиков (/api/gloves/estimates).
export const glovesEstimatesApi = {
  list: async () => (await request<{ estimates: GlovesEstimateSummary[] }>('/api/gloves/estimates')).estimates,
  create: async (input: GlovesEstimateInput) => (await request<{ estimate: GlovesEstimateSummary }>('/api/gloves/estimates', { method: 'POST', body: JSON.stringify(input) })).estimate,
  get: async (id: number) => (await request<{ estimate: GlovesEstimate }>(`/api/gloves/estimates/${id}`)).estimate,
  update: async (id: number, input: GlovesEstimateInput) => (await request<{ estimate: GlovesEstimateSummary }>(`/api/gloves/estimates/${id}`, { method: 'PATCH', body: JSON.stringify(input) })).estimate,
  remove: (id: number) => request<void>(`/api/gloves/estimates/${id}`, { method: 'DELETE' }),
  saveDraft: async (id: number, draft: Pick<GlovesDraft, 'input' | 'result'>) => (await request<{ estimate: GlovesEstimate }>(`/api/gloves/estimates/${id}/draft`, { method: 'PUT', body: JSON.stringify(draft) })).estimate,
}
