// Типы смет товаров: строки, параметры, результат
// расчёта и черновик. Совпадают с JSON-контрактом backend (/api/goods/*).
export type SheetInfo = { name: string; rows: number }

type CalculationItem = {
  productId: number; directoryEntryId?: number; directorySnapshot?: Record<string, unknown>; standardName: string; supplierName: string; photoUrl: string; imageAssetIds?: number[]; code: string
  pairs: string; places: string; pairsPerPlace: string; grossKg: string; grossWeightUnit?: string; netKg: string; netWeightUnit?: string; netSource: string; packageWeightKg: string; coefficient: string
  pricePerPair: string; costInput: string; costCurrency: string; extraExpenses: string; customsValueDirectKzt: string
  deliveryUsd: string; deliveryManual: boolean; deliveryAlreadyInCost: boolean; deliveryIncluded: boolean; excluded: boolean; exclusionReason: string
  costUsd: string; pairPriceUsd: string; goodsCost: string; goodsCostCurrency: string; pairPrice: string; customsValueKzt: string; dutyKzt: string; antiDumpingKzt: string; vatKzt: string; vatBaseKzt: string; feeKzt: string; deliveryKzt: string; status: string; errors: string[]; warnings: string[]
  // ТС по СИР: customsBasis = 'sir' и sir — как посчитано; без них — по инвойсу.
  customsBasis?: 'sir'; sir?: SirResult
}
// Как посчитана ТС строки по стоимостному индикатору риска (СИР).
export type SirResult = { profile: string; country: string; priceUsd: string; discountPercent: string; pricePerKgUsd: string; weight: SirWeight; weightKg: string; valueUsd: string }
export type CustomsValueMode = 'invoice' | 'sir'
export type SirWeight = 'net' | 'gross'
// Параметры СИР сметы: ТС по инвойсу или по СИР, вес, снижение индикатора, страна (по умолчанию CN).
export type SirParams = { customsValueMode?: CustomsValueMode; sirWeight?: SirWeight; sirDiscountPercent?: string; sirCountry?: string }
type CalculationTotals = { totalCostUsd: string; totalDeliveryUsd: string; deliveryKzt: string; deliveryInCustomsKzt: string; deliveryInExpensesKzt: string; customsValueKzt: string; dutyKzt: string; antiDumpingKzt: string; vatKzt: string; feeKzt: string; customsPaymentsKzt: string; expensesKzt: string; expensesUsd: string; cashBalanceUsd: string; calculatedCount: number; totalCount: number; needsReview: boolean; errorCount: number; warningCount: number; blockingErrors: string[] }
export type GoodsCalculationResult = { assessmentDate: string; origin: string; usdKzt: string; eurKzt: string; wagons: string; wagonTariffUsd: string; deliveryMode: '' | 'none' | 'wagons' | 'total'; deliveryTotalUsd: string; packageWeightKg: string; customsFeeKzt: string; mrp: string; feeMultiplier: string; feeIncluded: boolean; cashUsd: string; deliveryIncludedDefault?: boolean; items: CalculationItem[]; totals: CalculationTotals; updatedAt: string; formulaVersion: string; roundingPolicy: string; calculatedAt?: string } & SirParams

export type GoodsParseRow = { sourceRow: number; values: Record<string, string>; included: boolean; warnings?: string[] }
export type GoodsParseResult = { sheets: SheetInfo[]; headers: string[]; rows: GoodsParseRow[]; images: { sourceRow: number; mimeType: string; data: string }[]; warnings?: string[] }
export type GoodsSourceSnapshot = { filename: string; signature: string; sheet: string; headerRow: number; dataStartRow: number; headers: string[]; rows: GoodsParseRow[]; mapping: Record<string, string> }
export type GoodsItemInput = {
  id?: string; productId?: number; sourceRow?: number; directoryEntryId?: number; productDirectoryId?: number; directorySnapshot?: Record<string, unknown>
  name: string; supplierName: string; standardName: string; photoUrl: string; photoUrls?: string[]; code: string
  // Другие колонки наименования из файла («Наименование 2/3»).
  otherNames?: string[]
  // Вариант антидемпинга, выбранный сотрудником на этапе результата ('none' — не начислять).
  antiDumpingChoice?: string
  pairs: string; places: string; pairsPerPlace: string; grossKg: string; grossWeightUnit?: string; netKg: string; netWeightUnit?: string; netSource: string; packageWeightKg: string; coefficient: string
  pricePerPair: string; sourcePricePerPair?: string; costInput: string; costCurrency: string; extraExpenses: string; customsValueDirectKzt: string
  deliveryUsd: string; deliveryManual: boolean; deliveryAlreadyInCost: boolean; deliveryIncluded: boolean; excluded: boolean; exclusionReason: string
}
export type GoodsCalculationInput = { assessmentDate: string; origin: string; usdKzt: string; eurKzt: string; deliveryMode: '' | 'none' | 'wagons' | 'total'; wagons: string; wagonTariffUsd: string; deliveryTotalUsd: string; packageWeightKg: string; customsFeeKzt: string; mrp: string; feeMultiplier: string; cashUsd: string; items: GoodsItemInput[]; source?: GoodsSourceSnapshot; directoryCoefficientOnly?: boolean; genericItems?: boolean; resultTools?: GoodsResultTools } & SirParams
// Инструменты этапа «Результат», сохраняемые со сметой: подбор цен под бюджет (бюджет, разница,
// итог, сообщения, пояснения к ценам «По товарам» по номеру строки) и сверка по ИТП. Расчёт их не читает.
export type BudgetSummary = { status: 'ok' | 'closest'; budget: number; total: number; gap: number; fixedRows: number; evaluations: number }
export type BudgetExplanation = { code: string; names: string; price: string; reason: string; rows: number[] }
export type BudgetSaved = { amount: string; gapMin: string; gapMax: string; gapMode: 'under' | 'over'; summary?: BudgetSummary; warning?: string; notice?: string; notes?: Record<string, string>; explanations?: BudgetExplanation[] }
export type ItpSaved = { value: string; netKg: string; rate: string }
export type GoodsResultTools = { budget?: BudgetSaved; itp?: ItpSaved }
// Параметры расчёта без строк — то, что редактируется в панели «Параметры расчёта».
export type GoodsParams = Omit<GoodsCalculationInput, 'items'>

export type GoodsEstimateInput = { name: string; client: string; assessmentDate: string }
export type GoodsEstimateSummary = { id: number; name: string; client: string; assessmentDate: string; status: string; createdAt: string; updatedAt: string; ownerId?: number; ownerName?: string }
export type GoodsEstimate = GoodsEstimateSummary & { input: GoodsCalculationInput; result?: GoodsCalculationResult }
