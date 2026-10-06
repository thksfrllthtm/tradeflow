package goods

import (
	"context"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/riskindicators"
	"github.com/shopspring/decimal"
)

func TestSIRSettingsDefaultsAndValidation(t *testing.T) {
	settings, err := parseSIRSettings(GoodsCalculationInput{CustomsValueMode: "sir", SirDiscountPercent: "-60%", SirCountry: " cn"})
	if err != nil || !settings.enabled || settings.weight != "net" || settings.discount.String() != "60" || settings.country != "CN" {
		t.Fatalf("settings %+v, %v", settings, err)
	}
	if settings, _ := parseSIRSettings(GoodsCalculationInput{}); settings.enabled || settings.country != riskindicators.DefaultCountry || settings.mode() != "invoice" {
		t.Fatalf("по умолчанию — по инвойсу, страна CN: %+v", settings)
	}
	for _, input := range []GoodsCalculationInput{
		{CustomsValueMode: "auto"}, {SirWeight: "tare"}, {SirDiscountPercent: "100"}, {SirDiscountPercent: "abc"}, {SirCountry: "China"},
	} {
		if _, err := parseSIRSettings(input); err == nil {
			t.Errorf("ожидалась ошибка для %+v", input)
		}
	}
}

func TestSIRBasisForRow(t *testing.T) {
	settings := sirSettings{enabled: true, weight: "gross", discount: decimal.RequireFromString("70"), country: "CN"}
	indicators := map[string]riskindicators.Indicator{"6107290000": {Code: "6107290000", Profile: "50100/190318/00024", PriceUSD: decimal.RequireFromString("22")}}
	if basis := settings.basis("6107290000", indicators, true); !basis.HasIndicator() || basis.PriceUSD.String() != "22" || basis.Profile != "50100/190318/00024" || basis.Weight != "gross" {
		t.Fatalf("basis %+v", basis)
	}
	if basis := settings.basis("6117100000", indicators, true); basis.HasIndicator() || !strings.Contains(basis.Missing, "6117100000") {
		t.Fatalf("нет кода: %+v", basis)
	}
	if basis := settings.basis("6107290000", nil, false); basis.HasIndicator() || !strings.Contains(basis.Missing, "не загружены") {
		t.Fatalf("не загружены: %+v", basis)
	}
}

// Смета по СИР без загруженных индикаторов считается по инвойсу и возвращает параметры СИР.
func TestGoodsCalculationSIRModeEchoesSettings(t *testing.T) {
	result, err := NewGoodsService().Calculate(context.Background(), GoodsCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", PackageWeightKg: "0.3", CustomsValueMode: "sir", SirWeight: "gross", SirDiscountPercent: "60",
		Items: []GoodsItemInput{{Name: "supplier", StandardName: "Резиновые перчатки", DirectorySnapshot: testGoodsSnapshot(), Places: "100", PairsPerPlace: "10", GrossKg: "100", Coefficient: "0.65"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.CustomsValueMode != "sir" || result.SirWeight != "gross" || result.SirDiscountPercent != "60" || result.SirCountry != "CN" {
		t.Fatalf("параметры %+v", result)
	}
	if item := result.Items[0]; item.CustomsBasis != "" || item.CustomsValueKZT == "" {
		t.Fatalf("строка: основа %q, ТС %q, %v", item.CustomsBasis, item.CustomsValueKZT, item.Warnings)
	}
}
