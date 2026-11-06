package goods

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

func testGoodsSnapshot() map[string]any {
	return map[string]any{
		"entryId": float64(1), "name": "Резиновые перчатки", "code": "4015190000", "companyCoefficient": "0.65",
		"duty": map[string]any{"type": "percentage", "percent": "0"},
		"vat":  map[string]any{"type": "percentage", "percent": "0"},
	}
}

func TestGoodsCalculationUsesPairsWithoutDividingByTwo(t *testing.T) {
	svc := NewGoodsService()
	result, err := svc.Calculate(context.Background(), GoodsCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "total", DeliveryTotalUsd: "100", PackageWeightKg: "0.3",
		Items: []GoodsItemInput{{Name: "supplier", StandardName: "Резиновые перчатки", DirectorySnapshot: testGoodsSnapshot(), Places: "100", PairsPerPlace: "10", GrossKg: "100", Coefficient: "0.65", DeliveryIncluded: true}},
	})
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items: got %d", len(result.Items))
	}
	item := result.Items[0]
	if item.Pairs != "1000" || item.NetKG != "70" {
		t.Fatalf("unexpected quantities: pairs=%s net=%s", item.Pairs, item.NetKG)
	}
	if item.CostUSD != "45.50" {
		t.Fatalf("cost must be coefficient*net (0.65*70), got %s", item.CostUSD)
	}
	if item.PairPriceUSD != "0.0455" {
		t.Fatalf("pair price must use all pairs, got %s", item.PairPriceUSD)
	}
	if result.Totals.TotalDeliveryUSD != "100.00" || result.Totals.DeliveryKZT != "50000.00" {
		t.Fatalf("delivery must be a party total: %s USD / %s KZT", result.Totals.TotalDeliveryUSD, result.Totals.DeliveryKZT)
	}
	if item.Status != "calculated" {
		t.Fatalf("expected calculated item, got %s errors=%v warnings=%v", item.Status, item.Errors, item.Warnings)
	}
}

// Исключённая строка остаётся «excluded» после второго прохода (пошлина и проверки) и не
// меняет итоги.
func TestGoodsCalculationKeepsExcludedStatus(t *testing.T) {
	row := GoodsItemInput{Name: "supplier", StandardName: "Резиновые перчатки", DirectorySnapshot: testGoodsSnapshot(), Places: "100", PairsPerPlace: "10", GrossKg: "100", Coefficient: "0.65", DeliveryIncluded: true}
	excluded := row
	excluded.Name, excluded.Excluded = "excluded", true
	input := GoodsCalculationInput{AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "total", DeliveryTotalUsd: "100", PackageWeightKg: "0.3"}

	input.Items = []GoodsItemInput{row}
	alone, err := NewGoodsService().Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	input.Items = []GoodsItemInput{row, excluded}
	result, err := NewGoodsService().Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if got := result.Items[1].Status; got != "excluded" {
		t.Fatalf("excluded row status: got %s", got)
	}
	if got := result.Items[0].Status; got != "calculated" {
		t.Fatalf("included row status: got %s", got)
	}
	if !reflect.DeepEqual(result.Totals, alone.Totals) {
		t.Fatalf("excluded row changed totals:\n%+v\n%+v", result.Totals, alone.Totals)
	}
}

func TestGoodsCalculationAllowsPerRowCoefficientOverride(t *testing.T) {
	result, err := NewGoodsService().Calculate(context.Background(), GoodsCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "total", DeliveryTotalUsd: "0",
		Items: []GoodsItemInput{{Name: "supplier", StandardName: "Резиновые перчатки", Places: "1", PairsPerPlace: "10", GrossKg: "10", NetKg: "10", NetSource: "excel", GrossWeightUnit: "kg", NetWeightUnit: "kg", Coefficient: "0.80", DirectorySnapshot: testGoodsSnapshot()}},
	})
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if got := result.Items[0].Coefficient; got != "0.80" {
		t.Fatalf("expected row override to remain, got %s", got)
	}
	if got := result.Items[0].CostUSD; got != "8.00" {
		t.Fatalf("expected overridden cost, got %s", got)
	}
}

func TestGoodsCalculationRequiresDirectoryAndValidDate(t *testing.T) {
	svc := NewGoodsService()
	if _, err := svc.Calculate(context.Background(), GoodsCalculationInput{AssessmentDate: "17.09.2026", UsdKzt: "500"}); err == nil {
		t.Fatal("invalid assessment date should be rejected")
	}
	result, err := svc.Calculate(context.Background(), GoodsCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "total", DeliveryTotalUsd: "0", PackageWeightKg: "0.3",
		Items: []GoodsItemInput{{Name: "supplier", Places: "1", PairsPerPlace: "1", GrossKg: "1", Coefficient: "0.65", DeliveryIncluded: true}},
	})
	if err != nil {
		t.Fatalf("calculate incomplete item: %v", err)
	}
	if result.Items[0].Status != "needs_review" || len(result.Items[0].Errors) == 0 {
		t.Fatalf("missing directory should be a blocking review: %+v", result.Items[0])
	}
}

func TestGoodsCalculationRejectsNegativeInputs(t *testing.T) {
	svc := NewGoodsService()
	_, err := svc.Calculate(context.Background(), GoodsCalculationInput{AssessmentDate: "2026-09-17", UsdKzt: "-1"})
	if err == nil {
		t.Fatal("negative USD rate should be rejected")
	}
	if _, ok := err.(*service.ValidationError); !ok {
		t.Fatalf("expected validation error, got %T", err)
	}
}

func TestGoodsCalculationRejectsFractionalInferredPlaces(t *testing.T) {
	svc := NewGoodsService()
	result, err := svc.Calculate(context.Background(), GoodsCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "total", DeliveryTotalUsd: "0",
		Items: []GoodsItemInput{{Name: "supplier", StandardName: "Перчатки", Places: "", Pairs: "10", PairsPerPlace: "3", GrossKg: "10", NetKg: "10", NetSource: "excel", Coefficient: "1", GrossWeightUnit: "kg", NetWeightUnit: "kg", DirectorySnapshot: testGoodsSnapshot()}},
	})
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if result.Items[0].Status != "needs_review" {
		t.Fatalf("fractional inferred places must require review: %+v", result.Items[0])
	}
	found := false
	for _, issue := range result.Items[0].Errors {
		if strings.Contains(issue, "Количество мест должно быть целым") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected fractional-place error, got %v", result.Items[0].Errors)
	}
}

func TestGoodsCalculationWithoutDelivery(t *testing.T) {
	result, err := NewGoodsService().Calculate(context.Background(), GoodsCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "none", Wagons: "", PackageWeightKg: "0.3",
		Items: []GoodsItemInput{{Name: "supplier", StandardName: "Резиновые перчатки", DirectorySnapshot: testGoodsSnapshot(), Places: "100", PairsPerPlace: "10", GrossKg: "100", Coefficient: "0.65", DeliveryIncluded: true}},
	})
	if err != nil {
		t.Fatalf("calculation without delivery must be allowed: %v", err)
	}
	item := result.Items[0]
	if item.Status != "calculated" {
		t.Fatalf("expected calculated item, got %s errors=%v warnings=%v", item.Status, item.Errors, item.Warnings)
	}
	if result.Totals.TotalDeliveryUSD != "" && result.Totals.TotalDeliveryUSD != "0" && result.Totals.TotalDeliveryUSD != "0.00" {
		t.Fatalf("delivery must be zero without a delivery mode, got %s", result.Totals.TotalDeliveryUSD)
	}
}

func TestGoodsCalculationZeroFeeMultiplierDropsFee(t *testing.T) {
	row := GoodsItemInput{Name: "supplier", StandardName: "Резиновые перчатки", DirectorySnapshot: testGoodsSnapshot(), Places: "100", PairsPerPlace: "10", GrossKg: "100", Coefficient: "0.65"}
	input := GoodsCalculationInput{AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "none", PackageWeightKg: "0.3", Mrp: "4325", FeeMultiplier: "0", Items: []GoodsItemInput{row, row}}
	result, err := NewGoodsService().Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("multiplier 0 must be allowed: %v", err)
	}
	if result.Totals.FeeKZT != "0.00" {
		t.Fatalf("fee must be zero with multiplier 0, got %s", result.Totals.FeeKZT)
	}
	withFee := input
	withFee.FeeMultiplier = "6"
	expected, err := NewGoodsService().Calculate(context.Background(), withFee)
	if err != nil {
		t.Fatalf("calculate with fee: %v", err)
	}
	// Сбор входит в базу НДС: без сбора растаможка меньше на сбор и на НДС с него.
	if result.Totals.CustomsPaymentsKZT == expected.Totals.CustomsPaymentsKZT {
		t.Fatalf("zero fee must lower customs payments, both %s", result.Totals.CustomsPaymentsKZT)
	}
	if expected.Totals.FeeKZT != "25950.00" {
		t.Fatalf("fee with multiplier 6 must be 4325 × 6 = 25950.00, got %s", expected.Totals.FeeKZT)
	}
}

func TestGoodsCalculationUsesExcelPriceOnlyWhereGiven(t *testing.T) {
	withPrice := GoodsItemInput{Name: "with price", StandardName: "Резиновые перчатки", DirectorySnapshot: testGoodsSnapshot(), Places: "100", PairsPerPlace: "10", GrossKg: "100", Coefficient: "0.65", PricePerPair: "1.5", SourcePricePerPair: "1.5"}
	withoutPrice := GoodsItemInput{Name: "without price", StandardName: "Резиновые перчатки", DirectorySnapshot: testGoodsSnapshot(), Places: "100", PairsPerPlace: "10", GrossKg: "100", Coefficient: "0.65"}
	input := GoodsCalculationInput{AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "none", PackageWeightKg: "0.3", Items: []GoodsItemInput{withPrice, withoutPrice}}
	result, err := NewGoodsService().Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if got := result.Items[0].CostUSD; got != "1500.00" {
		t.Fatalf("row with an Excel price must cost price × pairs (1.5 × 1000), got %s", got)
	}
	if got := result.Items[1].CostUSD; got != "45.50" {
		t.Fatalf("row without a price must keep the usual ИТС × net cost (0.65 × 70), got %s", got)
	}

	encoded, _ := json.Marshal(input)
	var restored GoodsCalculationInput
	if err := json.Unmarshal(encoded, &restored); err != nil || restored.Items[0].SourcePricePerPair != "1.5" {
		t.Fatalf("the Excel price must survive a saved draft, got %+v (%v)", restored.Items[0].SourcePricePerPair, err)
	}
}

func TestGoodsCalculationFillsMissingQuantity(t *testing.T) {
	for name, item := range map[string]GoodsItemInput{
		"per place from quantity and places": {Pairs: "800", Places: "20", GrossKg: "1220"},
		"places from quantity and per place": {Pairs: "800", PairsPerPlace: "40", GrossKg: "1220"},
		"quantity from places and per place": {Places: "20", PairsPerPlace: "40", GrossKg: "1220"},
	} {
		item.Name, item.StandardName, item.DirectorySnapshot, item.Coefficient = "row", "Резиновые перчатки", testGoodsSnapshot(), "0.65"
		result, err := NewGoodsService().Calculate(context.Background(), GoodsCalculationInput{AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "none", PackageWeightKg: "0.3", Items: []GoodsItemInput{item}})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := result.Items[0]
		if got.Pairs != "800" || got.Places != "20" || got.PairsPerPlace != "40" || got.NetKG != "1214" {
			t.Fatalf("%s: got pairs=%s places=%s perPlace=%s net=%s", name, got.Pairs, got.Places, got.PairsPerPlace, got.NetKG)
		}
	}
}
