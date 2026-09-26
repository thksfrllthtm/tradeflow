// Подбор наименования и цены керамики — типы ответа API /api/ceramics.

export type SizeUnit = 'inch' | 'cm' | 'mm' | 'ml' | 'l'
export type CeramicSize = { value: string; unit: SizeUnit }

// Запись базы: наименование, марка и цена. Тип, размер и китайскую часть сервер выделяет
// из наименования сам.
export type CeramicsProduct = {
  id: number
  mark: string
  name: string
  price: string
  productType: string
  sizeValue: string
  sizeUnit: SizeUnit | ''
  chineseName: string
  createdAt: string
  updatedAt: string
}

export type CeramicsType = { name: string; productCount: number }

export type CeramicsProductInput = { mark: string; name: string; price: string }

export type ResolutionStatus = 'FOUND_BY_MARK' | 'AUTO' | 'NEED_REVIEW' | 'NOT_DETERMINED'
export type PartStatus = 'MATCHED' | 'ASSUMED' | 'NEED_REVIEW' | 'NOT_FOUND' | 'NOT_NEEDED'
export type PriceStatus = 'MATCHED' | 'PRICE_NEED_REVIEW' | 'NOT_FOUND' | 'NOT_CHECKED'

export type PriceResult = {
  status: PriceStatus
  price?: string
  options: { price: string; count: number }[]
  total: number
  availableSizes: CeramicSize[]
  hasUnsized: boolean
  message: string
  // Русская часть наименования для этого типа и размера — как в базе пишутся названия этого типа.
  name: string
}

export type TypeCandidate = { productType: string; confidence: number; reason: string; sources?: string[] }
export type Evidence = { field: 'type' | 'size' | 'price'; source: string; text: string }

export type Resolution = {
  key: string
  mark: string
  markKey: string
  status: ResolutionStatus
  product?: CeramicsProduct
  productType: string
  typeStatus: PartStatus
  typeCandidates: TypeCandidate[]
  size: CeramicSize | null
  sizeStatus: PartStatus
  sizeOptions: { size: CeramicSize; source: string; detail: string }[]
  price: PriceResult
  displayName: string
  chineseTerms: { text: string; meaning: string }[]
  evidence: Evidence[]
  warnings: string[]
  aiUsed: boolean
}

export type AnalyzeItem = { key: string; mark: string; chineseName: string; russianName: string; photo: string }

export type AIStatus = { configured: boolean; vision: boolean }
