package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCatalogValidationKeepsLeadingZeros(t *testing.T) {
	s := &CatalogService{}
	report, _ := s.Validate(CatalogImportRequest{Version: CatalogVersionInput{VersionKey: "v1", DatasetKind: "demo", Source: "fixture"}, Codes: []CatalogCodeInput{{Code: "000123", HierarchyLevel: 6, OfficialDescription: "demo", ValidFrom: "2024-01-01", Source: "fixture"}}})
	if !report.Valid {
		t.Fatalf("unexpected validation errors: %#v", report.Errors)
	}
	if !catalogCodePattern.MatchString("000123") {
		t.Fatal("leading-zero code rejected")
	}
}

func TestCatalogValidationRejectsOverlappingRules(t *testing.T) {
	s := &CatalogService{}
	input := CatalogImportRequest{Version: CatalogVersionInput{VersionKey: "v1", DatasetKind: "demo", Source: "fixture"}, Codes: []CatalogCodeInput{{Code: "000123", OfficialDescription: "demo", ValidFrom: "2024-01-01", Source: "fixture"}}, Rules: []CatalogRuleInput{
		{Code: "000123", RuleKey: "a", PaymentType: "duty", RateType: "ad_valorem", Value: json.RawMessage("5"), ValidFrom: "2024-01-01", Source: "fixture"},
		{Code: "000123", RuleKey: "b", PaymentType: "duty", RateType: "ad_valorem", Value: json.RawMessage("7"), ValidFrom: "2024-06-01", Source: "fixture"},
	}}
	report, _ := s.Validate(input)
	if report.Valid || !strings.Contains(strings.Join(report.Errors, " "), "пересекается") {
		t.Fatalf("expected overlap error, got %#v", report.Errors)
	}
}
