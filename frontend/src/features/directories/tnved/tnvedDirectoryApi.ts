import { request } from '@/shared/api/client'
import { directoryQuery, DirectoryListParams } from '../common/directoryQuery'
import { TNVEDDirectoryEntry, TNVEDDirectoryInput } from './types'

export const tnvedDirectoryApi = {
  list: async (params?: DirectoryListParams) => (await request<{ items: TNVEDDirectoryEntry[] }>(`/api/tnved-directory?${directoryQuery(params)}`)).items,
  create: async (input: TNVEDDirectoryInput) => (await request<{ item: TNVEDDirectoryEntry }>('/api/tnved-directory', { method: 'POST', body: JSON.stringify(input) })).item,
  update: async (id: number, input: TNVEDDirectoryInput) => (await request<{ item: TNVEDDirectoryEntry }>(`/api/tnved-directory/${id}`, { method: 'PATCH', body: JSON.stringify(input) })).item,
  archive: (id: number) => request<void>(`/api/tnved-directory/${id}/archive`, { method: 'POST' }),
  restore: (id: number) => request<void>(`/api/tnved-directory/${id}/restore`, { method: 'POST' }),
  copy: async (id: number, name?: string) => (await request<{ item: TNVEDDirectoryEntry }>(`/api/tnved-directory/${id}/copy`, { method: 'POST', body: JSON.stringify(name ? { name } : {}) })).item,
}
