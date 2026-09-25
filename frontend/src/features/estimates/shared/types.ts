// Типы смет товаров: строки, параметры, результат
// расчёта и черновик. Совпадают с JSON-контрактом backend (/api/gloves/*).
export type SheetInfo = { name: string; rows: number }

type CalculationItem = {
  productId: number; directoryEntryId?: number; directorySnapshot?: Record<string, unknown>; standardName: string; supplierName: string; photoUrl: string; imageAssetIds?: number[]; code: string
  pairs: string; places: string; pairsPerPlace: string; grossKg: string; grossWeightUnit?: string; netKg: string; netWeightUnit?: string; netSource: string; packageWeightKg: string; coefficient: string
  pricePerPair: string; costInput: string; costCurrency: string; extraExpenses: string; customsValueDirectKzt: string
  deliveryUsd: string; deliveryManual: boolean; deliveryAlreadyInCost: boolean; deliveryIncluded: boolean; excluded: boolean; exclusionReason: string
  costUsd: string; pairPriceUsd: string; goodsCost: string; goodsCostCurrency: string; pairPrice: string; customsValueKzt: string; dutyKzt: string; antiDumpingKzt: string; vatKzt: string; vatBaseKzt: string; feeKzt: string; deliveryKzt: string; status: string; errors: string[]; warnings: string[]
}
type CalculationTotals = { totalCostUsd: string; totalDeliveryUsd: string; deliveryKzt: string; deliveryInCustomsKzt: string; deliveryInExpensesKzt: string; customsValueKzt: string; dutyKzt: string; antiDumpingKzt: string; vatKzt: string; feeKzt: string; customsPaymentsKzt: string; expensesKzt: string; expensesUsd: string; cashBalanceUsd: string; calculatedCount: number; totalCount: number; needsReview: boolean; errorCount: number; warningCount: number; blockingErrors: string[] }
export type GlovesCalculationResult = { assessmentDate: string; origin: string; usdKzt: string; eurKzt: string; wagons: string; wagonTariffUsd: string; deliveryMode: '' | 'none' | 'wagons' | 'total'; deliveryTotalUsd: string; packageWeightKg: string; customsFeeKzt: string; mrp: string; feeMultiplier: string; feeIncluded: boolean; cashUsd: string; deliveryIncludedDefault?: boolean; items: CalculationItem[]; totals: CalculationTotals; updatedAt: string; formulaVersion: string; roundingPolicy: string; calculatedAt?: string }

export type GlovesParseRow = { sourceRow: number; values: Record<string, string>; included: boolean; warnings?: string[] }
export type GlovesParseResult = { sheets: SheetInfo[]; headers: string[]; rows: GlovesParseRow[]; images: { sourceRow: number; mimeType: string; data: string }[]; warnings?: string[] }
export type GlovesSourceSnapshot = { filename: string; signature: string; sheet: string; headerRow: number; dataStartRow: number; headers: string[]; rows: GlovesParseRow[]; mapping: Record<string, string> }
export type GlovesItemInput = {
  id?: string; productId?: number; sourceRow?: number; directoryEntryId?: number; productDirectoryId?: number; directorySnapshot?: Record<string, unknown>
  name: string; supplierName: string; standardName: string; photoUrl: string; photoUrls?: string[]; code: string
  pairs: string; places: string; pairsPerPlace: string; grossKg: string; grossWeightUnit?: string; netKg: string; netWeightUnit?: string; netSource: string; packageWeightKg: string; coefficient: string
  pricePerPair: string; sourcePricePerPair?: string; costInput: string; costCurrency: string; extraExpenses: string; customsValueDirectKzt: string
  deliveryUsd: string; deliveryManual: boolean; deliveryAlreadyInCost: boolean; deliveryIncluded: boolean; excluded: boolean; exclusionReason: string
}
export type GlovesCalculationInput = { assessmentDate: string; origin: string; usdKzt: string; eurKzt: string; deliveryMode: '' | 'none' | 'wagons' | 'total'; wagons: string; wagonTariffUsd: string; deliveryTotalUsd: string; packageWeightKg: string; customsFeeKzt: string; mrp: string; feeMultiplier: string; cashUsd: string; items: GlovesItemInput[]; source?: GlovesSourceSnapshot; directoryCoefficientOnly?: boolean; genericItems?: boolean }
// Параметры расчёта без строк — то, что редактируется в панели «Параметры расчёта».
export type GlovesParams = Omit<GlovesCalculationInput, 'items'>

export type GlovesDraft = { input: GlovesCalculationInput; result?: GlovesCalculationResult; updatedAt: string }
export type GlovesEstimateInput = { name: string; client: string; assessmentDate: string }
export type GlovesEstimateSummary = { id: number; name: string; client: string; assessmentDate: string; status: string; createdAt: string; updatedAt: string; ownerName?: string }
export type GlovesEstimate = GlovesEstimateSummary & { input: GlovesCalculationInput; result?: GlovesCalculationResult }
