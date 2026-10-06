package calculation

import (
	"slices"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func sirItem(sir *SIRBasis, change func(*CalculationItemInput)) CalculationItemInput {
	input := CalculationItemInput{Code: "4015190000", StandardName: "Перчатки", Pairs: "10", PricePerPair: "2", CostCurrency: "USD",
		GrossKG: "110", GrossWeightUnit: "kg", NetKG: "100", NetWeightUnit: "kg", PackageWeightKG: "0", GenericItems: true, SIR: sir}
	input.DirectorySnapshot = ApplyNomenclatureRates(nil, input.Code, testNomenclature(), "")
	if change != nil {
		change(&input)
	}
	return input
}

func sirBasis(price, discount, weight string) *SIRBasis {
	value := decimal.RequireFromString(price)
	return &SIRBasis{PriceUSD: &value, Profile: "50100/190318/00024", Country: "CN", Weight: weight, DiscountPercent: decimal.RequireFromString(discount)}
}

// ТС по СИР: 100 кг нетто × 27 USD × (1 − 60 %) = 1 080 USD × 500 = 540 000; цена инвойса (20 USD) не участвует.
func TestSIRCustomsValueFromNetWeight(t *testing.T) {
	usd := decimal.RequireFromString("500")
	out := CalculateItem(sirItem(sirBasis("27", "60", "net"), nil), usd, decimal.Zero, decimal.Zero)
	if out.CustomsValueKZT != "540000.00" || out.DutyKZT != "54000.00" || out.VATKZT != "95040.00" || out.Status != "calculated" {
		t.Fatalf("ТС %s, пошлина %s, НДС %s, статус %s, %v %v", out.CustomsValueKZT, out.DutyKZT, out.VATKZT, out.Status, out.Errors, out.Warnings)
	}
	if out.CustomsBasis != "sir" || out.SIRResult == nil || out.SIRResult.PricePerKgUSD != "10.8" || out.SIRResult.ValueUSD != "1080.00" || out.SIRResult.WeightKg != "100" {
		t.Fatalf("как посчитано: %s %+v", out.CustomsBasis, out.SIRResult)
	}
	if out.GoodsCost != "20.00" {
		t.Fatalf("стоимость по инвойсу остаётся видна: %s", out.GoodsCost)
	}
}

// По брутто, без снижения, с доставкой: доставка добавляется, даже если отмечено «уже в цене».
func TestSIRCustomsValueFromGrossWeightWithDelivery(t *testing.T) {
	usd := decimal.RequireFromString("500")
	items := []CalculationItem{CalculateItem(sirItem(sirBasis("1.25", "0", "gross"), func(input *CalculationItemInput) { input.DeliveryAlreadyInCost = true }), usd, decimal.Zero, decimal.Zero)}
	total := decimal.RequireFromString("100")
	AllocateDelivery(items, &total, usd)
	items[0].Warnings = FilterRateWarnings(items[0].Warnings)
	CalculateTariff(&items[0], nil, nil, nil, usd, decimal.Zero, &items[0].Warnings)
	// 110 × 1,25 = 137,50 USD; + доставка 100 USD = 237,50 × 500.
	if items[0].CustomsValueKZT != "118750.00" || items[0].SIRResult.Weight != "gross" {
		t.Fatalf("ТС %s, %+v", items[0].CustomsValueKZT, items[0].SIRResult)
	}
}

// По СИР цена инвойса не обязательна.
func TestSIRDoesNotRequireInvoicePrice(t *testing.T) {
	out := CalculateItem(sirItem(sirBasis("27", "0", "net"), func(input *CalculationItemInput) { input.PricePerPair = "" }), decimal.RequireFromString("500"), decimal.Zero, decimal.Zero)
	if out.Status != "calculated" || out.CustomsValueKZT != "1350000.00" {
		t.Fatalf("статус %s, ТС %s, %v %v", out.Status, out.CustomsValueKZT, out.Errors, out.Warnings)
	}
}

// Индикатора нет — ТС по инвойсу и одно предупреждение, в том числе после повторного расчёта.
func TestSIRMissingIndicatorFallsBackToInvoice(t *testing.T) {
	usd := decimal.RequireFromString("500")
	message := "СИР: для кода 4015190000 и страны CN индикатора нет — ТС по инвойсу"
	out := CalculateItem(sirItem(&SIRBasis{Missing: message, Country: "CN", Weight: "net"}, nil), usd, decimal.Zero, decimal.Zero)
	out.Warnings = FilterRateWarnings(out.Warnings)
	CalculateTariff(&out, nil, nil, nil, usd, decimal.Zero, &out.Warnings)
	count := 0
	for _, warning := range out.Warnings {
		if warning == message {
			count++
		}
	}
	if out.CustomsValueKZT != "10000.00" || out.CustomsBasis != "" || count != 1 {
		t.Fatalf("ТС %s, основа %q, предупреждения %v", out.CustomsValueKZT, out.CustomsBasis, out.Warnings)
	}
}

// Без веса посчитать ТС по СИР нельзя — ошибка строки.
func TestSIRRequiresWeight(t *testing.T) {
	out := CalculateItem(sirItem(sirBasis("27", "0", "net"), func(input *CalculationItemInput) { input.NetKG, input.GrossKG = "", "" }), decimal.RequireFromString("500"), decimal.Zero, decimal.Zero)
	if out.CustomsValueKZT != "" || !slices.ContainsFunc(out.Errors, func(issue string) bool { return strings.Contains(issue, "нужен вес нетто") }) {
		t.Fatalf("ТС %s, ошибки %v", out.CustomsValueKZT, out.Errors)
	}
}
