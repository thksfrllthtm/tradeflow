package calculation

import (
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/antidumping"
	"github.com/shopspring/decimal"
)

// Антидемпинг из реестра ЕЭК: по коду и стране происхождения, сверх пошлины, входит в базу НДС.
func TestAntiDumpingFromRegistryIsChargedAndInVATBase(t *testing.T) {
	rates := testNomenclature()
	rates.AntiDumping = []antidumping.Measure{{Number: "AD-99", ShortName: "Перчатки", Codes: []string{"4015190000"}, Countries: []string{"CN"}, Rates: map[string]string{"CN": "20"}}}
	item := func(origin string) CalculationItem {
		rates.Origin = origin
		input := CalculationItemInput{Code: "4015190000", StandardName: "Перчатки", Pairs: "10", PricePerPair: "2", CostCurrency: "USD", GrossKG: "5", GrossWeightUnit: "kg", PackageWeightKG: "0"}
		input.DirectorySnapshot = ApplyNomenclatureRates(nil, input.Code, rates)
		out := CalculateItem(input, decimal.RequireFromString("500"), decimal.Zero, decimal.Zero)
		CalculateTariff(&out, nil, ParseItemStringDecimal(out.Pairs), nil, decimal.RequireFromString("500"), decimal.Zero, &out.Warnings)
		return out
	}
	china := item("Китай")
	// Таможенная стоимость 20 USD × 500 = 10 000; пошлина 10 % = 1 000; антидемпинг 20 % = 2 000;
	// НДС 16 % от 10 000 + 1 000 + 2 000 = 2 080.
	if china.CustomsValueKZT != "10000.00" || china.DutyKZT != "1000.00" || china.AntiDumpingKZT != "2000.00" || china.VATKZT != "2080.00" {
		t.Fatalf("Китай: таможенная %s, пошлина %s, антидемпинг %s, НДС %s, предупреждения %v", china.CustomsValueKZT, china.DutyKZT, china.AntiDumpingKZT, china.VATKZT, china.Warnings)
	}
	if other := item("Германия"); other.AntiDumpingKZT != "0.00" || other.VATKZT != "1760.00" {
		t.Fatalf("не Китай — без антидемпинга: %s, НДС %s", other.AntiDumpingKZT, other.VATKZT)
	}
	unknown := item("")
	if unknown.AntiDumpingKZT != "0.00" || !strings.Contains(strings.Join(unknown.Warnings, " "), "укажите страну") {
		t.Fatalf("страна не указана — предупредить: %s %v", unknown.AntiDumpingKZT, unknown.Warnings)
	}
}
