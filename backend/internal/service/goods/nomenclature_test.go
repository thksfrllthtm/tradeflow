package goods

import (
	"context"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/calculation"
	"github.com/shopspring/decimal"
)

func TestCalculationUsesNomenclatureRates(t *testing.T) {
	row := func(code string) GoodsItemInput {
		snapshot := calculation.ApplyNomenclatureRates(nil, code, testNomenclature())
		return GoodsItemInput{Name: "Перчатки", SupplierName: "Перчатки", Code: code, StandardName: directoryDisplayName(snapshot), DirectorySnapshot: snapshot, Pairs: "100", Places: "1", GrossKg: "10", PricePerPair: "1"}
	}
	result, err := NewGoodsService().Calculate(context.Background(), GoodsCalculationInput{
		AssessmentDate: "2026-09-29", UsdKzt: "500", DeliveryMode: "none", GenericItems: true,
		Items: []GoodsItemInput{row("4015190000"), row("1111111111")},
	})
	if err != nil {
		t.Fatal(err)
	}
	found, missing := result.Items[0], result.Items[1]
	// 100 × 1 USD × 500 = 50 000 ₸; пошлина 10% = 5 000; НДС 16% от 55 000 = 8 800.
	if found.CustomsValueKZT != "50000.00" || found.DutyKZT != "5000.00" || found.VATKZT != "8800.00" {
		t.Fatalf("unexpected amounts: customs %s duty %s VAT %s (errors %v)", found.CustomsValueKZT, found.DutyKZT, found.VATKZT, found.Errors)
	}
	if missing.DutyKZT != "" || len(missing.Errors) != 1 || !strings.HasPrefix(missing.Errors[0], calculation.RateErrorPrefix) {
		t.Fatalf("a code outside the nomenclature must stop with one clear error, got duty %q errors %v warnings %v", missing.DutyKZT, missing.Errors, missing.Warnings)
	}
}

// testNomenclature — номенклатура из одного кода 4015 19 000 0 (пошлина 10%, НДС 16%).
func testNomenclature() calculation.NomenclatureRates {
	ten := decimal.RequireFromString("10")
	return calculation.NomenclatureRates{Loaded: true, ActualDate: "29.09.2026", ByCode: map[string]repository.TNVEDNomenclatureCode{
		"4015190000": {Code: "4015190000", Name: "перчатки, рукавицы и митенки: 🠺 прочие", TariffText: "10%", DutyType: "percentage", DutyPercent: &ten, VATPercent: decimal.RequireFromString("16")},
	}}
}
