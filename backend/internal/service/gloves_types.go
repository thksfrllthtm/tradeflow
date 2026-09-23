package service

// Request/response shapes of the gloves calculator: calculation input/result, drafts, estimates, XLSX parse result.

type GlovesCalculationInput struct {
	AssessmentDate           string                `json:"assessmentDate"`
	Origin                   string                `json:"origin"`
	UsdKzt                   string                `json:"usdKzt"`
	EurKzt                   string                `json:"eurKzt"`
	DeliveryMode             string                `json:"deliveryMode"`
	Wagons                   string                `json:"wagons"`
	WagonTariffUsd           string                `json:"wagonTariffUsd"`
	DeliveryTotalUsd         string                `json:"deliveryTotalUsd"`
	PackageWeightKg          string                `json:"packageWeightKg"`
	CustomsFeeKzt            string                `json:"customsFeeKzt"`
	Mrp                      string                `json:"mrp"`
	FeeMultiplier            string                `json:"feeMultiplier"`
	CashUsd                  string                `json:"cashUsd"`
	DirectoryCoefficientOnly bool                  `json:"directoryCoefficientOnly,omitempty"`
	GenericItems             bool                  `json:"genericItems,omitempty"`
	Items                    []GlovesItemInput     `json:"items"`
	Source                   *GlovesSourceSnapshot `json:"source,omitempty"`
}

// GlovesSourceSnapshot keeps the supplier values and the operator's mapping
// separate from the editable calculation rows. It is metadata only; formulas
// never read rates or quantities from this snapshot.
type GlovesSourceSnapshot struct {
	Filename     string            `json:"filename"`
	Signature    string            `json:"signature"`
	Sheet        string            `json:"sheet"`
	HeaderRow    int               `json:"headerRow"`
	DataStartRow int               `json:"dataStartRow"`
	Headers      []string          `json:"headers"`
	Rows         []GlovesParseRow  `json:"rows"`
	Mapping      map[string]string `json:"mapping"`
}

type GlovesItemInput struct {
	ID                    string         `json:"id,omitempty"`
	SourceRow             int            `json:"sourceRow,omitempty"`
	DirectoryEntryID      *int64         `json:"directoryEntryId,omitempty"`
	ProductDirectoryID    *int64         `json:"productDirectoryId,omitempty"`
	Name                  string         `json:"name"`
	SupplierName          string         `json:"supplierName"`
	StandardName          string         `json:"standardName"`
	Code                  string         `json:"code"`
	PhotoURL              string         `json:"photoUrl"`
	PhotoURLs             []string       `json:"photoUrls,omitempty"`
	Pairs                 string         `json:"pairs"`
	Places                string         `json:"places"`
	PairsPerPlace         string         `json:"pairsPerPlace"`
	GrossKg               string         `json:"grossKg"`
	GrossWeightUnit       string         `json:"grossWeightUnit,omitempty"`
	NetKg                 string         `json:"netKg"`
	NetWeightUnit         string         `json:"netWeightUnit,omitempty"`
	NetSource             string         `json:"netSource"`
	PackageWeightKg       string         `json:"packageWeightKg"`
	Coefficient           string         `json:"coefficient"`
	PricePerPair          string         `json:"pricePerPair"`
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

type GlovesCalculationResult struct {
	AssessmentDate   string            `json:"assessmentDate"`
	Origin           string            `json:"origin"`
	UsdKzt           string            `json:"usdKzt"`
	EurKzt           string            `json:"eurKzt"`
	Wagons           string            `json:"wagons"`
	WagonTariffUsd   string            `json:"wagonTariffUsd"`
	DeliveryMode     string            `json:"deliveryMode"`
	DeliveryTotalUsd string            `json:"deliveryTotalUsd"`
	PackageWeightKg  string            `json:"packageWeightKg"`
	CustomsFeeKzt    string            `json:"customsFeeKzt"`
	Mrp              string            `json:"mrp"`
	FeeMultiplier    string            `json:"feeMultiplier"`
	CashUsd          string            `json:"cashUsd"`
	Items            []CalculationItem `json:"items"`
	Totals           CalculationTotals `json:"totals"`
}

type GlovesDraft struct {
	Input     GlovesCalculationInput   `json:"input"`
	Result    *GlovesCalculationResult `json:"result,omitempty"`
	UpdatedAt string                   `json:"updatedAt"`
}

type GlovesEstimateInput struct {
	Name           string `json:"name"`
	Client         string `json:"client"`
	AssessmentDate string `json:"assessmentDate"`
}

type GlovesEstimateSummary struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Client         string `json:"client"`
	AssessmentDate string `json:"assessmentDate"`
	Status         string `json:"status"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

type GlovesEstimateRecord struct {
	GlovesEstimateSummary
	Input  GlovesCalculationInput   `json:"input"`
	Result *GlovesCalculationResult `json:"result,omitempty"`
}

type GlovesSheetInfo struct {
	Name string `json:"name"`
	Rows int    `json:"rows"`
}

type GlovesParseRow struct {
	SourceRow int               `json:"sourceRow"`
	Values    map[string]string `json:"values"`
	Included  bool              `json:"included"`
	Warnings  []string          `json:"warnings,omitempty"`
}

type GlovesParseImage struct {
	SourceRow int    `json:"sourceRow"`
	Data      string `json:"data"`
	MimeType  string `json:"mimeType"`
}

type GlovesParseResult struct {
	Sheets   []GlovesSheetInfo  `json:"sheets"`
	Headers  []string           `json:"headers"`
	Rows     []GlovesParseRow   `json:"rows"`
	Images   []GlovesParseImage `json:"images"`
	Warnings []string           `json:"warnings,omitempty"`
}
