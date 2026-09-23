package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestGlovesParseXlsxReturnsHeadersAndSourceRows(t *testing.T) {
	book := excelize.NewFile()
	if err := book.SetCellValue("Sheet1", "A1", "Наименование"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "B1", "Места"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "A2", "Перчатки"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "B2", 3); err != nil {
		t.Fatal(err)
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := NewGlovesService().ParseXlsx(context.Background(), bytes.NewReader(buffer.Bytes()), "test.xlsx", "Sheet1", 1, 2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Headers) != 2 || parsed.Headers[0] != "Наименование" || len(parsed.Rows) != 1 || parsed.Rows[0].SourceRow != 2 {
		t.Fatalf("unexpected parse result: %+v", parsed)
	}
}

func TestGlovesParseXlsxExcludesTotalRows(t *testing.T) {
	book := excelize.NewFile()
	if err := book.SetCellValue("Sheet1", "A1", "Наименование"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "A2", "Перчатки"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "A3", "Итого"); err != nil {
		t.Fatal(err)
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := NewGlovesService().ParseXlsx(context.Background(), bytes.NewReader(buffer.Bytes()), "test.xlsx", "Sheet1", 1, 2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Rows) != 2 || !parsed.Rows[0].Included || parsed.Rows[1].Included {
		t.Fatalf("expected product and excluded total row, got %+v", parsed.Rows)
	}
	if len(parsed.Rows[1].Warnings) == 0 || len(parsed.Warnings) == 0 {
		t.Fatalf("expected explicit total-row warnings, got row=%v result=%v", parsed.Rows[1].Warnings, parsed.Warnings)
	}
}

func TestGlovesParseXlsxKeepsBlankAndDuplicateHeaders(t *testing.T) {
	book := excelize.NewFile()
	for cell, value := range map[string]string{"A1": "Наименование", "B1": "", "C1": "Наименование", "A2": "Перчатки", "B2": "10", "C2": "другое"} {
		if err := book.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := NewGlovesService().ParseXlsx(context.Background(), bytes.NewReader(buffer.Bytes()), "test.xlsx", "Sheet1", 1, 2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Headers) != 3 || parsed.Headers[1] != "Колонка 2" || parsed.Headers[2] != "Наименование (2)" {
		t.Fatalf("unexpected canonical headers: %+v", parsed.Headers)
	}
	if parsed.Rows[0].Values["Колонка 2"] != "10" || parsed.Rows[0].Values["Наименование (2)"] != "другое" {
		t.Fatalf("values from blank/duplicate columns were lost: %+v", parsed.Rows[0].Values)
	}
}

func TestGlovesParseXlsxRejectsCorruptWorkbook(t *testing.T) {
	_, err := NewGlovesService().ParseXlsx(context.Background(), bytes.NewReader([]byte("not an xlsx")), "bad.xlsx", "", 1, 2)
	if err == nil || !strings.Contains(err.Error(), "Файл повреждён") {
		t.Fatalf("expected clear corrupt-file error, got %v", err)
	}
}

func TestValidateGlovesPhotoData(t *testing.T) {
	valid := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("png"))
	if err := validateGlovesPhotoData(valid); err != nil {
		t.Fatalf("valid photo rejected: %v", err)
	}
	if err := validateGlovesPhotoData("data:image/gif;base64," + base64.StdEncoding.EncodeToString([]byte("gif"))); err == nil {
		t.Fatal("unsupported image type should be rejected")
	}
	if err := validateGlovesPhotoData("data:image/png;base64,not-base64"); err == nil {
		t.Fatal("invalid base64 should be rejected")
	}
}

func testGlovesSnapshot() map[string]any {
	return map[string]any{
		"entryId": float64(1), "name": "Резиновые перчатки", "code": "4015190000", "companyCoefficient": "0.65",
		"duty": map[string]any{"type": "percentage", "percent": "0"},
		"vat":  map[string]any{"type": "percentage", "percent": "0"},
	}
}

func TestGlovesCalculationUsesPairsWithoutDividingByTwo(t *testing.T) {
	service := NewGlovesService()
	result, err := service.Calculate(context.Background(), GlovesCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "total", DeliveryTotalUsd: "100", PackageWeightKg: "0.3",
		Items: []GlovesItemInput{{Name: "supplier", StandardName: "Резиновые перчатки", DirectorySnapshot: testGlovesSnapshot(), Places: "100", PairsPerPlace: "10", GrossKg: "100", Coefficient: "0.65", DeliveryIncluded: true}},
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

func TestGlovesCalculationAllowsPerRowCoefficientOverride(t *testing.T) {
	result, err := NewGlovesService().Calculate(context.Background(), GlovesCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "total", DeliveryTotalUsd: "0",
		Items: []GlovesItemInput{{Name: "supplier", StandardName: "Резиновые перчатки", Places: "1", PairsPerPlace: "10", GrossKg: "10", NetKg: "10", NetSource: "excel", GrossWeightUnit: "kg", NetWeightUnit: "kg", Coefficient: "0.80", DirectorySnapshot: testGlovesSnapshot()}},
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

func TestGlovesCalculationRequiresDirectoryAndValidDate(t *testing.T) {
	service := NewGlovesService()
	if _, err := service.Calculate(context.Background(), GlovesCalculationInput{AssessmentDate: "17.09.2026", UsdKzt: "500"}); err == nil {
		t.Fatal("invalid assessment date should be rejected")
	}
	result, err := service.Calculate(context.Background(), GlovesCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "total", DeliveryTotalUsd: "0", PackageWeightKg: "0.3",
		Items: []GlovesItemInput{{Name: "supplier", Places: "1", PairsPerPlace: "1", GrossKg: "1", Coefficient: "0.65", DeliveryIncluded: true}},
	})
	if err != nil {
		t.Fatalf("calculate incomplete item: %v", err)
	}
	if result.Items[0].Status != "needs_review" || len(result.Items[0].Errors) == 0 {
		t.Fatalf("missing directory should be a blocking review: %+v", result.Items[0])
	}
}

func TestGlovesCalculationRejectsNegativeInputs(t *testing.T) {
	service := NewGlovesService()
	_, err := service.Calculate(context.Background(), GlovesCalculationInput{AssessmentDate: "2026-09-17", UsdKzt: "-1"})
	if err == nil {
		t.Fatal("negative USD rate should be rejected")
	}
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("expected validation error, got %T", err)
	}
}

func TestGlovesCalculationRejectsFractionalInferredPlaces(t *testing.T) {
	service := NewGlovesService()
	result, err := service.Calculate(context.Background(), GlovesCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "total", DeliveryTotalUsd: "0",
		Items: []GlovesItemInput{{Name: "supplier", StandardName: "Перчатки", Places: "", Pairs: "10", PairsPerPlace: "3", GrossKg: "10", NetKg: "10", NetSource: "excel", Coefficient: "1", GrossWeightUnit: "kg", NetWeightUnit: "kg", DirectorySnapshot: testGlovesSnapshot()}},
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
