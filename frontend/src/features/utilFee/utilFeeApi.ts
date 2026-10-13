import { request } from '@/shared/api/client'

// Калькулятор утильсбора — POST /api/util-fee (backend/internal/service/utilfee). Суммы считает сервер.

export type UtilFeeKind = 'product' | 'packaging' | 'vehicle'
export type UtilFeePackagingInput = { material: string; massKg: string }
export type UtilFeeInput = {
  code: string
  mrp: string
  massKg: string
  packaging: UtilFeePackagingInput[]
  vehicleCategory: string
  fromBelarusRussia: boolean
}
export type UtilFeeMatch = { section: number; item: string; prefix: string; title: string; kind: UtilFeeKind; k?: string; note?: string; vehicleGroup?: string }
export type UtilFeeMaterial = { id: string; label: string; k: string }
export type UtilFeeVehicleCategory = { id: string; group: string; label: string; coef1: string; coef2: string }
export type UtilFeeResult = {
  code: string
  edition: string
  matches: UtilFeeMatch[]
  possible: UtilFeeMatch[]
  materials: UtilFeeMaterial[]
  vehicleCategories: UtilFeeVehicleCategory[] | null
  product: { massKg: string; k: string; amount: string } | null
  packaging: { index: number; material: string; label: string; k: string; massKg: string; amount: string }[]
  vehicle: { category: string; label: string; baseMrp: string; coefficient: string; amount: string } | null
  total: string
}

export const utilFeeApi = {
  calculate: (input: UtilFeeInput) => request<UtilFeeResult>('/api/util-fee', { method: 'POST', body: JSON.stringify(input) }),
}
