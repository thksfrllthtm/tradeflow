import { request } from '@/shared/api/client'
import { NomenclatureCode } from '@/shared/tnved/nomenclature'
import { CodeDecision } from '@/shared/tnved/decisions'

// ИИ-агент подбора кода ТН ВЭД для строки сметы: подбор идёт на сервере, браузер забирает
// новые сообщения чата опросом.

export type AgentStatus = { configured: boolean; model: string; vision: boolean; dailyLimit: number; usedToday: number }

// Расход агента за 30 дней: всего и отдельно модель для фото (vision) и модель рассуждений (reasoning).
type AgentUsageTotals = { promptTokens: number; cachedTokens: number; completionTokens: number; costUsd: string }
type AgentUsagePart = AgentUsageTotals & { model: string }
// searching — поиск в базе решений, ready — поиск закончен, агента запускает сотрудник.
export type AgentSessionStatus = 'searching' | 'ready' | 'running' | 'waiting' | 'done' | 'error' | 'cancelled'

export type AgentItem = {
  name: string
  supplierName: string
  otherNames?: string[]
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
  // page — страница кода (TWS.BY), help — справка tnved.info или документ по ссылке, name — наименование кода.
  quoteSource: '' | 'page' | 'help' | 'name'
  reason: string
  check: string
  pageOpened: boolean
  nomenclature: NomenclatureCode | null
  // Описания из деклараций под кодом (ifcg.ru) по словам агента; count — записей в его поиске (нет — код
  // в найденном не встретился). Нет поля — агент декларации не искал.
  declarations?: { url: string; query: string; count?: number }
}

export type AgentResult = {
  description: string
  candidates: AgentCandidate[]
  name: string
  // Подробный вариант наименования (только в справочнике на главной; name там — краткий).
  nameFull?: string
  notes: string
  droppedCodes: string[]
  quotesRemoved: number
  // Уверенность в лучшем коде и страницы из интернета, на которые опирался агент.
  confidence?: 'high' | 'medium' | 'low'
  confidenceReason?: string
  sources?: { title: string; url: string }[]
  // Код из файла поставщика, если подобран другой: пошлины обоих (считает сервер).
  fileCode?: { code: string; found: boolean; duty: string; bestCode: string; bestDuty: string; dutyDiffers: boolean }
  // Ответ по кнопке «Документы для ввоза»: для какого кода, меры и сколько чужих ссылок убрала программа.
  documentsCode?: string
  documents?: AgentDocument[]
  linksRemoved?: number
}

// Мера или документ для ввоза: applies — относится ли мера к товару по пояснению под актом.
// basis — дословная цитата из справки, акта или страницы кода, на которой основан вывод (проверена сервером);
// nameHint — при каком реальном признаке товара мера не применяется и какое наименование тогда указать.
// kind — вид документа: cc — сертификат соответствия, dc — декларация соответствия, sgr — СГР, conditional — условный выпуск.
export type DocumentKind = 'cc' | 'dc' | 'sgr' | 'conditional' | 'other'
export type AgentDocument = { kind?: DocumentKind; measure: string; act: string; url: string; requirement: string; applies: 'yes' | 'maybe' | 'no'; basis?: string; basisSource?: string; nameHint?: string }

// Последний сохранённый ответ «Документы для ввоза» по коду: product — для какого товара он был получен.
export type SavedDocuments = AgentResult & { product: string; savedAt: string }

// photoDiffers — совпало наименование, но фото другое: фото главнее, это может быть другой товар.
export type SameProductMatch = { decision: CodeDecision; byPhoto: boolean; byName: boolean; photoDiffers?: boolean }
// Похожий по фото товар из базы решений: похожесть 0–100, по снимку или по описанию фото, визуальная
// сверка ИИ (same — тот же товар, similar — похожий, '' — не сверялся).
export type SimilarMatch = { decision: CodeDecision; score: number; byPhoto: boolean; byDescription: boolean; description: string; verdict: '' | 'same' | 'similar'; why: string }

export type AgentMessage =
  | { id: number; kind: 'input'; content: { item: AgentItem; photos: number; ai: boolean }; createdAt: string }
  | { id: number; kind: 'text' | 'question' | 'user' | 'notice'; content: { text: string }; createdAt: string }
  | { id: number; kind: 'tool'; content: { tool: string; label: string; detail: string; url?: string; failed?: boolean }; createdAt: string }
  | { id: number; kind: 'result'; content: AgentResult; createdAt: string }
  | { id: number; kind: 'match'; content: { matches: SameProductMatch[]; skipped: boolean }; createdAt: string }
  | { id: number; kind: 'similar'; content: { matches: SimilarMatch[]; description: string }; createdAt: string }
  | { id: number; kind: 'error'; content: { message: string }; createdAt: string }
  | { id: number; kind: 'rerun' | 'start'; content: Record<string, never>; createdAt: string }
  | { id: number; kind: 'documents'; content: { code: string }; createdAt: string }

export type AgentSession = {
  id: number
  // row — подбор для строки сметы, reference — чат-справочник на главной (title — первый вопрос).
  mode?: 'row' | 'reference'
  title?: string
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

// Вопрос чата-справочника в истории сотрудника.
export type ReferenceSummary = { id: number; title: string; status: AgentSessionStatus; bestCode: string; hasPhoto: boolean; createdAt: string; updatedAt: string }

export type AgentRowStatus = { sessionId: number; itemId: string; status: AgentSessionStatus; bestCode: string; verdict: string }

export const agentApi = {
  status: () => request<AgentStatus>('/api/tnved-agent/status'),
  usage: () => request<AgentUsageTotals & { sessions: number; runs: number; vision: AgentUsagePart; reasoning: AgentUsagePart }>('/api/tnved-agent/usage'),
  rows: (estimateId: number) => request<{ items: AgentRowStatus[] }>(`/api/tnved-agent/sessions?estimateId=${estimateId}`),
  // auto — сразу запустить агента после поиска в базе (подбор для всех строк).
  start: (input: { estimateId: number; itemId: string; item: AgentItem; photos: string[]; auto?: boolean }) => request<AgentSession>('/api/tnved-agent/sessions', { method: 'POST', body: JSON.stringify(input) }),
  begin: (id: number) => request<AgentSession>(`/api/tnved-agent/sessions/${id}/begin`, { method: 'POST' }),
  get: (id: number, after = 0) => request<AgentSession>(`/api/tnved-agent/sessions/${id}?after=${after}`),
  message: (id: number, text: string) => request<AgentSession>(`/api/tnved-agent/sessions/${id}/messages`, { method: 'POST', body: JSON.stringify({ text }) }),
  // «Документы для ввоза» для кода из ответа агента (ответ для того же кода повторно не запрашивается).
  documents: (id: number, code: string) => request<AgentSession>(`/api/tnved-agent/sessions/${id}/documents`, { method: 'POST', body: JSON.stringify({ code }) }),
  savedDocuments: (code: string) => request<SavedDocuments>(`/api/tnved-agent/documents/${code}`),
  rerun: (id: number) => request<AgentSession>(`/api/tnved-agent/sessions/${id}/rerun`, { method: 'POST' }),
  cancel: (id: number) => request<AgentSession>(`/api/tnved-agent/sessions/${id}/cancel`, { method: 'POST' }),
  // Наименование: перевод русской части на китайский — только по «Применить наименование».
  name: (id: number, name: string) => request<{ name: string }>(`/api/tnved-agent/sessions/${id}/name`, { method: 'POST', body: JSON.stringify({ name }) }),
  // Чат-справочник: вопрос о товаре (текст и/или фото), история и удаление вопроса.
  ask: (input: { text: string; photos: string[] }) => request<AgentSession>('/api/tnved-agent/reference', { method: 'POST', body: JSON.stringify(input) }),
  history: () => request<{ items: ReferenceSummary[] }>('/api/tnved-agent/reference'),
  removeQuestion: (id: number) => request<void>(`/api/tnved-agent/reference/${id}`, { method: 'DELETE' }),
  apply: (id: number, code: string, name: string) => request<CodeDecision>(`/api/tnved-agent/sessions/${id}/apply`, { method: 'POST', body: JSON.stringify({ code, name }) }),
}

