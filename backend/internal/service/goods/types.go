package goods

import "github.com/east-trans-logistic/import-calculator/backend/internal/service/calculation"

// Данные смет товаров: ввод и результат расчёта, сметы и черновики, результат разбора Excel.

type GoodsCalculationInput struct {
	AssessmentDate           string               `json:"assessmentDate"`
	Origin                   string               `json:"origin"`
	UsdKzt                   string               `json:"usdKzt"`
	EurKzt                   string               `json:"eurKzt"`
	DeliveryMode             string               `json:"deliveryMode"`
	Wagons                   string               `json:"wagons"`
	WagonTariffUsd           string               `json:"wagonTariffUsd"`
	DeliveryTotalUsd         string               `json:"deliveryTotalUsd"`
	PackageWeightKg          string               `json:"packageWeightKg"`
	CustomsFeeKzt            string               `json:"customsFeeKzt"`
	Mrp                      string               `json:"mrp"`
	FeeMultiplier            string               `json:"feeMultiplier"`
	CashUsd                  string               `json:"cashUsd"`
	DirectoryCoefficientOnly bool                 `json:"directoryCoefficientOnly,omitempty"`
	GenericItems             bool                 `json:"genericItems,omitempty"`
	Items                    []GoodsItemInput     `json:"items"`
	Source                   *GoodsSourceSnapshot `json:"source,omitempty"`
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
	DirectoryEntryID   *int64 `json:"directoryEntryId,omitempty"`
	ProductDirectoryID *int64 `json:"productDirectoryId,omitempty"`
	Name               string `json:"name"`
	SupplierName       string `json:"supplierName"`
	// OtherNames — другие колонки наименования из файла («Наименование 2/3»): для подбора кода.
	OtherNames      []string `json:"otherNames,omitempty"`
	StandardName    string   `json:"standardName"`
	Code            string   `json:"code"`
	PhotoURL        string   `json:"photoUrl"`
	PhotoURLs       []string `json:"photoUrls,omitempty"`
	Pairs           string   `json:"pairs"`
	Places          string   `json:"places"`
	PairsPerPlace   string   `json:"pairsPerPlace"`
	GrossKg         string   `json:"grossKg"`
	GrossWeightUnit string   `json:"grossWeightUnit,omitempty"`
	NetKg           string   `json:"netKg"`
	NetWeightUnit   string   `json:"netWeightUnit,omitempty"`
	NetSource       string   `json:"netSource"`
	PackageWeightKg string   `json:"packageWeightKg"`
	Coefficient     string   `json:"coefficient"`
	PricePerPair    string   `json:"pricePerPair"`
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
	AssessmentDate   string                        `json:"assessmentDate"`
	Origin           string                        `json:"origin"`
	UsdKzt           string                        `json:"usdKzt"`
	EurKzt           string                        `json:"eurKzt"`
	Wagons           string                        `json:"wagons"`
	WagonTariffUsd   string                        `json:"wagonTariffUsd"`
	DeliveryMode     string                        `json:"deliveryMode"`
	DeliveryTotalUsd string                        `json:"deliveryTotalUsd"`
	PackageWeightKg  string                        `json:"packageWeightKg"`
	CustomsFeeKzt    string                        `json:"customsFeeKzt"`
	Mrp              string                        `json:"mrp"`
	FeeMultiplier    string                        `json:"feeMultiplier"`
	CashUsd          string                        `json:"cashUsd"`
	Items            []calculation.CalculationItem `json:"items"`
	Totals           calculation.CalculationTotals `json:"totals"`
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
