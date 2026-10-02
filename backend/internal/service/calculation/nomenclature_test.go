package calculation

import (
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/shopspring/decimal"
)

func testNomenclature() NomenclatureRates {
	ten := decimal.RequireFromString("10")
	return NomenclatureRates{Loaded: true, ActualDate: "29.09.2026", ByCode: map[string]repository.TNVEDNomenclatureCode{
		"4015190000": {Code: "4015190000", Name: "перчатки, рукавицы и митенки: 🠺 прочие", TariffText: "10%", DutyType: "percentage", DutyPercent: &ten, VATPercent: decimal.RequireFromString("16")},
	}}
}

func TestRowTNVEDCodeReadsExcelForms(t *testing.T) {
	for cell, want := range map[string]string{"4015190000": "4015190000", "4015 19 000 0": "4015190000", "4015190000.0": "4015190000", "'4015190000": "4015190000", "12345": "", "": ""} {
		if got := RowTNVEDCode(nil, cell); got != want {
			t.Errorf("%q: got %q, want %q", cell, got, want)
		}
	}
	if got := RowTNVEDCode(map[string]any{"code": "6116102000"}, "4015190000"); got != "6116102000" {
		t.Fatalf("the chosen card's code wins over the Excel cell, got %q", got)
	}
}

func TestApplyNomenclatureRatesTakesDutyAndVATOnlyFromNomenclature(t *testing.T) {
	// A directory card with its own rates, anti-dumping and a product ИТС.
	directory := map[string]any{
		"code": "4015190000", "name": "Перчатки нитриловые", "companyCoefficient": "0.65", "antiDumpingPercent": "5", "verificationStatus": "operator_verified",
		"duty": map[string]any{"type": "percentage", "percent": "8"}, "vat": map[string]any{"type": "percentage", "percent": "12"},
	}
	result := ApplyNomenclatureRates(directory, "", testNomenclature())
	duty, _ := result["duty"].(map[string]any)
	vat, _ := result["vat"].(map[string]any)
	if duty["percent"] != "10" || duty["sourceKind"] != "nomenclature" || vat["percent"] != "16" {
		t.Fatalf("duty and VAT must come from the nomenclature: %v %v", duty, vat)
	}
	if _, ok := result["antiDumpingPercent"]; ok {
		t.Fatal("anti-dumping of the directory must not be used")
	}
	if result["name"] != "Перчатки нитриловые" || result["companyCoefficient"] != "0.65" {
		t.Fatalf("name and ИТС of the chosen card stay: %v", result)
	}
	if directory["antiDumpingPercent"] != "5" {
		t.Fatal("the directory snapshot itself must not be changed")
	}

	// Only a code from Excel: the row gets the nomenclature name and rates.
	fromExcel := ApplyNomenclatureRates(nil, "4015190000", testNomenclature())
	if fromExcel["name"] != "перчатки, рукавицы и митенки: прочие" || fromExcel["code"] != "4015190000" {
		t.Fatalf("an Excel code must be taken from the nomenclature: %v", fromExcel)
	}
	if ApplyNomenclatureRates(nil, "", testNomenclature()) != nil {
		t.Fatal("a row without a code keeps no snapshot")
	}
}

func TestApplyNomenclatureRatesExplainsMissingCode(t *testing.T) {
	missing := ApplyNomenclatureRates(nil, "1111111111", testNomenclature())
	if message, _ := missing["rateError"].(string); !strings.Contains(message, "кода 1111111111 нет в номенклатуре") {
		t.Fatalf("expected a missing-code message, got %q", message)
	}
	notLoaded := ApplyNomenclatureRates(nil, "4015190000", NomenclatureRates{})
	if message, _ := notLoaded["rateError"].(string); !strings.Contains(message, "не загружена") {
		t.Fatalf("expected a not-loaded message, got %q", message)
	}
}
