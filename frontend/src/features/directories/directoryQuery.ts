// Параметры поиска общие для обоих справочников (ТН ВЭД и товаров).
export type DirectoryListParams = { q?: string; category?: string; status?: string; includeArchived?: boolean }

export function directoryQuery(params?: DirectoryListParams) {
  const query = new URLSearchParams()
  if (params?.q) query.set('q', params.q)
  if (params?.category) query.set('category', params.category)
  if (params?.status) query.set('status', params.status)
  if (params?.includeArchived) query.set('includeArchived', 'true')
  return query
}
