package goods

import "github.com/east-trans-logistic/import-calculator/backend/internal/service/calculation"

// Данные смет товаров: ввод и результат расчёта, сметы и черновики, результат разбора Excel.

type GoodsCalculationInput struct {
	AssessmentDate   string `json:"assessmentDate"`
	Origin           string `json:"origin"`
	UsdKzt           string `json:"usdKzt"`
	EurKzt           string `json:"eurKzt"`
	DeliveryMode     string `json:"deliveryMode"`
	Wagons           string `json:"wagons"`
	WagonTariffUsd   string `json:"wagonTariffUsd"`
	DeliveryTotalUsd string `json:"deliveryTotalUsd"`
	PackageWeightKg  string `json:"packageWeightKg"`
	CustomsFeeKzt    string `json:"customsFeeKzt"`
	Mrp              string `json:"mrp"`
	FeeMultiplier    string `json:"feeMultiplier"`
	CashUsd          string `json:"cashUsd"`
	// Таможенная стоимость: "" или "invoice" — по инвойсу, "sir" — по стоимостному индикатору риска.
	CustomsValueMode string `json:"customsValueMode,omitempty"`
	// Для режима «по СИР»: вес "net" или "gross", снижение индикатора в процентах и страна (по умолчанию CN).
	SirWeight                string               `json:"sirWeight,omitempty"`
	SirDiscountPercent       string               `json:"sirDiscountPercent,omitempty"`
	SirCountry               string               `json:"sirCountry,omitempty"`
	DirectoryCoefficientOnly bool                 `json:"directoryCoefficientOnly,omitempty"`
	GenericItems             bool                 `json:"genericItems,omitempty"`
	Items                    []GoodsItemInput     `json:"items"`
	Source                   *GoodsSourceSnapshot `json:"source,omitempty"`
	// ResultTools — что сотрудник сделал инструментами этапа «Результат»; хранится со сметой,
	// чтобы после выхода и входа было видно то же самое. Расчёт его не читает.
	ResultTools *GoodsResultTools `json:"resultTools,omitempty"`
}

// GoodsResultTools — подбор цен под бюджет и сверка по ИТП этапа «Результат».
type GoodsResultTools struct {
	Budget *GoodsBudgetState `json:"budget,omitempty"`
	ITP    *GoodsITPState    `json:"itp,omitempty"`
}

// GoodsBudgetState — бюджет, разница с ним и последний подбор: итог, сообщения, пояснения к ценам
// подбора «По товарам» (Notes — пояснение по номеру строки результата, с нуля).
type GoodsBudgetState struct {
	Amount       string                   `json:"amount"`
	GapMin       string                   `json:"gapMin"`
	GapMax       string                   `json:"gapMax"`
	GapMode      string                   `json:"gapMode"`
	Summary      *GoodsBudgetSummary      `json:"summary,omitempty"`
	Warning      string                   `json:"warning,omitempty"`
	Notice       string                   `json:"notice,omitempty"`
	Notes        map[string]string        `json:"notes,omitempty"`
	Explanations []GoodsBudgetExplanation `json:"explanations,omitempty"`
}

type GoodsBudgetSummary struct {
	Status      string  `json:"status"`
	Budget      float64 `json:"budget"`
	Total       float64 `json:"total"`
	Gap         float64 `json:"gap"`
	FixedRows   int     `json:"fixedRows"`
	Evaluations int     `json:"evaluations"`
}

type GoodsBudgetExplanation struct {
	Code   string `json:"code"`
	Names  string `json:"names"`
	Price  string `json:"price"`
	Reason string `json:"reason"`
	Rows   []int  `json:"rows"`
}

// GoodsITPState — «Сверка по ИТП»: ИТП, нетто и курс, введённые сотрудником.
type GoodsITPState struct {
	Value string `json:"value"`
	NetKg string `json:"netKg"`
	Rate  string `json:"rate"`
}

// GoodsSourceSnapshot keeps the supplier values and the operator's mapping
// separate from the editable calculation rows. It is metadata only; formulas
// never read rates or quantities from this snapshot.
type GoodsSourceSnapshot struct {
	Filename     string            `json:"filename"`
	Signature    string            `json:"signature"`
	Sheet        string            `json:"sheet"`
	HeaderRow    int               `json:"headerRow"`
	DataStartRow int               `json:"dataStartRow"`
	Headers      []string          `json:"headers"`
	Rows         []GoodsParseRow   `json:"rows"`
	Mapping      map[string]string `json:"mapping"`
}

type GoodsItemInput struct {
	ID                 string `json:"id,omitempty"`
	SourceRow          int    `json:"sourceRow,omitempty"`
	ProductDirectoryID *int64 `json:"productDirectoryId,omitempty"`
	Name               string `json:"name"`
	SupplierName       string `json:"supplierName"`
	// OtherNames — другие колонки наименования из файла («Наименование 2/3»): для подбора кода.
	OtherNames []string `json:"otherNames,omitempty"`
	// AntiDumpingChoice — вариант антидемпинга, выбранный сотрудником ("none" — не начислять).
	AntiDumpingChoice string   `json:"antiDumpingChoice,omitempty"`
	StandardName      string   `json:"standardName"`
	Code              string   `json:"code"`
	PhotoURL          string   `json:"photoUrl"`
	PhotoURLs         []string `json:"photoUrls,omitempty"`
	Pairs             string   `json:"pairs"`
	Places            string   `json:"places"`
	PairsPerPlace     string   `json:"pairsPerPlace"`
	GrossKg           string   `json:"grossKg"`
	GrossWeightUnit   string   `json:"grossWeightUnit,omitempty"`
	NetKg             string   `json:"netKg"`
	NetWeightUnit     string   `json:"netWeightUnit,omitempty"`
	NetSource         string   `json:"netSource"`
	PackageWeightKg   string   `json:"packageWeightKg"`
	Coefficient       string   `json:"coefficient"`
	PricePerPair      string   `json:"pricePerPair"`
	// SourcePricePerPair is the price from the imported Excel file. It is only
	// kept in drafts so the browser can restore PricePerPair from it; the
	// calculation itself uses PricePerPair.
	SourcePricePerPair    string         `json:"sourcePricePerPair,omitempty"`
	CostInput             string         `json:"costInput"`
	CostCurrency          string         `json:"costCurrency"`
	ExtraExpenses         string         `json:"extraExpenses"`
	CustomsValueDirectKZT string         `json:"customsValueDirectKzt"`
	DeliveryUsd           string         `json:"deliveryUsd"`
	DeliveryManual        bool           `json:"deliveryManual"`
	DeliveryAlreadyInCost bool           `json:"deliveryAlreadyInCost"`
	DeliveryIncluded      bool           `json:"deliveryIncluded"`
	Excluded              bool           `json:"excluded"`
	DirectorySnapshot     map[string]any `json:"directorySnapshot,omitempty"`
}

type GoodsCalculationResult struct {
	AssessmentDate     string                        `json:"assessmentDate"`
	Origin             string                        `json:"origin"`
	UsdKzt             string                        `json:"usdKzt"`
	EurKzt             string                        `json:"eurKzt"`
	Wagons             string                        `json:"wagons"`
	WagonTariffUsd     string                        `json:"wagonTariffUsd"`
	DeliveryMode       string                        `json:"deliveryMode"`
	DeliveryTotalUsd   string                        `json:"deliveryTotalUsd"`
	PackageWeightKg    string                        `json:"packageWeightKg"`
	CustomsFeeKzt      string                        `json:"customsFeeKzt"`
	Mrp                string                        `json:"mrp"`
	FeeMultiplier      string                        `json:"feeMultiplier"`
	CashUsd            string                        `json:"cashUsd"`
	CustomsValueMode   string                        `json:"customsValueMode"`
	SirWeight          string                        `json:"sirWeight,omitempty"`
	SirDiscountPercent string                        `json:"sirDiscountPercent,omitempty"`
	SirCountry         string                        `json:"sirCountry,omitempty"`
	Items              []calculation.CalculationItem `json:"items"`
	Totals             calculation.CalculationTotals `json:"totals"`
}

type GoodsEstimateInput struct {
	Name           string `json:"name"`
	Client         string `json:"client"`
	AssessmentDate string `json:"assessmentDate"`
}

type GoodsEstimateSummary struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Client         string `json:"client"`
	AssessmentDate string `json:"assessmentDate"`
	Status         string `json:"status"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
	// OwnerID and OwnerName are filled when an administrator opens an estimate: the page
	// marks an employee's estimate.
	OwnerID   int64  `json:"ownerId,omitempty"`
	OwnerName string `json:"ownerName,omitempty"`
}

type GoodsEstimateRecord struct {
	GoodsEstimateSummary
	Input  GoodsCalculationInput   `json:"input"`
	Result *GoodsCalculationResult `json:"result,omitempty"`
}

type GoodsSheetInfo struct {
	Name string `json:"name"`
	Rows int    `json:"rows"`
}

type GoodsParseRow struct {
	SourceRow int               `json:"sourceRow"`
	Values    map[string]string `json:"values"`
	Included  bool              `json:"included"`
	Warnings  []string          `json:"warnings,omitempty"`
}

// GoodsParseImage links a sheet row to a picture in GoodsParseResult.ImageData.
// One picture often covers several product rows, so it is sent once and
// referenced by index: repeating it per row made responses (and server memory)
// grow many times over the file size.
type GoodsParseImage struct {
	SourceRow int    `json:"sourceRow"`
	DataIndex int    `json:"dataIndex"`
	MimeType  string `json:"mimeType"`
}

type GoodsParseResult struct {
	Sheets  []GoodsSheetInfo  `json:"sheets"`
	Headers []string          `json:"headers"`
	Rows    []GoodsParseRow   `json:"rows"`
	Images  []GoodsParseImage `json:"images"`
	// ImageFiles are the picture bytes referenced by GoodsParseImage.DataIndex.
	// WriteJSON sends them as base64 "imageData" one at a time, so a large
	// workbook never exists as one huge JSON string in memory.
	ImageFiles [][]byte `json:"-"`
	Warnings   []string `json:"warnings,omitempty"`
}
