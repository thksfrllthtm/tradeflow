import { request } from '@/shared/api/client'
import { AgentDocument, DocumentKind } from '@/features/goods/agent/agentApi'

// База документов (только администратор): что программа собрала по «Документам для ввоза» — ответы по
// кодам со всеми версиями, справки tnved.info и тексты актов, которые читали агенты.

export type DocumentsBaseSummary = { answers: number; codes: number; helps: number; acts: number }
export type DocumentsBaseItem = { code: string; product: string; savedAt: string; author: string; versions: number; yes: number; maybe: number; no: number; kinds: DocumentKind[] }
export type DocumentsBaseVersion = { id: number; product: string; description: string; documents: AgentDocument[]; sessionId: number | null; author: string; savedAt: string }
export type CachedPage = { url: string; title: string; fetchedAt: string | null; size: number; cached: boolean }
export type DocumentsBaseEntry = {
  code: string
  name: string
  versions: DocumentsBaseVersion[]
  help: { url: string; title: string; text: string; fetchedAt: string } | null
  acts: CachedPage[]
}

const query = (q: string) => q.trim() ? `?q=${encodeURIComponent(q.trim())}` : ''

export const documentsBaseApi = {
  list: (q: string) => request<{ summary: DocumentsBaseSummary; items: DocumentsBaseItem[] }>(`/api/tnved-agent/documents-base${query(q)}`),
  code: (code: string) => request<DocumentsBaseEntry>(`/api/tnved-agent/documents-base/codes/${code}`),
  sources: (q: string) => request<{ items: CachedPage[] }>(`/api/tnved-agent/documents-base/sources${query(q)}`),
}
