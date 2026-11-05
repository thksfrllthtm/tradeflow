import { NomenclatureCode } from './nomenclature'

// Запись базы решений «товар из инвойса → код ТН ВЭД» и подписи её статусов.

export type DecisionStatus = 'pending' | 'verified' | 'rejected'
export type RejectedCode = { code: string; why: string }

export type CodeDecision = {
  id: number
  status: DecisionStatus
  source: 'agent' | 'manual' | 'import'
  invoiceName: string
  supplierName: string
  finalName: string
  code: string
  reason: string
  quote: string
  sourceUrl: string
  rejectedCodes: RejectedCode[]
  unitNetKg: string
  unitPriceUsd: string
  photos: string[]
  thumb: string
  agentCode: string
  agentCorrect: boolean | null
  estimateId?: number
  itemId?: string
  sessionId?: number
  note: string
  createdByName: string
  verifiedByName: string
  verifiedAt: string | null
  createdAt: string
  updatedAt: string
  nomenclature: NomenclatureCode | null
  // Описание фото от ИИ одной строкой; пусто — ещё не составлено.
  photoDescription: string
  // То же по полям — для карточки записи.
  photoFeatures?: PhotoFeatures
}

export type PhotoFeatures = {
  kind: string
  synonyms?: string[]
  purpose?: string
  material: string[]
  shape: string[]
  color: string[]
  features: string[]
  keywords?: string[]
}

export const decisionStatusLabels: Record<DecisionStatus, string> = { pending: 'Не проверено', verified: 'Проверено', rejected: 'Отклонено' }
// Тон точки-статуса (.status-dot) и бейджа (.badge) из base.css.
export const decisionStatusTones: Record<DecisionStatus, string> = { pending: 'warn', verified: 'ok', rejected: '' }
