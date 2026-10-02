import { request } from '@/shared/api/client'
import { directoryQuery, DirectoryListParams } from '../common/directoryQuery'
import { ProductDirectoryEntry, ProductDirectoryInput } from './types'

export const productDirectoryApi = {
  list: async (params?: DirectoryListParams) => (await request<{ items: ProductDirectoryEntry[] }>(`/api/product-directory?${directoryQuery(params)}`)).items,
  create: async (input: ProductDirectoryInput) => (await request<{ item: ProductDirectoryEntry }>('/api/product-directory', { method: 'POST', body: JSON.stringify(input) })).item,
  update: async (id: number, input: ProductDirectoryInput) => (await request<{ item: ProductDirectoryEntry }>(`/api/product-directory/${id}`, { method: 'PATCH', body: JSON.stringify(input) })).item,
  archive: (id: number) => request<void>(`/api/product-directory/${id}/archive`, { method: 'POST' }),
  restore: (id: number) => request<void>(`/api/product-directory/${id}/restore`, { method: 'POST' }),
  copy: async (id: number) => (await request<{ item: ProductDirectoryEntry }>(`/api/product-directory/${id}/copy`, { method: 'POST' })).item,
}
