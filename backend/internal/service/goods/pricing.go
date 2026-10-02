package goods

// Unit price hints for distributing a budget over an estimate: price history
// from saved calculations and DeepSeek suggestions. Final prices are computed
// in the browser; nothing here knows the budget.

import (
	"context"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
)

type PricingService struct {
	repository *repository.Repository
	ai         *ai.DeepSeekClient
}

func NewPricingService(repo *repository.Repository, ai *ai.DeepSeekClient) *PricingService {
	return &PricingService{repository: repo, ai: ai}
}

func (s *PricingService) SuggestPrices(ctx context.Context, userID int64, request ai.PriceWeightsRequest) ([]ai.SuggestedPrice, error) {
	return s.ai.SuggestUnitPrices(ctx, userID, request)
}

type UnitPrice struct {
	Code      string  `json:"code"`
	UnitPrice float64 `json:"unitPrice"`
}

// PriceHistory returns the latest known unit price for each code. userID is
// the estimate scope (repository.AllUsers for an administrator).
func (s *PricingService) PriceHistory(ctx context.Context, userID, estimateID int64, codes []string) ([]UnitPrice, error) {
	unique := make([]string, 0, len(codes))
	seen := map[string]bool{}
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code != "" && len(code) <= 64 && !seen[code] {
			seen[code] = true
			unique = append(unique, code)
		}
	}
	if len(unique) > ai.MaxPriceGroups {
		return nil, &service.ValidationError{Field: "codes", Message: "Слишком много кодов"}
	}
	result := []UnitPrice{}
	if len(unique) == 0 || s.repository == nil {
		return result, nil
	}
	prices, err := s.repository.UnitPriceHistory(ctx, userID, estimateID, unique)
	if err != nil {
		return nil, err
	}
	for _, code := range unique {
		if price, ok := prices[code]; ok {
			result = append(result, UnitPrice{Code: code, UnitPrice: price})
		}
	}
	return result, nil
}
