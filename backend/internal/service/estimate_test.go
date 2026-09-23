package service

import "testing"

func TestEstimateFromInputRejectsInvalidExchangeRate(t *testing.T) {
	_, err := estimateFromInput(EstimateInput{
		Name:           "Тестовая смета",
		AssessmentDate: "2026-09-07",
		Currency:       "USD",
		ExchangeRate:   "0",
	})
	validation, ok := err.(*ValidationError)
	if !ok || validation.Field != "exchangeRate" {
		t.Fatalf("expected exchangeRate validation error, got %v", err)
	}
}

func TestEstimateFromInputRequiresOneForKZT(t *testing.T) {
	_, err := estimateFromInput(EstimateInput{
		Name:           "Тестовая смета",
		AssessmentDate: "2026-09-07",
		Currency:       "KZT",
		ExchangeRate:   "2",
	})
	validation, ok := err.(*ValidationError)
	if !ok || validation.Field != "exchangeRate" {
		t.Fatalf("expected KZT exchangeRate validation error, got %v", err)
	}
}
