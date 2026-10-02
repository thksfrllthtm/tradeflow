package calculation

import (
	"testing"

	"github.com/shopspring/decimal"
)

// Строка сметы округляется по графам декларации: вес — гр.35/38, цена — гр.42, таможенная
// стоимость — гр.45 (от уже округлённой цены), пошлина и НДС — гр.47.
func TestCalculateItemRoundsByDeclarationColumns(t *testing.T) {
	item := CalculationItemInput{
		Code: "4015190000", StandardName: "Перчатки", DirectorySnapshot: ApplyNomenclatureRates(nil, "4015190000", testNomenclature(), ""),
		Pairs: "3", PricePerPair: "0.335", CostCurrency: "USD",
		GrossKG: "12.34567", GrossWeightUnit: "kg", NetKG: "0.4567", NetWeightUnit: "g", PackageWeightKG: "0",
	}
	out := CalculateItem(item, decimal.RequireFromString("450"), decimal.Zero, decimal.Zero)
	checks := map[string][2]string{
		"брутто, кг (3 знака)":          {out.GrossKG, "12.346"},
		"нетто, кг (меньше грамма — 6)": {out.NetKG, "0.000457"},
		"цена товара гр.42":             {out.GoodsCost, "1.01"},
		"таможенная стоимость гр.45":    {out.CustomsValueKZT, "454.50"},
		"пошлина гр.47":                 {out.DutyKZT, "45.45"},
		"НДС гр.47 (499.95 × 16%)":      {out.VATKZT, "79.99"},
	}
	for name, pair := range checks {
		if pair[0] != pair[1] {
			t.Errorf("%s: got %q, want %q", name, pair[0], pair[1])
		}
	}

	// Расчётное нетто: брутто (уже округлённое) − места × упаковка, снова до трёх знаков.
	computed := CalculateItem(CalculationItemInput{Pairs: "3", Places: "3", GrossKG: "100.0005", GrossWeightUnit: "kg", PackageWeightKG: "0.3333", NetSource: "calculated"},
		decimal.RequireFromString("450"), decimal.Zero, decimal.Zero)
	if computed.GrossKG != "100.001" || computed.NetKG != "99.001" {
		t.Fatalf("computed weights: gross %s, net %s", computed.GrossKG, computed.NetKG)
	}
}
