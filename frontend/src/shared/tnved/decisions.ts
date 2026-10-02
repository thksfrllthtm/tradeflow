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
}

export const decisionStatusLabels: Record<DecisionStatus, string> = { pending: 'Не проверено', verified: 'Проверено', rejected: 'Отклонено' }
