package calculation

// Строки расчёта растаможки: ввод строки, посчитанная строка и итоги сметы товаров.

import (
	"github.com/shopspring/decimal"
)

type CalculationItemInput struct {
	ProductID             int64          `json:"productId"`
	SourceRow             int            `json:"sourceRow,omitempty"`
	DirectoryEntryID      *int64         `json:"directoryEntryId,omitempty"`
	DirectorySnapshot     map[string]any `json:"directorySnapshot,omitempty"`
	StandardName          string         `json:"standardName"`
	SupplierName          string         `json:"supplierName"`
	PhotoURL              string         `json:"photoUrl"`
	ImageAssetIDs         []int64        `json:"imageAssetIds,omitempty"`
	Code                  string         `json:"code"`
	Pairs                 string         `json:"pairs"`
	Places                string         `json:"places"`
	PairsPerPlace         string         `json:"pairsPerPlace"`
	GrossKG               string         `json:"grossKg"`
	GrossWeightUnit       string         `json:"grossWeightUnit,omitempty"`
	NetKG                 string         `json:"netKg"`
	NetWeightUnit         string         `json:"netWeightUnit,omitempty"`
	NetSource             string         `json:"netSource"`
	PackageWeightKG       string         `json:"packageWeightKg"`
	Coefficient           string         `json:"coefficient"`
	PricePerPair          string         `json:"pricePerPair"`
	CostInput             string         `json:"costInput"`
	CostCurrency          string         `json:"costCurrency"`
	ExtraExpenses         string         `json:"extraExpenses"`
	CustomsValueDirectKZT string         `json:"customsValueDirectKzt"`
	DeliveryUSD           string         `json:"deliveryUsd"`
	DeliveryManual        bool           `json:"deliveryManual"`
	DeliveryAlreadyInCost bool           `json:"deliveryAlreadyInCost"`
	DeliveryIncluded      bool           `json:"deliveryIncluded"`
	Excluded              bool           `json:"excluded"`
	ExclusionReason       string         `json:"exclusionReason"`
	GenericItems          bool           `json:"-"`
}

type CalculationItem struct {
	CalculationItemInput
	CostUSD           string   `json:"costUsd"`
	PairPriceUSD      string   `json:"pairPriceUsd"`
	GoodsCost         string   `json:"goodsCost"`
	GoodsCostCurrency string   `json:"goodsCostCurrency"`
	PairPrice         string   `json:"pairPrice"`
	CustomsValueKZT   string   `json:"customsValueKzt"`
	DutyKZT           string   `json:"dutyKzt"`
	AntiDumpingKZT    string   `json:"antiDumpingKzt"`
	VATKZT            string   `json:"vatKzt"`
	VATBaseKZT        string   `json:"vatBaseKzt"`
	FeeKZT            string   `json:"feeKzt"`
	DeliveryKZT       string   `json:"deliveryKzt"`
	Status            string   `json:"status"`
	Errors            []string `json:"errors"`
	Warnings          []string `json:"warnings"`

	// Derived working values for the current recalculation. They are never
	// persisted and keep the exact decimal inputs until the customs value is
	// rounded at the two-decimal display boundary.
	costDecimal        *decimal.Decimal `json:"-"`
	costCurrencyValue  string           `json:"-"`
	extraDecimal       *decimal.Decimal `json:"-"`
	deliveryShareExact *decimal.Decimal `json:"-"`
}

type CalculationTotals struct {
	TotalCostUSD          string   `json:"totalCostUsd"`
	TotalDeliveryUSD      string   `json:"totalDeliveryUsd"`
	DeliveryKZT           string   `json:"deliveryKzt"`
	DeliveryInCustomsKZT  string   `json:"deliveryInCustomsKzt"`
	DeliveryInExpensesKZT string   `json:"deliveryInExpensesKzt"`
	CustomsValueKZT       string   `json:"customsValueKzt"`
	DutyKZT               string   `json:"dutyKzt"`
	AntiDumpingKZT        string   `json:"antiDumpingKzt"`
	VATKZT                string   `json:"vatKzt"`
	FeeKZT                string   `json:"feeKzt"`
	CustomsPaymentsKZT    string   `json:"customsPaymentsKzt"`
	ExpensesKZT           string   `json:"expensesKzt"`
	ExpensesUSD           string   `json:"expensesUsd"`
	CashBalanceUSD        string   `json:"cashBalanceUsd"`
	CalculatedCount       int      `json:"calculatedCount"`
	TotalCount            int      `json:"totalCount"`
	NeedsReview           bool     `json:"needsReview"`
	ErrorCount            int      `json:"errorCount"`
	WarningCount          int      `json:"warningCount"`
	BlockingErrors        []string `json:"blockingErrors"`
}
